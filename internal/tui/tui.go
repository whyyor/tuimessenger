// gomuks - A terminal Matrix client written in Go.
// Copyright (C) 2025 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package tui

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"

	"github.com/gdamore/tcell/v2"
	"github.com/zyedidia/clipboard"
	"go.mau.fi/mauview"
	"go.mau.fi/util/exerrors"
	"go.mau.fi/util/exzerolog"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
	"github.com/whyyor/tuimessenger/internal/hicli/jsoncmd"
	"github.com/whyyor/tuimessenger/internal/rpc"
	"github.com/whyyor/tuimessenger/internal/rpc/client"
	"github.com/whyyor/tuimessenger/internal/rpc/store"
	"github.com/whyyor/tuimessenger/internal/tui/config"
	"github.com/whyyor/tuimessenger/internal/tui/debug"
	"github.com/whyyor/tuimessenger/internal/tui/messages"
)

type View string

// Allowed views in App
const (
	ViewMain  View = "main"
	ViewSetup View = "setup"
)

type App struct {
	gmx *client.Client
	app *mauview.Application

	// Backend is what the TUI talks to; set it before Run (tuimessenger runs hicli in-process).
	Backend client.Backend

	Config *config.Config

	// LoggedIn says whether the engine already has a login; without one the setup screen comes first.
	LoggedIn bool
	// Setup is what the setup screen's Complete setup logs in with.
	Setup   SetupDefaults
	inSetup atomic.Bool

	MainView *MainView

	NeedsRender bool

	// ConnState is the state of the client engine (hicli) as the TUI sees it.
	ConnState rpc.ConnState
	// Resyncing is true from a manual resync until the engine finishes
	// sending the fresh initial data; this is the phase a human can see.
	Resyncing bool
	// SyncStatus mirrors the engine's sync with the tuimessenger backend,
	// the layer that actually delays new messages when it fails.
	SyncStatus jsoncmd.SyncStatusType

	ctx    context.Context
	cancel context.CancelFunc

	views map[View]mauview.Component
}

func init() {
	mauview.Styles.PrimitiveBackgroundColor = tcell.ColorDefault
	mauview.Styles.PrimaryTextColor = tcell.ColorDefault
	mauview.Styles.BorderColor = tcell.ColorDefault
	mauview.Styles.ContrastBackgroundColor = ColorSelectionBackground
	// Rounded corners, and focused boxes keep the thin line (no double).
	mauview.Borders.TopLeft, mauview.Borders.TopRight = '╭', '╮'
	mauview.Borders.BottomLeft, mauview.Borders.BottomRight = '╰', '╯'
	mauview.Borders.HorizontalFocus = mauview.Borders.Horizontal
	mauview.Borders.VerticalFocus = mauview.Borders.Vertical
	mauview.Borders.TopLeftFocus = mauview.Borders.TopLeft
	mauview.Borders.TopRightFocus = mauview.Borders.TopRight
	mauview.Borders.BottomLeftFocus = mauview.Borders.BottomLeft
	mauview.Borders.BottomRightFocus = mauview.Borders.BottomRight
	if tcellDB := os.Getenv("TCELLDB"); len(tcellDB) == 0 {
		if info, err := os.Stat("/usr/share/tcell/database"); err == nil && info.IsDir() {
			_ = os.Setenv("TCELLDB", "/usr/share/tcell/database")
		}
	}
}

func New() *App {
	ctx, cancel := context.WithCancel(context.Background())
	ui := &App{
		app:    mauview.NewApplication(),
		Config: config.NewConfig(),
		ctx:    ctx,
		cancel: cancel,
	}
	debug.OnRecover = ui.app.ForceStop
	return ui
}

func (ui *App) Run() {
	ui.Config.LoadAll()
	// tuimessenger.yaml (yours, in dotfiles) wins over terminal.yaml.
	if problems := applyStyleFile(ui.Config); len(problems) > 0 {
		toasts.Show(toastWarning, styleFileName+": "+strings.Join(problems, "; "))
	}
	// The Ctrl+s sidebar toggle is session-only: launch follows the
	// "sidebar on start" setting (off by default), not the last toggle.
	ui.Config.Preferences.HideRoomList = !ui.Config.Preferences.SidebarOnStart
	log := exerrors.Must(ui.Config.LogConfig.Compile())
	exzerolog.SetupDefaults(log)
	if ui.Backend == nil {
		panic("tui: App.Backend must be set before Run")
	}
	ui.gmx = client.New(ui.Backend)

	mauview.Backspace2RemovesWord = ui.Config.Backspace2RemovesWord
	mauview.Backspace1RemovesWord = ui.Config.Backspace1RemovesWord
	// The buffered root starts every frame from a clean grid; clearing the
	// real screen as well would make every cell look changed to tcell again.
	ui.app.SetAlwaysClear(false)
	_ = clipboard.Initialize()
	ui.views = map[View]mauview.Component{
		ViewMain: ui.NewMainView(),
	}
	if ui.LoggedIn {
		ui.SetView(ViewMain)
	} else {
		ui.views[ViewSetup] = ui.NewSetupView()
		ui.inSetup.Store(true)
		ui.SetView(ViewSetup)
	}
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c
		go ui.Stop()
		<-c
		ui.Finish()
	}()

	go ui.Connect()
	exerrors.PanicIfNotNil(ui.app.Start())
}

func (ui *App) Connect() {
	ui.gmx.ReversedRoomList.Listen(func(rooms []*store.RoomListEntry) {
		ui.NeedsRender = true
		onRoomListChange(ui.gmx, rooms)
	})
	ui.gmx.SendNotification = ui.MainView.NotifyMessage
	toasts.render = ui.Render
	ui.gmx.OnNotice = func(evt *database.Event) bool {
		text := ""
		if evt.LocalContent != nil {
			text = noticeText(evt.LocalContent.SanitizedHTML)
		}
		if text == "" {
			text = evt.GetMautrixContent().AsMessage().Body
		}
		toasts.Show(classifyNotice(text), text)
		return true
	}
	ui.gmx.EventHandler = ui.eventHandler
	messages.RequestRender = ui.Render
	ui.MainView.matrix = ui.gmx
	err := ui.gmx.Connect(ui.ctx, ui.onConnStateChange)
	if err != nil && !errors.Is(err, context.Canceled) {
		exerrors.PanicIfNotNil(err)
	}
}

// Resync reconnects to the backend right away and asks for a full resync, which
// is the terminal equivalent of reloading the web client.
func (ui *App) Resync() {
	debug.Print("Manual resync requested")
	ui.Resyncing = true
	ui.Render()
	ui.gmx.Backend.Resync()
}

func (ui *App) onConnStateChange(state rpc.ConnState, err error) {
	debug.Printf("Connection state: %s (%v)", state, err)
	ui.ConnState = state
	// Render immediately: the localhost reconnect is over in milliseconds, so
	// waiting for the next sync would repaint after the state is already back
	// to connected and the indicator would never be visible.
	ui.Render()
}

// leaveSetup switches from the setup screen to the chats once there's a login; later calls do nothing.
func (ui *App) leaveSetup() {
	if ui.inSetup.CompareAndSwap(true, false) {
		ui.SetView(ViewMain)
		ui.Render()
	}
}

func (ui *App) eventHandler(ctx context.Context, rawEvt any) {
	switch evt := rawEvt.(type) {
	case *jsoncmd.ClientState:
		if evt.IsLoggedIn {
			ui.leaveSetup()
		}
	case *jsoncmd.SyncComplete:
		ui.reportSendFailures(evt)
		// Our own public reads from any device hide the Ctrl+m hint.
		for roomID, sr := range evt.Rooms {
			if len(sr.Receipts) > 0 {
				if room := ui.gmx.GetRoom(roomID); room != nil {
					notePublicReceipts(room, sr.Receipts)
				}
			}
		}
		if mv := ui.MainView; mv != nil {
			mv.autoRead(evt)
		}
		if ui.NeedsRender {
			debug.Print("Rendering...")
			ui.Render()
		}
	case *jsoncmd.InitComplete:
		if ui.Resyncing {
			ui.Resyncing = false
			ui.Render()
		}
	case *jsoncmd.SyncStatus:
		if ui.SyncStatus != evt.Type {
			ui.SyncStatus = evt.Type
			ui.Render()
		}
	case *jsoncmd.SendComplete:
		reportSendError(evt)
	case *jsoncmd.Typing:
		debug.Print("Typing in", evt.RoomID, evt.UserIDs)
		// Typing arrives outside sync batches, so nothing else repaints for
		// it. Only the open room's status line shows it; every bridged room
		// emits typing, so rendering for all of them would be constant churn.
		if mv := ui.MainView; mv != nil && mv.currentRoom != nil && mv.currentRoom.Room.ID == evt.RoomID {
			ui.Render()
		}
	}
}

func (ui *App) Stop() {
	debug.Print("Stopping")
	ui.cancel()
	ui.gmx.Backend.Disconnect()
	debug.Print("Disconnection complete")
	ui.app.Stop()
	debug.Print("Stopped")
	os.Exit(0)
}

func (ui *App) Finish() {
	ui.app.ForceStop()
	os.Exit(0)
}

func (ui *App) Render() {
	ui.app.Redraw()
	ui.NeedsRender = false
}

func (ui *App) HandleNewPreferences() {
	ui.Render()
}

func (ui *App) SetView(name View) {
	ui.app.SetRoot(newBufferedRoot(ui.views[name]))
}

func (ui *App) RunExternal(executablePath string, args ...string) error {
	callback := make(chan error)
	ui.app.Suspend(func() {
		cmd := exec.Command(executablePath, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		cmd.Env = os.Environ()
		callback <- cmd.Run()
	})
	return <-callback
}

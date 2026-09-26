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
	"syscall"

	"github.com/gdamore/tcell/v2"
	"github.com/zyedidia/clipboard"
	"go.mau.fi/mauview"
	"go.mau.fi/util/exerrors"
	"go.mau.fi/util/exzerolog"

	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
	"go.mau.fi/gomuks/pkg/rpc"
	"go.mau.fi/gomuks/pkg/rpc/client"
	"go.mau.fi/gomuks/pkg/rpc/store"
	"go.mau.fi/gomuks/tui/config"
	"go.mau.fi/gomuks/tui/debug"
	"go.mau.fi/gomuks/tui/messages"
)

type View string

// Allowed views in GomuksTUI
const (
	ViewLogin View = "login"
	ViewMain  View = "main"
)

type GomuksTUI struct {
	gmx *client.GomuksClient
	app *mauview.Application

	Config *config.Config

	MainView  *MainView
	LoginView *LoginView

	NeedsRender bool

	// ConnState is the current state of the connection to the gomuks backend.
	ConnState rpc.ConnState
	// Resyncing is true from a manual resync until the backend finishes
	// streaming the fresh initial data. The websocket reconnects in
	// milliseconds; this is the phase a human can actually see.
	Resyncing bool

	ctx    context.Context
	cancel context.CancelFunc

	views map[View]mauview.Component
}

func init() {
	mauview.Styles.PrimitiveBackgroundColor = tcell.ColorDefault
	mauview.Styles.PrimaryTextColor = tcell.ColorDefault
	mauview.Styles.BorderColor = tcell.ColorDefault
	mauview.Styles.ContrastBackgroundColor = ColorSelectionBackground
	if tcellDB := os.Getenv("TCELLDB"); len(tcellDB) == 0 {
		if info, err := os.Stat("/usr/share/tcell/database"); err == nil && info.IsDir() {
			_ = os.Setenv("TCELLDB", "/usr/share/tcell/database")
		}
	}
}

func NewGomuksTUI() *GomuksTUI {
	ctx, cancel := context.WithCancel(context.Background())
	ui := &GomuksTUI{
		app:    mauview.NewApplication(),
		Config: config.NewConfig(),
		ctx:    ctx,
		cancel: cancel,
	}
	debug.OnRecover = ui.app.ForceStop
	return ui
}

func (ui *GomuksTUI) Run() {
	ui.Config.LoadAll()
	// The Ctrl+s sidebar toggle is session-only: always start visible, even
	// though the toggled state gets persisted with the rest of the config.
	ui.Config.Preferences.HideRoomList = false
	log := exerrors.Must(ui.Config.LogConfig.Compile())
	exzerolog.SetupDefaults(log)
	loggedIn := false
	if ui.Config.Server != "" && ui.Config.Username != "" && ui.Config.Password != "" {
		ui.gmx = exerrors.Must(client.NewGomuksClient(ui.Config.Server))
		exerrors.PanicIfNotNil(ui.gmx.GomuksAPI.(*rpc.GomuksRPC).Authenticate(context.TODO(), ui.Config.Username, ui.Config.Password))
		loggedIn = true
	}

	mauview.Backspace2RemovesWord = ui.Config.Backspace2RemovesWord
	mauview.Backspace1RemovesWord = ui.Config.Backspace1RemovesWord
	ui.app.SetAlwaysClear(ui.Config.AlwaysClearScreen)
	_ = clipboard.Initialize()
	ui.views = map[View]mauview.Component{
		ViewLogin: ui.NewLoginView(),
		ViewMain:  ui.NewMainView(),
	}
	if loggedIn {
		ui.SetView(ViewMain)
	} else {
		ui.SetView(ViewLogin)
	}
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c
		go ui.Stop()
		<-c
		ui.Finish()
	}()

	if ui.gmx != nil {
		go ui.Connect()
	}
	exerrors.PanicIfNotNil(ui.app.Start())
}

func (ui *GomuksTUI) Connect() {
	ui.gmx.ReversedRoomList.Listen(func(_ []*store.RoomListEntry) {
		ui.NeedsRender = true
	})
	ui.gmx.SendNotification = ui.MainView.NotifyMessage
	ui.gmx.EventHandler = ui.gomuksEventHandler
	messages.RequestRender = ui.Render
	ui.MainView.matrix = ui.gmx
	rpcClient := ui.gmx.GomuksAPI.(*rpc.GomuksRPC)
	rpcClient.ConnStateHandler = ui.onConnStateChange
	err := rpcClient.ConnectWithRetry(ui.ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		exerrors.PanicIfNotNil(err)
	}
}

// Resync reconnects to the backend right away and asks for a full resync, which
// is the terminal equivalent of reloading the web client.
func (ui *GomuksTUI) Resync() {
	debug.Print("Manual resync requested")
	ui.Resyncing = true
	ui.Render()
	ui.gmx.GomuksAPI.(*rpc.GomuksRPC).Resync()
}

func (ui *GomuksTUI) onConnStateChange(state rpc.ConnState, err error) {
	debug.Printf("Connection state: %s (%v)", state, err)
	ui.ConnState = state
	// Render immediately: the localhost reconnect is over in milliseconds, so
	// waiting for the next sync would repaint after the state is already back
	// to connected and the indicator would never be visible.
	ui.Render()
}

func (ui *GomuksTUI) gomuksEventHandler(ctx context.Context, rawEvt any) {
	switch evt := rawEvt.(type) {
	case *jsoncmd.SyncComplete:
		if ui.NeedsRender {
			debug.Print("Rendering...")
			ui.Render()
		}
	case *jsoncmd.InitComplete:
		if ui.Resyncing {
			ui.Resyncing = false
			ui.Render()
		}
	case *jsoncmd.Typing:
		// Typing arrives outside sync batches, so nothing else repaints for
		// it. Only the open room's status line shows it; every bridged room
		// emits typing, so rendering for all of them would be constant churn.
		if mv := ui.MainView; mv != nil && mv.currentRoom != nil && mv.currentRoom.Room.ID == evt.RoomID {
			ui.Render()
		}
	}
}

func (ui *GomuksTUI) Stop() {
	debug.Print("Stopping")
	ui.cancel()
	ui.gmx.GomuksAPI.(*rpc.GomuksRPC).Disconnect()
	debug.Print("Disconnection complete")
	ui.app.Stop()
	debug.Print("Stopped")
	os.Exit(0)
}

func (ui *GomuksTUI) Finish() {
	ui.app.ForceStop()
	os.Exit(0)
}

func (ui *GomuksTUI) Render() {
	ui.app.Redraw()
	ui.NeedsRender = false
}

func (ui *GomuksTUI) OnLogin() {
	ui.SetView(ViewMain)
}

func (ui *GomuksTUI) OnLogout() {
	ui.SetView(ViewLogin)
}

func (ui *GomuksTUI) HandleNewPreferences() {
	ui.Render()
}

func (ui *GomuksTUI) SetView(name View) {
	ui.app.SetRoot(ui.views[name])
}

func (ui *GomuksTUI) RunExternal(executablePath string, args ...string) error {
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

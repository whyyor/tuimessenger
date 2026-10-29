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
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"go.mau.fi/mauview"
	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
	"go.mau.fi/gomuks/pkg/rpc"
	"go.mau.fi/gomuks/pkg/rpc/client"
	"go.mau.fi/gomuks/pkg/rpc/store"
	"go.mau.fi/gomuks/tui/config"
	"go.mau.fi/gomuks/tui/debug"
	"go.mau.fi/gomuks/tui/lib/emojistrip"
	"go.mau.fi/gomuks/tui/lib/notification"
	"go.mau.fi/gomuks/tui/widget"
)

type MainView struct {
	flex *mauview.Flex

	roomList    *RoomList
	roomView    *mauview.Box
	currentRoom *RoomView
	//cmdProcessor *CommandProcessor
	focused mauview.Focusable

	modal mauview.Component

	lastFocusTime time.Time
	// browsing is set while ↓/↑ flip through rooms; see handleBrowseKey.
	browsing bool
	// hintRows maps hint-screen unread rows to rooms for mouse clicks.
	hintRows []hintRow

	matrix *client.GomuksClient
	config *config.Config
	parent *GomuksTUI
}

func (ui *GomuksTUI) NewMainView() mauview.Component {
	mainView := &MainView{
		flex:     mauview.NewFlex().SetDirection(mauview.FlexColumn),
		roomView: mauview.NewBox(nil).SetBorder(false),

		matrix: ui.gmx,
		config: ui.Config,
		parent: ui,
	}
	mainView.roomList = NewRoomList(mainView)
	onFocusGained = func() { mainView.autoRead(nil) }
	go mainView.tickRelativeTimes()
	//mainView.cmdProcessor = NewCommandProcessor(mainView)

	mainView.flex.
		AddFixedComponent(mainView.roomList, 25).
		AddProportionalComponent(mainView.roomView, 1)
	mainView.BumpFocus(nil)

	ui.MainView = mainView

	return mainView
}

func (view *MainView) ShowModal(modal mauview.Component) {
	view.modal = modal
	var ok bool
	view.focused, ok = modal.(mauview.Focusable)
	if !ok {
		view.focused = nil
	} else {
		view.focused.Focus()
	}
}

func (view *MainView) HideModal() {
	view.modal = nil
	view.focused = view.roomView
}

func (view *MainView) Draw(screen mauview.Screen) {
	if view.config.Preferences.HideRoomList {
		if view.currentRoom == nil {
			// Sidebar hidden and nothing open leaves a fully blank screen;
			// hint at the way out instead of looking broken.
			view.drawEmptyHint(screen)
		} else {
			view.roomView.Draw(screen)
		}
	} else {
		view.flex.Draw(screen)
	}

	if view.modal != nil {
		view.modal.Draw(screen)
	}
	toasts.Draw(screen)
	view.drawConnDialog(screen)
}

// drawConnDialog overlays a small centered dialog while the connection is
// anything but healthy, or while a manual resync streams fresh data. Drawn
// last so it sits on top of modals too: connectivity trumps everything.
func (view *MainView) drawConnDialog(screen mauview.Screen) {
	var text string
	switch {
	case view.parent.ConnState != rpc.ConnStateConnected:
		text = view.parent.ConnState.String() + "…"
	case view.parent.SyncStatus == jsoncmd.SyncStatusErroring:
		// The daemon is connected to us but cannot sync with the homeserver;
		// this is what actually delays new messages, so the dialog stays up
		// until a sync succeeds.
		text = "sync erroring…"
	case view.parent.SyncStatus == jsoncmd.SyncStatusFailed:
		text = "sync failed"
	case view.parent.Resyncing:
		text = "resyncing…"
	default:
		return
	}
	width, height := screen.Size()
	boxWidth := runewidth.StringWidth(text) + 6
	boxX := (width - boxWidth) / 2
	boxY := height/2 - 1
	if boxX < 0 || boxY < 0 {
		return
	}
	// Default background so the box inherits the terminal's translucency;
	// the interior is still cleared so chat text doesn't bleed through.
	borderStyle := tcell.StyleDefault.Foreground(ColorBorder)
	textStyle := tcell.StyleDefault.Foreground(tcell.ColorYellow).Bold(true)
	for y := boxY; y < boxY+3; y++ {
		widget.WriteLinePadded(screen, mauview.AlignLeft, "", boxX, y, boxWidth, tcell.StyleDefault)
	}
	screen.SetContent(boxX, boxY, '╭', nil, borderStyle)
	screen.SetContent(boxX+boxWidth-1, boxY, '╮', nil, borderStyle)
	screen.SetContent(boxX, boxY+2, '╰', nil, borderStyle)
	screen.SetContent(boxX+boxWidth-1, boxY+2, '╯', nil, borderStyle)
	for x := boxX + 1; x < boxX+boxWidth-1; x++ {
		screen.SetContent(x, boxY, '─', nil, borderStyle)
		screen.SetContent(x, boxY+2, '─', nil, borderStyle)
	}
	screen.SetContent(boxX, boxY+1, '│', nil, borderStyle)
	screen.SetContent(boxX+boxWidth-1, boxY+1, '│', nil, borderStyle)
	widget.WriteLine(screen, mauview.AlignLeft, text, boxX+3, boxY+1, boxWidth-6, textStyle)
}

// hintMaxUnread caps the unread list on the hint screen.
const hintMaxUnread = 8

func (view *MainView) drawEmptyHint(screen mauview.Screen) {
	width, height := screen.Size()
	hintY := height / 2
	view.drawUnreadList(screen, width, hintY)
	hint := "↑↓ to browse chats · Ctrl+k to search · Ctrl+p for commands"
	switch n := len(view.hintRows); {
	case n == 1:
		hint = "1 to open · ↑↓ browse · Ctrl+k search · Ctrl+p commands"
	case n > 1:
		hint = fmt.Sprintf("1–%d to open · ↑↓ browse · Ctrl+k search · Ctrl+p commands", n)
	}
	x := max(0, (width-runewidth.StringWidth(hint))/2)
	widget.WriteLine(screen, mauview.AlignLeft, hint, x, hintY, width,
		tcell.StyleDefault.Foreground(ColorStatusText))
}

// hintRoomForKey maps a bare 1–9 to the matching unread row, or "".
func hintRoomForKey(rows []hintRow, event mauview.KeyEvent) id.RoomID {
	if event.Key() != tcell.KeyRune || event.Modifiers() != tcell.ModNone {
		return ""
	}
	idx := int(event.Rune() - '1')
	if idx < 0 || idx >= len(rows) || idx > 8 {
		return ""
	}
	return rows[idx].roomID
}

// drawUnreadList draws unread chats above the hint, newest at the top,
// redrawn on every sync like the sidebar. Rows are remembered for clicks.
func (view *MainView) drawUnreadList(screen mauview.Screen, width, hintY int) {
	view.hintRows = view.hintRows[:0]
	rooms := view.roomList.UnreadRooms()
	extra := 0
	if len(rooms) > hintMaxUnread {
		extra = len(rooms) - hintMaxUnread
		rooms = rooms[:hintMaxUnread]
	}
	rowCount := len(rooms)
	if extra > 0 {
		rowCount++
	}
	if rowCount == 0 {
		return
	}
	blockW := min(44, width-2)
	x := max(0, (width-blockW)/2)
	y := hintY - 1 - rowCount
	if y < 0 {
		return
	}
	dim := tcell.StyleDefault.Foreground(ColorStatusText)
	for i, room := range rooms {
		widget.WriteLine(screen, mauview.AlignLeft, strconv.Itoa(i+1), x, y, 1, dim)
		icon, iconColor := BridgeIconColor(room.Bridge)
		widget.WriteLine(screen, mauview.AlignLeft, icon, x+2, y, 2, tcell.StyleDefault.Foreground(iconColor))
		badge, badgeColor := "●", ColorUnreadBadge
		if room.UnreadMessages > 0 {
			badge = "99+"
			if room.UnreadMessages < 100 {
				badge = strconv.Itoa(room.UnreadMessages)
			}
		}
		if room.UnreadHighlights > 0 {
			badgeColor = ColorUnreadHighlight
		}
		badgeW := runewidth.StringWidth(badge)
		widget.WriteLine(screen, mauview.AlignLeft, badge, x+blockW-badgeW, y, badgeW,
			tcell.StyleDefault.Foreground(badgeColor).Bold(true))
		nameMax := blockW - 4 - badgeW - 1
		writeNameAlias(screen, x+4, y, nameMax, room.Name, view.showAlias(room.RoomID), tcell.StyleDefault.Bold(true), dim)
		view.hintRows = append(view.hintRows, hintRow{y: y, roomID: room.RoomID})
		y++
	}
	if extra > 0 {
		widget.WriteLine(screen, mauview.AlignLeft, fmt.Sprintf("+%d more", extra), x+4, y, blockW-4, dim)
	}
}

type hintRow struct {
	y      int
	roomID id.RoomID
}

func (view *MainView) BumpFocus(roomView *RoomView) {
	if roomView != nil {
		view.lastFocusTime = time.Now()
		view.MarkRead(roomView)
	}
}

func (view *MainView) MarkRead(roomView *RoomView) {
	if view.browsing {
		return
	}
	if roomView != nil && roomView == view.currentRoom && roomView.MessageView().GetScrollOffset() == 0 {
		ClearNotifications(roomView.Room.ID)
		req := roomView.Room.GetMarkAsReadParams()
		if req != nil && !sendsPublicRead(roomView) {
			// /seen off or peeking: no receipt at all, so a later Ctrl+m can still bridge one.
			go view.markSeenLocally(roomView.Room, req)
			return
		}
		if req != nil {
			req.ReceiptType = event.ReceiptTypeRead
		}
		if req != nil {
			go func() {
				defer debug.Recover()
				err := view.matrix.MarkRead(context.TODO(), req)
				if err != nil {
					debug.Print("Failed to mark read for", roomView.Room.ID, err)
				} else {
					notePublicReadOf(roomView.Room, req.EventID)
				}
			}()
		}
	}
}

func (view *MainView) InputChanged(roomView *RoomView, text string) {
	// Typing stays hidden unless this chat opted in (/typing on).
	if !chatPrivacy(roomView.Room).Typing {
		return
	}
	// Commands aren't worth announcing to the other side.
	roomView.SendTyping(len(text) > 0 && text[0] != '/')
}

func (view *MainView) ShowBare(roomView *RoomView) {
	if roomView == nil {
		return
	}
	// Loading history can take a few round trips; keep the UI responsive.
	go view.showBare(roomView)
}

// bareHistoryTarget is how many events Ctrl+L tries to have loaded.
const bareHistoryTarget = 500

// loadHistoryFor paginates until target events are loaded, the room's
// history (or its /clear point) is reached, or a page fails.
func (view *MainView) loadHistoryFor(room *store.RoomStore, target int) {
	for range 20 {
		tl := room.TimelineCache.Current()
		if tl != nil && (len(*tl) >= target || historyExhausted(*tl, clearedAt(room))) {
			return
		}
		if !room.HasMoreHistory() {
			return
		}
		if room.Paginating.Load() {
			// Another load (scroll, fill-screen) is running; let it finish.
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if err := view.matrix.LoadMoreHistory(context.TODO(), room.ID); err != nil {
			debug.Print("Failed to load history for bare view:", err)
			return
		}
	}
}

func (view *MainView) showBare(roomView *RoomView) {
	defer debug.Recover()
	view.loadHistoryFor(roomView.Room, bareHistoryTarget)
	text := roomView.MessageView().CapturePlaintext()
	view.parent.app.Suspend(func() {
		print("\033[2J\033[0;0H")
		less, version := findLess()
		if less == "" {
			fmt.Println(text)
			fmt.Println("Press enter to return to normal mode.")
			_, _, _ = bufio.NewReader(os.Stdin).ReadRune()
		} else {
			// Every loaded message, opened at the newest; less reads keys from
			// the tty. --mouse scrolls with the wheel (Shift+drag selects).
			cmd := exec.Command(less, lessArgs(version)...)
			cmd.Stdin = strings.NewReader(text)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			_ = cmd.Run()
		}
		print("\033[2J\033[0;0H")
	})
	RequestRepaint()
	view.parent.Render()
}

func (view *MainView) OpenSyncingModal() *SyncingModal {
	component, modal := NewSyncingModal(view)
	view.ShowModal(component)
	return modal
}

func (view *MainView) OnKeyEvent(event mauview.KeyEvent) bool {
	userAttention.touch()
	// Before anything else: a key normally marks the open chat read.
	if view.modal == nil && view.config.Keybindings.Main[config.Keybind{
		Key: event.Key(), Ch: event.Rune(), Mod: event.Modifiers(),
	}] == "mark_unread" {
		view.MarkUnread()
		return true
	}
	if view.modal == nil && view.currentRoom == nil && view.config.Preferences.HideRoomList {
		if roomID := hintRoomForKey(view.hintRows, event); roomID != "" {
			view.SwitchRoom(roomID)
			return true
		}
	}
	if view.modal == nil && (view.browsing || view.currentRoom == nil) {
		if view.handleBrowseKey(event) {
			return true
		}
		if view.browsing {
			view.browsing = false
			if event.Key() == tcell.KeyEscape {
				// Leave without marking the room read.
				view.CloseCurrentRoom()
				return true
			}
			// Any other key settles on this room: BumpFocus below marks it
			// read, then the key is handled normally (e.g. typed).
		}
	}
	view.BumpFocus(view.currentRoom)

	if view.modal != nil {
		return view.modal.OnKeyEvent(event)
	}

	kb := config.Keybind{
		Key: event.Key(),
		Ch:  event.Rune(),
		Mod: event.Modifiers(),
	}
	switch view.config.Keybindings.Main[kb] {
	case "next_room":
		view.SwitchRoom(view.roomList.Next())
	case "prev_room":
		view.SwitchRoom(view.roomList.Previous())
	case "search_rooms":
		view.ShowModal(NewFuzzySearchModal(view, 42, 12))
	case "scroll_up":
		msgView := view.currentRoom.MessageView()
		msgView.AddScrollOffset(msgView.TotalHeight())
	case "scroll_down":
		msgView := view.currentRoom.MessageView()
		msgView.AddScrollOffset(-msgView.TotalHeight())
	case "add_newline":
		return view.flex.OnKeyEvent(tcell.NewEventKey(tcell.KeyEnter, '\n', event.Modifiers()|tcell.ModShift))
	case "next_active_room":
		view.SwitchRoom(view.roomList.NextWithActivity())
	case "show_bare":
		view.ShowBare(view.currentRoom)
	case "resync":
		// Disconnect writes a close frame, so keep it off the render thread.
		go view.parent.Resync()
	case "toggle_sidebar":
		view.toggleSidebar()
	case "toggle_members":
		view.config.Preferences.ShowUserList = !view.config.Preferences.ShowUserList
		RequestRepaint()
	case "command_palette":
		view.ShowModal(NewCommandPalette(view))
	case "force_quit":
		// Like a shell: Ctrl+C first clears typed text (Ctrl+Z restores it),
		// and only quits from an empty input.
		if view.currentRoom != nil && view.currentRoom.input.GetText() != "" {
			// Clear resets cursor, selection and scroll; SetText then records
			// the undo snapshot and fires the change callback (typing, draft).
			view.currentRoom.input.Clear()
			view.currentRoom.input.SetText("")
			return true
		}
		view.parent.Finish()
		return false
	case "quit":
		view.parent.Stop()
		return false
	default:
		if view.config.Preferences.HideRoomList {
			return view.roomView.OnKeyEvent(event)
		}
		return view.flex.OnKeyEvent(event)
	}
	return true
}

const WheelScrollOffsetDiff = 3

func (view *MainView) OnMouseEvent(event mauview.MouseEvent) bool {
	if !event.HasMotion() {
		userAttention.touch()
	}
	if x, y := event.Position(); event.Buttons() == tcell.Button1 && !event.HasMotion() && toasts.Hit(x, y) {
		toasts.Dismiss()
		return true
	}
	if !event.HasMotion() {
		view.browsing = false
	}
	view.BumpFocus(view.currentRoom)
	if view.modal != nil {
		return view.modal.OnMouseEvent(event)
	}
	if view.config.Preferences.HideRoomList {
		if view.currentRoom == nil && event.Buttons() == tcell.Button1 && !event.HasMotion() {
			// A click on an unread row of the hint screen opens that chat.
			_, y := event.Position()
			for _, row := range view.hintRows {
				if row.y == y {
					view.SwitchRoom(row.roomID)
					return true
				}
			}
		}
		return view.roomView.OnMouseEvent(event)
	}
	return view.flex.OnMouseEvent(event)
}

func (view *MainView) OnPasteEvent(event mauview.PasteEvent) bool {
	if view.browsing {
		view.browsing = false
		view.MarkRead(view.currentRoom)
	}
	if view.modal != nil {
		return view.modal.OnPasteEvent(event)
	} else if view.config.Preferences.HideRoomList {
		return view.roomView.OnPasteEvent(event)
	}
	return view.flex.OnPasteEvent(event)
}

func (view *MainView) Focus() {
	if view.focused != nil {
		view.focused.Focus()
	}
}

func (view *MainView) Blur() {
	if view.focused != nil {
		view.focused.Blur()
	}
}

// handleBrowseKey opens the next or previous room on ↓ and ↑, starting
// from the newest chat when no room is open. Rooms passed through aren't
// marked read: browsing gates MarkRead until a different key settles.
func (view *MainView) handleBrowseKey(event mauview.KeyEvent) bool {
	if event.Modifiers() != tcell.ModNone {
		return false
	}
	var down bool
	switch event.Key() {
	case tcell.KeyDown:
		down = true
	case tcell.KeyUp:
		down = false
	default:
		return false
	}
	view.browsing = true
	if next := view.roomList.BrowseStep(down); next != "" && (view.currentRoom == nil || next != view.currentRoom.Room.ID) {
		view.SwitchRoom(next)
	}
	view.parent.Render()
	return true
}

// tickRelativeTimes redraws so "now" becomes "1m", "59m" becomes "1h"…
func (view *MainView) tickRelativeTimes() {
	for range time.Tick(30 * time.Second) {
		prefs := view.config.Preferences
		if view.currentRoom != nil && !prefs.ClassicLayout && prefs.TimestampStyle == "" {
			view.parent.Render()
		}
	}
}

func (view *MainView) toggleSidebar() {
	view.config.Preferences.HideRoomList = !view.config.Preferences.HideRoomList
	// The topic bar carries the room name while the sidebar is hidden.
	if view.currentRoom != nil {
		view.currentRoom.Update(view.currentRoom.Room.Meta.Current())
	}
	RequestRepaint()
}

// CloseCurrentRoom returns to the launch state: sidebar only, no room open.
func (view *MainView) CloseCurrentRoom() {
	if view.currentRoom == nil {
		return
	}
	StopPlayback()
	view.currentRoom.Blur()
	view.currentRoom.Unload()
	view.currentRoom = nil
	view.roomView.SetInnerComponent(nil)
	view.roomList.SetSelected("")
	// Every way of closing (Escape, browse Escape) wipes leftover cells.
	RequestRepaint()
	view.parent.Render()
}

func (view *MainView) SwitchRoom(roomID id.RoomID) {
	view.switchRoom(roomID, false)
}

// PeekRoom opens a chat without sending seen for this visit; Ctrl+m still
// can. Switching away ends it, as every visit gets a fresh RoomView.
func (view *MainView) PeekRoom(roomID id.RoomID) {
	view.switchRoom(roomID, true)
}

func (view *MainView) switchRoom(roomID id.RoomID, peek bool) {
	roomData := view.matrix.GetRoom(roomID)
	if roomData == nil {
		debug.Print("Tried to switch to nonexistent room!", roomID)
		return
	}
	debug.Print("Selecting room", roomID)
	// Voice notes belong to the room they were played from.
	StopPlayback()
	view.roomList.SetSelected(roomID)
	// Safety net: wipe anything a width mismatch left behind in the old room.
	RequestRepaint()
	view.flex.SetFocused(view.roomView)
	if view.currentRoom != nil {
		view.currentRoom.Unload()
	}
	currentRoom := NewRoomView(view, roomData)
	currentRoom.peek = peek
	view.currentRoom = currentRoom
	view.roomView.SetInnerComponent(currentRoom)
	view.roomView.Focus()
	view.MarkRead(currentRoom)
	go view.loadReceipts(roomData)
	if len(ptr.Val(roomData.TimelineCache.Current())) < 50 {
		go view.LoadHistory(roomID)
	}
	if !roomData.FullMembersLoaded.Load() {
		// TODO only load necessary members rather than all?
		go func() {
			defer debug.Recover()
			err := view.matrix.LoadRoomState(context.TODO(), roomID, true, false)
			if err != nil {
				debug.Print("Failed to load room state for", roomID, err)
			} else {
				currentRoom.UpdateUserList()
				view.parent.Render()
			}
		}()
	}
	view.parent.Render()
}

func (view *MainView) NotifyMessage(room *store.RoomStore, notif jsoncmd.SyncNotification) {
	if view.config.Preferences.DisableNotifications {
		return
	}
	body := notif.Event.GetMautrixContent().AsMessage().Body
	if len(body) == 0 {
		debug.Print("Not sending notification with empty body")
		return
	}
	if len(body) > 400 {
		body = body[:350] + " […]"
	}
	memberEvt := room.GetMember(notif.Event.Sender)
	notifTitle := notif.Event.Sender.Localpart()
	if memberEvt != nil && memberEvt.Displayname != "" {
		if stripped := emojistrip.Strip(memberEvt.Displayname); stripped != "" {
			notifTitle = stripped
		}
	}
	if roomName := room.Meta.Current().Name; roomName != nil && *roomName != "" {
		if stripped := emojistrip.Strip(*roomName); stripped != "" && stripped != notifTitle {
			notifTitle = fmt.Sprintf("%s (%s)", notifTitle, stripped)
		}
	}
	tag := notificationTag(room.ID, notif.Event.ID)
	err := notification.SendTagged(room.BridgeID(), tag, notifTitle, body, notif.Highlight, notif.Sound)
	if err != nil {
		debug.Print("Failed to send notification:", err)
	} else {
		trackNotification(room.ID, room.BridgeID(), tag)
		debug.Print("Sent notification:", notifTitle, body)
	}
}

func (view *MainView) LoadHistory(roomID id.RoomID) {
	defer debug.Recover()
	// Every pagination path (open, scroll, fill-the-screen) comes through here.
	if room := view.matrix.GetRoom(roomID); room != nil {
		if tl := room.TimelineCache.Current(); tl != nil && historyExhausted(*tl, clearedAt(room)) {
			return
		}
	}
	err := view.matrix.LoadMoreHistory(context.TODO(), roomID)
	if err != nil {
		debug.Print("Failed to fetch history for", roomID, err)
		return
	}
	view.parent.Render()
	if room := view.currentRoom; room != nil && room.Room.ID == roomID {
		view.MarkRead(room)
	}
}

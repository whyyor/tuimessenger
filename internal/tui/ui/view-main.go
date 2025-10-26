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

package ui

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/mauview"
	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/rpc/client"
	"go.mau.fi/gomuks/tui/config"
	"go.mau.fi/gomuks/tui/debug"
	"go.mau.fi/gomuks/tui/ui/widget"
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
	//mainView.cmdProcessor = NewCommandProcessor(mainView)

	mainView.flex.
		AddFixedComponent(mainView.roomList, 25).
		AddFixedComponent(widget.NewBorder(), 1).
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
		view.roomView.Draw(screen)
	} else {
		view.flex.Draw(screen)
	}

	if view.modal != nil {
		view.modal.Draw(screen)
	}
}

func (view *MainView) BumpFocus(roomView *RoomView) {
	if roomView != nil {
		view.lastFocusTime = time.Now()
		view.MarkRead(roomView)
	}
}

func (view *MainView) MarkRead(roomView *RoomView) {
	//if roomView != nil && roomView.Room.HasNewMessages() && roomView.MessageView().GetScrollOffset() == 0 {
	//	msgList := roomView.MessageView().messages
	//	if len(msgList) > 0 {
	//		msg := msgList[len(msgList)-1]
	//		if roomView.Room.MarkRead(msg.EventID) {
	//			view.matrix.MarkRead(roomView.Room.ID, msg.EventID)
	//		}
	//	}
	//}
}

func (view *MainView) InputChanged(roomView *RoomView, text string) {
	//if !roomView.config.Preferences.DisableTypingNotifs {
	//	view.matrix.SendTyping(roomView.Room.ID, len(text) > 0 && text[0] != '/')
	//}
}

func (view *MainView) ShowBare(roomView *RoomView) {
	if roomView == nil {
		return
	}
	_, height := view.parent.app.Screen().Size()
	view.parent.app.Suspend(func() {
		print("\033[2J\033[0;0H")
		// We don't know how much space there exactly is. Too few messages looks weird,
		// and too many messages shouldn't cause any problems, so we just show too many.
		height *= 2
		fmt.Println(roomView.MessageView().CapturePlaintext(height))
		fmt.Println("Press enter to return to normal mode.")
		reader := bufio.NewReader(os.Stdin)
		_, _, _ = reader.ReadRune()
		print("\033[2J\033[0;0H")
	})
}

func (view *MainView) OpenSyncingModal() *SyncingModal {
	component, modal := NewSyncingModal(view)
	view.ShowModal(component)
	return modal
}

func (view *MainView) OnKeyEvent(event mauview.KeyEvent) bool {
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
	case "force_quit":
		view.parent.Finish()
		return false
	case "quit":
		view.parent.Stop()
		return false
	default:
		goto defaultHandler
	}
	return true
defaultHandler:
	if view.config.Preferences.HideRoomList {
		return view.roomView.OnKeyEvent(event)
	}
	return view.flex.OnKeyEvent(event)
}

const WheelScrollOffsetDiff = 3

func (view *MainView) OnMouseEvent(event mauview.MouseEvent) bool {
	if view.modal != nil {
		return view.modal.OnMouseEvent(event)
	}
	if view.config.Preferences.HideRoomList {
		return view.roomView.OnMouseEvent(event)
	}
	return view.flex.OnMouseEvent(event)
}

func (view *MainView) OnPasteEvent(event mauview.PasteEvent) bool {
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

func (view *MainView) SwitchRoom(roomID id.RoomID) {
	roomData := view.matrix.GetRoom(roomID)
	if roomData == nil {
		debug.Print("Tried to switch to nonexistent room!", roomID)
		return
	}
	debug.Print("Selecting room", roomID)
	view.roomList.SetSelected(roomID)
	view.flex.SetFocused(view.roomView)
	view.currentRoom = NewRoomView(view, roomData)
	view.roomView.SetInnerComponent(view.currentRoom)
	view.roomView.Focus()
	if len(ptr.Val(roomData.TimelineCache.Current())) < 50 {
		go view.LoadHistory(roomID)
	}
	debug.Print("Finished setting selected")
	view.parent.Render()
	debug.Print("Finished rendering after selecting")
}

func (view *MainView) SetTyping(roomID id.RoomID, users []id.UserID) {
	//roomView, ok := view.getRoomView(roomID, true)
	//if ok {
	//	roomView.SetTyping(users)
	//	view.parent.Render()
	//}
}

//func (view *MainView) NotifyMessage(room *rooms.Room, message ifc2.Message, should pushrules.PushActionArrayShould) {
//	view.Bump(room)
//	uiMsg, ok := message.(*messages.UIMessage)
//	if ok && uiMsg.SenderID == view.config.UserID {
//		return
//	}
//	// Whether or not the room where the message came is the currently shown room.
//	isCurrent := room == view.roomList.SelectedRoom()
//	// Whether or not the terminal window is focused.
//	recentlyFocused := time.Now().Add(-30 * time.Second).Before(view.lastFocusTime)
//	isFocused := time.Now().Add(-5 * time.Second).Before(view.lastFocusTime)
//
//	if !isCurrent || !isFocused {
//		// The message is not in the current room, show new message status in room list.
//		room.AddUnread(message.ID(), should.Notify, should.Highlight)
//	} else {
//		view.matrix.MarkRead(room.ID, message.ID())
//	}
//
//	if should.Notify && !recentlyFocused && !view.config.Preferences.DisableNotifications {
//		// Push rules say notify and the terminal is not focused, send desktop notification.
//		shouldPlaySound := should.PlaySound &&
//			should.SoundName == "default" &&
//			view.config.NotifySound
//		sendNotification(room, message.NotificationSenderName(), message.NotificationContent(), should.Highlight, shouldPlaySound)
//	}
//
//	// TODO this should probably happen somewhere else
//	//      (actually it's probably completely broken now)
//	message.SetIsHighlight(should.Highlight)
//}

func (view *MainView) LoadHistory(roomID id.RoomID) {
	defer debug.Recover()
	//roomView, ok := view.getRoomView(roomID, true)
	//if !ok {
	//	return
	//}
	//msgView := roomView.MessageView()

	//if !atomic.CompareAndSwapInt32(&msgView.loadingMessages, 0, 1) {
	//	// Locked
	//	return
	//}
	//defer atomic.StoreInt32(&msgView.loadingMessages, 0)
	//Update the "Loading more messages..." text
	//view.parent.Render()

	err := view.matrix.LoadMoreHistory(context.TODO(), roomID)
	if err != nil {
		debug.Print("Failed to fetch history for", roomID, err)
		//view.parent.Render()
		return
	}
	view.parent.Render()
}

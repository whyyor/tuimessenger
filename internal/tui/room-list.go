// gomuks - A terminal Matrix client written in Go.
// Copyright (C) 2020 Tulir Asokan
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
	"fmt"
	"slices"
	"strconv"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"go.mau.fi/mauview"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/rpc"
	"go.mau.fi/gomuks/pkg/rpc/store"
	"go.mau.fi/gomuks/tui/widget"
)

type RoomList struct {
	lock sync.RWMutex

	parent *MainView

	rooms    []*store.RoomListEntry
	selected id.RoomID

	scrollOffset int
	height       int
	width        int

	// The item main text color.
	mainTextColor tcell.Color
	// The text color for selected items.
	selectedTextColor tcell.Color
	// The background color for selected items.
	selectedBackgroundColor tcell.Color
}

func NewRoomList(parent *MainView) *RoomList {
	list := &RoomList{
		parent: parent,

		scrollOffset: 0,

		mainTextColor:           tcell.ColorDefault,
		selectedTextColor:       ColorSelectionText,
		selectedBackgroundColor: ColorSelectionBackground,
	}
	return list
}

func (list *RoomList) SetSelected(roomID id.RoomID) {
	list.selected = roomID
	pos := list.index(roomID)
	if pos <= list.scrollOffset {
		list.scrollOffset = pos - 1
	} else if pos >= list.scrollOffset+list.height {
		list.scrollOffset = pos - list.height + 1
	}
	if list.scrollOffset < 0 {
		list.scrollOffset = 0
	}
}

func (list *RoomList) HasSelected() bool {
	return list.selected != ""
}

func (list *RoomList) SelectedRoom() id.RoomID {
	return list.selected
}

func (list *RoomList) Previous() id.RoomID {
	list.lock.RLock()
	defer list.lock.RUnlock()
	idx := list.index(list.selected)
	if idx > 0 && idx < len(list.rooms) {
		return list.rooms[idx-1].RoomID
	}
	return ""
}

func (list *RoomList) Next() id.RoomID {
	list.lock.RLock()
	defer list.lock.RUnlock()
	if len(list.rooms) == 0 {
		return ""
	}
	if list.selected == "" {
		return list.rooms[0].RoomID
	}
	idx := list.index(list.selected)
	if idx >= 0 && idx < len(list.rooms)-1 {
		return list.rooms[idx+1].RoomID
	}
	return ""
}

func (list *RoomList) NextWithActivity() id.RoomID {
	list.lock.RLock()
	defer list.lock.RUnlock()
	for _, room := range list.rooms {
		if room.UnreadHighlights > 0 || room.UnreadMessages > 0 || room.MarkedUnread {
			return room.RoomID
		}
	}
	return ""
}

func (list *RoomList) index(roomID id.RoomID) int {
	return slices.IndexFunc(list.rooms, func(entry *store.RoomListEntry) bool {
		return entry.RoomID == roomID
	})
}

func (list *RoomList) OnKeyEvent(_ mauview.KeyEvent) bool {
	return false
}

func (list *RoomList) OnPasteEvent(_ mauview.PasteEvent) bool {
	return false
}

func (list *RoomList) OnMouseEvent(event mauview.MouseEvent) bool {
	if event.HasMotion() {
		return false
	}
	switch event.Buttons() {
	case tcell.WheelUp:
		list.addScrollOffset(-WheelScrollOffsetDiff)
		return true
	case tcell.WheelDown:
		list.addScrollOffset(WheelScrollOffsetDiff)
		return true
	case tcell.Button1:
		_, y := event.Position()
		list.lock.RLock()
		defer list.lock.RUnlock()
		y += list.scrollOffset
		if y < 0 || y > len(list.rooms) {
			return false
		}
		list.parent.SwitchRoom(list.rooms[y].RoomID)
		return true
	}
	return false
}

func (list *RoomList) addScrollOffset(offset int) {
	list.scrollOffset += offset
	if list.scrollOffset > len(list.rooms)-list.height {
		list.scrollOffset = len(list.rooms) - list.height
	}
	if list.scrollOffset < 0 {
		list.scrollOffset = 0
	}
}

func (list *RoomList) Focus() {}
func (list *RoomList) Blur()  {}

func (list *RoomList) Draw(screen mauview.Screen) {
	list.lock.Lock()
	list.rooms = list.parent.matrix.ReversedRoomList.Current()
	list.width, list.height = screen.Size()
	// A healthy connection stays out of the way; anything else claims the bottom
	// row so sync trouble (and the result of a manual resync) is visible.
	connState := list.parent.parent.ConnState
	statusText := ""
	if connState != rpc.ConnStateConnected {
		statusText = connState.String()
	} else if list.parent.parent.Resyncing {
		statusText = "resyncing"
	}
	listHeight := list.height
	if statusText != "" {
		listHeight--
	}
	roomSlice := list.rooms[min(len(list.rooms), list.scrollOffset):min(len(list.rooms), list.scrollOffset+listHeight)]
	list.lock.Unlock()

	for y, room := range roomSlice {
		unread := room.MarkedUnread || room.UnreadNotifications > 0 || room.UnreadHighlights > 0
		isSelected := room.RoomID == list.selected
		rowStyle := tcell.StyleDefault.
			Foreground(list.mainTextColor).
			Bold(unread)
		if isSelected {
			rowStyle = rowStyle.
				Foreground(list.selectedTextColor).
				Background(list.selectedBackgroundColor)
		}
		// Paint the full row first so selection and badges share one background.
		widget.WriteLinePadded(screen, mauview.AlignLeft, "", 0, y, list.width, rowStyle)

		icon, iconColor := BridgeIconColor(room.Bridge)
		iconStyle := rowStyle.Foreground(iconColor)
		if isSelected {
			// Brand colors clash on the red selection bar; inherit its text color.
			iconStyle = rowStyle
		}
		widget.WriteLine(screen, mauview.AlignLeft, icon, 1, y, 2, iconStyle)

		// Reserve space on the right for the unread badge before truncating.
		badge := ""
		badgeStyle := rowStyle
		if room.UnreadMessages > 0 {
			badge = "99+"
			if room.UnreadMessages < 100 {
				badge = strconv.Itoa(room.UnreadMessages)
			}
			badgeStyle = rowStyle.Bold(true)
			if !isSelected {
				badgeColor := ColorUnreadBadge
				if room.UnreadHighlights > 0 {
					badgeColor = ColorUnreadHighlight
				}
				badgeStyle = badgeStyle.Foreground(badgeColor)
			}
		} else if room.MarkedUnread {
			badge = "●"
			if !isSelected {
				badgeStyle = rowStyle.Foreground(ColorUnreadBadge)
			}
		}

		nameMax := list.width - 3 - 1
		if badge != "" {
			badgeWidth := runewidth.StringWidth(badge)
			nameMax -= badgeWidth + 1
			widget.WriteLine(screen, mauview.AlignLeft, badge, list.width-badgeWidth-1, y, badgeWidth, badgeStyle)
		}
		widget.WriteLine(screen, mauview.AlignLeft, runewidth.Truncate(room.Name, nameMax, "…"), 3, y, nameMax, rowStyle)
	}

	if statusText != "" && list.height > 0 {
		widget.WriteLinePadded(screen, mauview.AlignLeft,
			fmt.Sprintf(" %s…", statusText), 0, list.height-1, list.width,
			tcell.StyleDefault.Foreground(tcell.ColorYellow).Bold(true))
	}
}

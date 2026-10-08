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

	"go.mau.fi/gomuks/pkg/rpc/store"
	"go.mau.fi/gomuks/tui/widget"
)

type RoomList struct {
	lock sync.RWMutex

	parent *MainView

	// rooms is what's drawn, one entry per row; a nil entry is the Archive
	// header. See buildRoomRows.
	rooms         []*store.RoomListEntry
	archivedCount int
	showArchive   bool
	selected      id.RoomID

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
	for i := list.index(list.selected) - 1; i >= 0; i-- {
		if list.rooms[i] != nil {
			return list.rooms[i].RoomID
		}
	}
	return ""
}

func (list *RoomList) Next() id.RoomID {
	list.lock.RLock()
	defer list.lock.RUnlock()
	// index is -1 with nothing selected, so this starts at the top.
	for i := list.index(list.selected) + 1; i >= 0 && i < len(list.rooms); i++ {
		if list.rooms[i] != nil {
			return list.rooms[i].RoomID
		}
	}
	return ""
}

// NextWithActivity skips archived chats: they're junk by definition.
func (list *RoomList) NextWithActivity() id.RoomID {
	list.lock.RLock()
	defer list.lock.RUnlock()
	for _, room := range list.rooms {
		if room != nil && !room.LowPriority &&
			(room.UnreadHighlights > 0 || room.UnreadMessages > 0 || room.MarkedUnread) {
			return room.RoomID
		}
	}
	return ""
}

func (list *RoomList) ToggleArchive() {
	list.lock.Lock()
	list.showArchive = !list.showArchive
	list.lock.Unlock()
}

func (list *RoomList) index(roomID id.RoomID) int {
	return slices.IndexFunc(list.rooms, func(entry *store.RoomListEntry) bool {
		return entry != nil && entry.RoomID == roomID
	})
}

// buildRoomRows lays out the sidebar: active chats newest first, then an
// Archive header (nil) when any chat is archived, then the archived chats if
// expanded. A collapsed archive still shows the open chat, so it stays
// visible and selectable.
func buildRoomRows(rooms []*store.RoomListEntry, expanded bool, selected id.RoomID) (rows []*store.RoomListEntry, archived int) {
	rows = make([]*store.RoomListEntry, 0, len(rooms)+1)
	var archivedRooms []*store.RoomListEntry
	for _, room := range rooms {
		if room.LowPriority {
			archivedRooms = append(archivedRooms, room)
		} else {
			rows = append(rows, room)
		}
	}
	if len(archivedRooms) == 0 {
		return rows, 0
	}
	rows = append(rows, nil)
	for _, room := range archivedRooms {
		if expanded || room.RoomID == selected {
			rows = append(rows, room)
		}
	}
	return rows, len(archivedRooms)
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
		y += list.scrollOffset
		if y < 0 || y >= len(list.rooms) {
			list.lock.RUnlock()
			return false
		}
		row := list.rooms[y]
		list.lock.RUnlock()
		if row == nil {
			list.ToggleArchive()
		} else {
			list.parent.SwitchRoom(row.RoomID)
		}
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
	list.rooms, list.archivedCount = buildRoomRows(list.parent.matrix.ReversedRoomList.Current(), list.showArchive, list.selected)
	list.width, list.height = screen.Size()
	roomSlice := list.rooms[min(len(list.rooms), list.scrollOffset):min(len(list.rooms), list.scrollOffset+list.height)]
	archivedCount, showArchive := list.archivedCount, list.showArchive
	list.lock.Unlock()

	dim := tcell.StyleDefault.Foreground(tcell.ColorGray)
	for y, room := range roomSlice {
		if room == nil {
			arrow := "▸"
			if showArchive {
				arrow = "▾"
			}
			widget.WriteLinePadded(screen, mauview.AlignLeft, fmt.Sprintf(" %s Archive (%d)", arrow, archivedCount), 0, y, list.width, dim)
			continue
		}
		// Archived chats are muted: no bold, no red badge.
		unread := !room.LowPriority && (room.MarkedUnread || room.UnreadNotifications > 0 || room.UnreadHighlights > 0)
		isSelected := room.RoomID == list.selected
		rowStyle := tcell.StyleDefault.
			Foreground(list.mainTextColor).
			Bold(unread)
		if room.LowPriority {
			rowStyle = dim
		}
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
			badgeStyle = rowStyle.Bold(!room.LowPriority)
			if !isSelected && !room.LowPriority {
				badgeColor := ColorUnreadBadge
				if room.UnreadHighlights > 0 {
					badgeColor = ColorUnreadHighlight
				}
				badgeStyle = badgeStyle.Foreground(badgeColor)
			}
		} else if room.MarkedUnread {
			badge = "●"
			if !isSelected && !room.LowPriority {
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

}

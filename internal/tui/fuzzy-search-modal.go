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
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/mattn/go-runewidth"
	"go.mau.fi/mauview"

	"go.mau.fi/gomuks/pkg/rpc/store"
	"go.mau.fi/gomuks/tui/config"
	"go.mau.fi/gomuks/tui/debug"
	"go.mau.fi/gomuks/tui/widget"
)

type FuzzySearchModal struct {
	mauview.Component

	container *mauview.Box

	search  *mauview.InputArea
	results *fuzzyResultsView

	matches      fuzzy.Ranks
	selected     int
	scrollOffset int

	roomList    []*store.RoomListEntry
	roomTitles  []string
	roomLower   []string
	roomRecency []time.Time

	parent *MainView
}

func NewFuzzySearchModal(mainView *MainView, width int, height int) *FuzzySearchModal {
	fs := &FuzzySearchModal{
		parent:   mainView,
		roomList: mainView.matrix.ReversedRoomList.Current(),
	}
	fs.roomTitles = make([]string, len(fs.roomList))
	fs.roomLower = make([]string, len(fs.roomList))
	fs.roomRecency = make([]time.Time, len(fs.roomList))
	for i, room := range fs.roomList {
		fs.roomTitles[i] = room.Name
		fs.roomLower[i] = strings.ToLower(room.Name)
		fs.roomRecency[i] = room.SortingTimestamp
	}

	fs.results = &fuzzyResultsView{parent: fs}
	fs.search = mauview.NewInputArea().
		SetChangedFunc(fs.changeHandler).
		SetTextColor(tcell.ColorDefault).
		SetBackgroundColor(ColorBarBackground).
		SetPlaceholder(" Search rooms...").
		SetPlaceholderTextColor(ColorStatusText)
	fs.search.Focus()

	flex := mauview.NewFlex().
		SetDirection(mauview.FlexRow).
		AddFixedComponent(fs.search, 1).
		AddProportionalComponent(fs.results, 1)

	fs.container = mauview.NewBox(flex).
		SetBorder(true).
		SetBorderStyle(tcell.StyleDefault.Foreground(ColorBorder)).
		SetTitle("Quick Room Switcher").
		SetBlurCaptureFunc(func() bool {
			fs.parent.HideModal()
			return true
		})

	fs.Component = mauview.Center(fs.container, width, height).SetAlwaysFocusChild(true)

	// SetChangedFunc only fires on edits, so seed the initial full list.
	fs.changeHandler("")

	return fs
}

func (fs *FuzzySearchModal) Focus() {
	fs.container.Focus()
}

func (fs *FuzzySearchModal) Blur() {
	fs.container.Blur()
}

func (fs *FuzzySearchModal) changeHandler(str string) {
	// An empty query lists every room in room-list (recency) order. Ranking would
	// destroy that ordering, so only sort when there is an actual query.
	if str == "" {
		fs.matches = make(fuzzy.Ranks, len(fs.roomTitles))
		for i, title := range fs.roomTitles {
			fs.matches[i] = fuzzy.Rank{Source: str, Target: title, OriginalIndex: i}
		}
	} else {
		fs.matches = fuzzy.RankFindFold(str, fs.roomTitles)
		sortRoomMatches(fs.matches, str, fs.roomLower, fs.roomRecency)
	}
	fs.selected = 0
	fs.scrollOffset = 0
}

func (fs *FuzzySearchModal) OnKeyEvent(event mauview.KeyEvent) bool {
	kb := config.Keybind{
		Key: event.Key(),
		Ch:  event.Rune(),
		Mod: event.Modifiers(),
	}
	switch fs.parent.config.Keybindings.Modal[kb] {
	case "cancel":
		fs.parent.HideModal()
		return true
	case "select_next":
		if len(fs.matches) > 0 {
			fs.selected = (fs.selected + 1) % len(fs.matches)
		}
		return true
	case "select_prev":
		if len(fs.matches) > 0 {
			fs.selected = (fs.selected - 1 + len(fs.matches)) % len(fs.matches)
		}
		return true
	case "confirm":
		if fs.selected < len(fs.matches) {
			room := fs.roomList[fs.matches[fs.selected].OriginalIndex]
			debug.Print("Fuzzy Selected Room:", room.Name)
			fs.parent.SwitchRoom(room.RoomID)
		}
		fs.parent.HideModal()
		return true
	}
	return fs.search.OnKeyEvent(event)
}

// fuzzyResultsView renders the match list. mauview's TextView highlight is
// hardcoded to reverse video, so drawing directly is the only way to give the
// selection the same treatment as the room list, and it allows per-network
// icon colors without pushing tag-like room names through a region parser.
type fuzzyResultsView struct {
	parent *FuzzySearchModal
}

func (fr *fuzzyResultsView) Draw(screen mauview.Screen) {
	fs := fr.parent
	width, height := screen.Size()
	// Keep the selection in view.
	if fs.selected < fs.scrollOffset {
		fs.scrollOffset = fs.selected
	} else if fs.selected >= fs.scrollOffset+height {
		fs.scrollOffset = fs.selected - height + 1
	}
	for y := 0; y < height; y++ {
		i := fs.scrollOffset + y
		if i >= len(fs.matches) {
			break
		}
		entry := fs.roomList[fs.matches[i].OriginalIndex]
		rowStyle := tcell.StyleDefault
		if i == fs.selected {
			rowStyle = rowStyle.
				Foreground(ColorSelectionText).
				Background(ColorSelectionBackground).
				Bold(true)
		}
		widget.WriteLinePadded(screen, mauview.AlignLeft, "", 0, y, width, rowStyle)
		icon, iconColor := BridgeIconColor(entry.Bridge)
		widget.WriteLine(screen, mauview.AlignLeft, icon, 1, y, 2, rowStyle.Foreground(iconColor))
		nameMax := width - 4
		widget.WriteLine(screen, mauview.AlignLeft, runewidth.Truncate(entry.Name, nameMax, "…"), 3, y, nameMax, rowStyle)
	}
}

func (fr *fuzzyResultsView) OnKeyEvent(_ mauview.KeyEvent) bool     { return false }
func (fr *fuzzyResultsView) OnPasteEvent(_ mauview.PasteEvent) bool { return false }
func (fr *fuzzyResultsView) OnMouseEvent(_ mauview.MouseEvent) bool { return false }

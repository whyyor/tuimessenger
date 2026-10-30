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
	roomAliases []string
	roomRecency []time.Time

	parent *MainView
}

func NewFuzzySearchModal(mainView *MainView, width int, height int) *FuzzySearchModal {
	fs := &FuzzySearchModal{parent: mainView}
	// Skip the chat that's already open; a new slice, as the store owns the old one.
	for _, room := range applyLocalSeen(mainView.matrix, mainView.matrix.ReversedRoomList.Current()) {
		if mainView.currentRoom == nil || room.RoomID != mainView.currentRoom.Room.ID {
			fs.roomList = append(fs.roomList, room)
		}
	}
	fs.roomTitles = make([]string, len(fs.roomList))
	fs.roomLower = make([]string, len(fs.roomList))
	fs.roomRecency = make([]time.Time, len(fs.roomList))
	fs.roomAliases = make([]string, len(fs.roomList))
	for i, room := range fs.roomList {
		fs.roomTitles[i] = room.Name
		fs.roomLower[i] = strings.ToLower(room.Name)
		fs.roomAliases[i] = strings.ToLower(aliasOf(mainView.matrix, room.RoomID))
		fs.roomRecency[i] = room.SortingTimestamp
	}

	fs.results = &fuzzyResultsView{parent: fs}
	fs.search = mauview.NewInputArea().
		SetChangedFunc(fs.changeHandler).
		SetTextColor(tcell.ColorDefault).
		SetBackgroundColor(tcell.ColorDefault)
	fs.search.Focus()

	// Telescope's proportions: the prompt on top (a search icon over the chat
	// icons, typing over the names, the count at the right), a blank row, a
	// centred label, then the list, best match first.
	prompt := mauview.NewTextView().
		SetText("  \uf002 ").
		SetTextColor(ColorComposerBar)
	inputRow := mauview.NewFlex().
		SetDirection(mauview.FlexColumn).
		AddFixedComponent(prompt, 4).
		AddProportionalComponent(fs.search, 1).
		AddFixedComponent(&fuzzyCountView{parent: fs}, fuzzyCountWidth)
	flex := mauview.NewFlex().
		SetDirection(mauview.FlexRow).
		AddFixedComponent(inputRow, 1).
		AddFixedComponent(spacer{}, 1).
		AddFixedComponent(fuzzyLabel{}, 1).
		AddProportionalComponent(fs.results, 1)

	// A solid borderless panel with a centred pill title; the borderless box
	// only keeps "click outside closes".
	fs.container = mauview.NewBox(&panelFrame{inner: flex, title: " \uf086 Chats "}).
		SetBorder(false).
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
		// A chat matches on its name or its alias.
		fs.matches = fs.matches[:0]
		for i, title := range fs.roomTitles {
			if fuzzy.MatchFold(str, title) || (fs.roomAliases[i] != "" && fuzzy.MatchFold(str, fs.roomAliases[i])) {
				fs.matches = append(fs.matches, fuzzy.Rank{Source: str, Target: title, OriginalIndex: i})
			}
		}
		sortRoomMatches(fs.matches, str, fs.roomLower, fs.roomAliases, fs.roomRecency)
	}
	fs.selected = 0
	fs.scrollOffset = 0
}

func (fs *FuzzySearchModal) OnKeyEvent(event mauview.KeyEvent) bool {
	// Ctrl+p peeks: opens the chat without sending seen this visit.
	if event.Key() == tcell.KeyCtrlP {
		if fs.selected < len(fs.matches) {
			fs.parent.PeekRoom(fs.roomList[fs.matches[fs.selected].OriginalIndex].RoomID)
		}
		fs.parent.HideModal()
		return true
	}
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
	accentStyle := tcell.StyleDefault.Foreground(ColorSelectionBackground)
	for visRow := 0; visRow < height; visRow++ {
		i := fs.scrollOffset + visRow
		// Best match first, right under the prompt.
		y := visRow
		if i >= len(fs.matches) {
			break
		}
		entry := fs.roomList[fs.matches[i].OriginalIndex]
		// One grid for the whole switcher: ▌ and › in column 0, icons, the
		// count and typed text in 2, names in 4. The panel pads the edges.
		rowStyle := tcell.StyleDefault
		if i == fs.selected {
			rowStyle = rowStyle.Background(ColorBarBackground).Bold(true)
			widget.WriteLinePadded(screen, mauview.AlignLeft, "", 0, y, width, rowStyle)
			screen.SetContent(0, y, '▌', nil, accentStyle.Background(ColorBarBackground))
		}
		icon, iconColor := BridgeIconColor(entry.Bridge)
		// Icons never bold: Nerd Font glyphs have no bold face and render oddly.
		iconStyle := rowStyle.Foreground(iconColor).Bold(false)
		if i == fs.selected {
			iconStyle = iconStyle.Foreground(vivid(iconColor))
		}
		widget.WriteLine(screen, mauview.AlignLeft, icon, 2, y, 2, iconStyle)
		nameMax := width - 5
		if entry.UnreadMessages > 0 || entry.UnreadNotifications > 0 || entry.UnreadHighlights > 0 || entry.MarkedUnread {
			nameMax -= 2
			widget.WriteLine(screen, mauview.AlignLeft, "●", width-2, y, 1, rowStyle.Foreground(tcell.ColorWhite))
		}
		writeNameAlias(screen, 4, y, nameMax, entry.Name, fs.parent.showAlias(entry.RoomID), rowStyle, aliasDim(rowStyle))
	}
}

// fuzzyCountView is the dim "12 / 775" at the prompt's right, ending where
// the unread dots do.
type fuzzyCountView struct {
	parent *FuzzySearchModal
}

const fuzzyCountWidth = 14

func (fc *fuzzyCountView) Draw(screen mauview.Screen) {
	fs := fc.parent
	width, _ := screen.Size()
	count := fmt.Sprintf("%d / %d", len(fs.matches), len(fs.roomList))
	cw := runewidth.StringWidth(count)
	widget.WriteLine(screen, mauview.AlignLeft, count, max(width-1-cw, 0), 0, cw, tcell.StyleDefault.Foreground(ColorStatusText))
}

// fuzzyLabel is the centred dim line over the list, like Telescope's
// "Results", carrying the keys.
type fuzzyLabel struct{}

func (fuzzyLabel) Draw(screen mauview.Screen) {
	width, _ := screen.Size()
	label := "enter open  ·  ctrl+p peek"
	if runewidth.StringWidth(label) > width {
		label = "ctrl+p peek"
	}
	lw := runewidth.StringWidth(label)
	widget.WriteLine(screen, mauview.AlignLeft, label, max((width-lw)/2, 0), 0, lw, tcell.StyleDefault.Foreground(ColorStatusText))
}

func (fuzzyLabel) OnKeyEvent(mauview.KeyEvent) bool     { return false }
func (fuzzyLabel) OnPasteEvent(mauview.PasteEvent) bool { return false }
func (fuzzyLabel) OnMouseEvent(mauview.MouseEvent) bool { return false }

func (fc *fuzzyCountView) OnKeyEvent(_ mauview.KeyEvent) bool     { return false }
func (fc *fuzzyCountView) OnPasteEvent(_ mauview.PasteEvent) bool { return false }
func (fc *fuzzyCountView) OnMouseEvent(_ mauview.MouseEvent) bool { return false }

func (fr *fuzzyResultsView) OnKeyEvent(_ mauview.KeyEvent) bool     { return false }
func (fr *fuzzyResultsView) OnPasteEvent(_ mauview.PasteEvent) bool { return false }
func (fr *fuzzyResultsView) OnMouseEvent(_ mauview.MouseEvent) bool { return false }

// Switcher panel colours: a solid dark panel, a filled pink title pill.
var (
	// Just above Ghostty's own background (#0b0e13): a deep, dark pane.
	ColorPanel = tcell.NewHexColor(0x0e0e13)
	// The pill is the selection's ▌ red, with the sidebar's off-white text.
	ColorTitlePill   = ColorSelectionBackground
	ColorOnTitlePill = ColorSelectionText
)

// panelFrame draws a component on a solid panel with no border: the title
// as a centred pill in the panel's top row, then the component with one
// cell of padding at the sides and one row at the bottom.
type panelFrame struct {
	inner mauview.Component
	title string
}

func (f *panelFrame) Draw(screen mauview.Screen) {
	w, h := screen.Size()
	panel := &panelScreen{Screen: screen, bg: ColorPanel}
	panel.Fill(' ', tcell.StyleDefault)
	if tw := runewidth.StringWidth(f.title); tw <= w {
		widget.WriteLine(screen, mauview.AlignLeft, f.title, (w-tw)/2, 0, tw,
			tcell.StyleDefault.Background(ColorTitlePill).Foreground(ColorOnTitlePill))
	}
	f.inner.Draw(&mauview.ProxyScreen{Parent: panel, OffsetX: 1, OffsetY: 1, Width: max(w-2, 0), Height: max(h-2, 0)})
}

func (f *panelFrame) OnKeyEvent(event mauview.KeyEvent) bool     { return f.inner.OnKeyEvent(event) }
func (f *panelFrame) OnPasteEvent(event mauview.PasteEvent) bool { return f.inner.OnPasteEvent(event) }
func (f *panelFrame) OnMouseEvent(event mauview.MouseEvent) bool { return f.inner.OnMouseEvent(event) }

func (f *panelFrame) Focus() {
	if fc, ok := f.inner.(mauview.Focusable); ok {
		fc.Focus()
	}
}

func (f *panelFrame) Blur() {
	if fc, ok := f.inner.(mauview.Focusable); ok {
		fc.Blur()
	}
}

// panelScreen paints the panel colour wherever a cell would otherwise show
// the (transparent) terminal background.
type panelScreen struct {
	mauview.Screen
	bg tcell.Color
}

func (p *panelScreen) style(st tcell.Style) tcell.Style {
	if _, bg, _ := st.Decompose(); bg == tcell.ColorDefault {
		return st.Background(p.bg)
	}
	return st
}

func (p *panelScreen) SetContent(x, y int, mainc rune, combc []rune, st tcell.Style) {
	p.Screen.SetContent(x, y, mainc, combc, p.style(st))
}

func (p *panelScreen) SetCell(x, y int, st tcell.Style, ch ...rune) {
	p.Screen.SetCell(x, y, p.style(st), ch...)
}

func (p *panelScreen) Fill(r rune, st tcell.Style) {
	w, h := p.Size()
	st = p.style(st)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p.Screen.SetContent(x, y, r, nil, st)
		}
	}
}

func (p *panelScreen) Clear() { p.Fill(' ', tcell.StyleDefault) }

// spacer is an empty row in a flex layout.
type spacer struct{}

func (spacer) Draw(mauview.Screen)                  {}
func (spacer) OnKeyEvent(mauview.KeyEvent) bool     { return false }
func (spacer) OnPasteEvent(mauview.PasteEvent) bool { return false }
func (spacer) OnMouseEvent(mauview.MouseEvent) bool { return false }

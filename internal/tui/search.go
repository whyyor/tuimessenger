package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"go.mau.fi/mauview"

	"go.mau.fi/gomuks/pkg/hicli/cmdspec"
	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/tui/messages"
	"go.mau.fi/gomuks/tui/widget"
)

// chatSearch is Ctrl+f: find text in the open chat, newest match first.
type chatSearch struct {
	query   []rune
	matches []database.EventRowID // oldest first
	cur     int                   // index into matches, -1 if none
	// Where we were, restored by Escape.
	prevSelected database.EventRowID
	prevScroll   int32
}

// searchText is what a message is searched by: its text, or for media the
// caption and file name (not the local download link).
func searchText(m *messages.UIMessage) string {
	if fm, ok := m.Renderer.(*messages.FileMessage); ok {
		return fm.Body + " " + fm.FileName
	}
	return m.Renderer.PlainText()
}

// SearchRows lists messages containing query (any case), oldest first.
func (view *MessageView) SearchRows(query string) []database.EventRowID {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	view.lock.RLock()
	defer view.lock.RUnlock()
	var out []database.EventRowID
	var prev *messages.UIMessage
	for _, m := range view.msgBuffer {
		if m == prev {
			continue
		}
		prev = m
		if m.IsService || m.Event == nil || m.RowID == 0 || m.Sender == cmdspec.FakeGomuksSender {
			continue
		}
		if strings.Contains(strings.ToLower(searchText(m)), query) {
			out = append(out, m.RowID)
		}
	}
	return out
}

// FocusRow selects a message and scrolls it into view on the next draw.
func (view *MessageView) FocusRow(row database.EventRowID) {
	view.lock.Lock()
	defer view.lock.Unlock()
	view.selected = row
	view.followSelection = row != 0
}

func (view *RoomView) OpenSearch() {
	if view.search != nil {
		return
	}
	mv := view.MessageView()
	view.search = &chatSearch{cur: -1, prevSelected: mv.selected, prevScroll: mv.ScrollOffset.Load()}
	// Like Ctrl+L, search more than the screenful: preload older history.
	go view.parent.loadHistoryFor(view.Room, bareHistoryTarget)
}

// refreshSearch re-runs the query; jumpNewest moves to the newest match,
// otherwise the current match is kept (history arriving shifts indexes).
func (view *RoomView) refreshSearch(jumpNewest bool) {
	s := view.search
	var curRow database.EventRowID
	if s.cur >= 0 && s.cur < len(s.matches) {
		curRow = s.matches[s.cur]
	}
	s.matches = view.MessageView().SearchRows(string(s.query))
	s.cur = -1
	if len(s.matches) == 0 {
		return
	}
	if !jumpNewest {
		for i, row := range s.matches {
			if row == curRow {
				s.cur = i
				return
			}
		}
	}
	s.cur = len(s.matches) - 1
	view.MessageView().FocusRow(s.matches[s.cur])
}

func (view *RoomView) closeSearch(restore bool) {
	s := view.search
	view.search = nil
	mv := view.MessageView()
	if restore {
		mv.FocusRow(0)
		mv.lock.Lock()
		mv.selected = s.prevSelected
		mv.lock.Unlock()
		mv.ScrollOffset.Store(s.prevScroll)
	}
	RequestRepaint()
}

// onSearchKey handles every key while searching.
func (view *RoomView) onSearchKey(event mauview.KeyEvent) bool {
	s := view.search
	move := func(step int) {
		if len(s.matches) == 0 {
			return
		}
		s.cur = min(max(s.cur+step, 0), len(s.matches)-1)
		view.MessageView().FocusRow(s.matches[s.cur])
	}
	switch event.Key() {
	case tcell.KeyEscape:
		view.closeSearch(true)
	case tcell.KeyEnter:
		if s.cur >= 0 {
			// Stay on the match in visual mode: r, y, +, Space… act on it.
			view.closeSearch(false)
			view.selecting = true
			view.selectReason = SelectReply
			view.selectContent = ""
			view.input.Blur()
		} else {
			view.closeSearch(true)
		}
	case tcell.KeyUp, tcell.KeyCtrlP:
		move(-1)
	case tcell.KeyDown, tcell.KeyCtrlN:
		move(1)
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if len(s.query) > 0 {
			s.query = s.query[:len(s.query)-1]
			view.refreshSearch(true)
		}
	case tcell.KeyCtrlU:
		s.query = s.query[:0]
		view.refreshSearch(true)
	case tcell.KeyRune:
		s.query = append(s.query, event.Rune())
		view.refreshSearch(true)
	}
	return true
}

// drawSearch draws "/ query_   2 of 7 · ↑↓ older/newer · ⏎ select · esc".
func (view *RoomView) drawSearch(screen mauview.Screen, x, y, width int) {
	s := view.search
	view.refreshSearch(false) // history may have loaded meanwhile
	accent := tcell.StyleDefault.Foreground(ColorComposerBar).Bold(true)
	dim := tcell.StyleDefault.Foreground(ColorStatusText)
	widget.WriteLinePadded(screen, mauview.AlignLeft, "", x, y, width, tcell.StyleDefault)
	widget.WriteLine(screen, mauview.AlignLeft, "/ ", x, y, 2, accent)
	query := string(s.query)
	widget.WriteLine(screen, mauview.AlignLeft, query, x+2, y, width-2, tcell.StyleDefault)
	screen.ShowCursor(x+2+runewidth.StringWidth(query), y)

	status := "search this chat"
	switch {
	case len(s.query) > 0 && len(s.matches) == 0:
		status = "no matches"
	case len(s.matches) > 0:
		// Count from the newest: 1 of 7 is the latest match.
		status = fmt.Sprintf("%d of %d · ↑↓ older/newer · ⏎ select", len(s.matches)-s.cur, len(s.matches))
	}
	status += " · esc"
	sw := runewidth.StringWidth(status)
	if sx := x + width - sw; sx > x+2+runewidth.StringWidth(query)+2 {
		widget.WriteLine(screen, mauview.AlignLeft, status, sx, y, sw, dim)
	}
}

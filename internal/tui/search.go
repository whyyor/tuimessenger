package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"go.mau.fi/mauview"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/cmdspec"
	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
	"go.mau.fi/gomuks/tui/debug"
	"go.mau.fi/gomuks/tui/messages"
	"go.mau.fi/gomuks/tui/widget"
)

// searchLimit caps local full-text hits per query (newest first).
const searchLimit = 500

// searchHit is one match: its row and timestamp (ms) for ordering.
type searchHit struct {
	row database.EventRowID
	ts  int64
}

// chatSearch is Ctrl+f: find text in the open chat's whole local history.
type chatSearch struct {
	mu      sync.Mutex
	query   []rune
	local   []searchHit // full-text results for the current query
	matches []searchHit // local ∪ loaded substring matches, oldest first
	cur     int         // index into matches, -1 if none
	seq     int         // bumps per query; stale results are dropped
	busy    bool        // a local search is running
	loading bool        // paging history back to reach a match
	closed  bool        // Escape/Enter: background work must stop touching it
	// Where we were, restored by Escape.
	prevSelected database.EventRowID
	prevScroll   int32
}

// searchText is what a loaded message is searched by: its text, or for media
// the caption and file name (not the local download link).
func searchText(m *messages.UIMessage) string {
	if fm, ok := m.Renderer.(*messages.FileMessage); ok {
		return fm.Body + " " + fm.FileName
	}
	return m.Renderer.PlainText()
}

// ftsQuery turns typed words into an fts5 query: every word as a prefix.
func ftsQuery(query string) string {
	var parts []string
	for _, word := range strings.Fields(query) {
		parts = append(parts, `"`+strings.ReplaceAll(word, `"`, `""`)+`"*`)
	}
	return strings.Join(parts, " ")
}

// SearchRows finds loaded messages containing query (any case).
func (view *MessageView) SearchRows(query string) []searchHit {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}
	view.lock.RLock()
	defer view.lock.RUnlock()
	var out []searchHit
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
			out = append(out, searchHit{m.RowID, m.Timestamp.UnixMilli()})
		}
	}
	return out
}

// isLoaded reports whether the row is in the drawn timeline.
func (view *MessageView) isLoaded(row database.EventRowID) bool {
	view.lock.RLock()
	defer view.lock.RUnlock()
	return slices.ContainsFunc(view.msgBuffer, func(m *messages.UIMessage) bool { return m.RowID == row })
}

// SelectedRow is the selected message, read under the view lock.
func (view *MessageView) SelectedRow() database.EventRowID {
	view.lock.RLock()
	defer view.lock.RUnlock()
	return view.selected
}

// FocusRow selects a message and scrolls it into view on the next draw.
func (view *MessageView) FocusRow(row database.EventRowID) {
	view.lock.Lock()
	defer view.lock.Unlock()
	view.selected = row
	view.followSelection = row != 0
}

// mergeHits unions hit lists by row, oldest first.
func mergeHits(lists ...[]searchHit) []searchHit {
	seen := make(map[database.EventRowID]bool)
	var out []searchHit
	for _, list := range lists {
		for _, h := range list {
			if !seen[h.row] {
				seen[h.row] = true
				out = append(out, h)
			}
		}
	}
	slices.SortStableFunc(out, func(a, b searchHit) int {
		switch {
		case a.ts < b.ts:
			return -1
		case a.ts > b.ts:
			return 1
		}
		return int(a.row - b.row)
	})
	return out
}

func (view *RoomView) OpenSearch() {
	if view.search != nil {
		return
	}
	mv := view.MessageView()
	view.search = &chatSearch{cur: -1, prevSelected: mv.selected, prevScroll: mv.ScrollOffset.Load()}
}

// searchLocal is swapped out in tests.
var searchLocal = func(view *RoomView, query string) ([]*database.Event, error) {
	resp, err := view.parent.matrix.SearchLocal(context.TODO(), &jsoncmd.SearchParams{
		SearchTerm: ftsQuery(query),
		RoomIDs:    []id.RoomID{view.Room.ID},
		Limit:      searchLimit,
		SortByTime: true,
	})
	if err != nil {
		return nil, err
	}
	return resp.Events, nil
}

// visibleHits drops results /clear or h hid in gomuks.
func (view *RoomView) visibleHits(evts []*database.Event) []searchHit {
	clear := view.Room.ClearState()
	cleared := clear.At()
	var out []searchHit
	for _, evt := range evts {
		if evt == nil || clear.IsHidden(evt.ID) || (!cleared.IsZero() && !evt.Timestamp.After(cleared)) {
			continue
		}
		out = append(out, searchHit{evt.RowID, evt.Timestamp.UnixMilli()})
	}
	return out
}

// runSearch starts the full-text search for the current query.
func (view *RoomView) runSearch() {
	s := view.search
	s.mu.Lock()
	s.seq++
	seq, query := s.seq, string(s.query)
	s.local = nil
	busy := strings.TrimSpace(query) != ""
	s.busy = busy
	s.mu.Unlock()
	view.refreshSearch(s, true)
	if !busy {
		return
	}
	search := searchLocal // read the hook here, not in the goroutine
	go func() {
		defer debug.Recover()
		evts, err := search(view, query)
		if err != nil {
			debug.Print("Local search failed:", err)
		}
		hits := view.visibleHits(evts)
		s.mu.Lock()
		if s.seq != seq || s.closed {
			s.mu.Unlock()
			return // a newer query took over, or the search closed
		}
		s.busy = false
		s.local = hits
		s.mu.Unlock()
		view.refreshSearch(s, true)
		view.requestRender()
	}()
}

// refreshSearch rebuilds the match list; jumpNewest moves to the newest
// match, otherwise the current one is kept as results grow.
func (view *RoomView) refreshSearch(s *chatSearch, jumpNewest bool) {
	// The search goroutine calls this too: copy the query under the lock.
	s.mu.Lock()
	query := string(s.query)
	s.mu.Unlock()
	loaded := view.MessageView().SearchRows(query)
	s.mu.Lock()
	var curRow database.EventRowID
	if s.cur >= 0 && s.cur < len(s.matches) {
		curRow = s.matches[s.cur].row
	}
	s.matches = mergeHits(s.local, loaded)
	s.cur = -1
	if len(s.matches) == 0 {
		s.mu.Unlock()
		return
	}
	if !jumpNewest {
		for i, h := range s.matches {
			if h.row == curRow {
				s.cur = i
				s.mu.Unlock()
				return
			}
		}
	}
	s.cur = len(s.matches) - 1
	row := s.matches[s.cur].row
	s.mu.Unlock()
	view.focusMatch(s, row)
}

// loadUntil is swapped out in tests: page history back until row is loaded.
var loadUntil = func(view *RoomView, row database.EventRowID) {
	for range 400 {
		if view.MessageView().isLoaded(row) || !view.Room.HasMoreHistory() {
			return
		}
		if view.Room.Paginating.Load() {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if err := view.parent.matrix.LoadMoreHistory(context.TODO(), view.Room.ID); err != nil {
			debug.Print("Failed to load history for search:", err)
			return
		}
		// The new events are drawn (msgBuffer rebuilt) on the next frame.
		view.requestRender()
		time.Sleep(30 * time.Millisecond)
	}
}

// focusMatch selects a match, paging history back first if it's older
// than what's loaded.
func (view *RoomView) focusMatch(s *chatSearch, row database.EventRowID) {
	mv := view.MessageView()
	mv.FocusRow(row)
	if mv.isLoaded(row) {
		return
	}
	s.mu.Lock()
	if s.loading || s.closed {
		s.mu.Unlock()
		return
	}
	s.loading = true
	s.mu.Unlock()
	load := loadUntil
	go func() {
		defer debug.Recover()
		load(view, row)
		s.mu.Lock()
		s.loading = false
		still := !s.closed && s.cur >= 0 && s.cur < len(s.matches) && s.matches[s.cur].row == row
		s.mu.Unlock()
		if still {
			mv.FocusRow(row)
		}
		view.requestRender()
	}()
}

func (view *RoomView) closeSearch(restore bool) {
	s := view.search
	view.search = nil
	s.mu.Lock()
	s.seq++ // drop any search still running
	s.closed = true
	s.mu.Unlock()
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
		s.mu.Lock()
		if len(s.matches) == 0 {
			s.mu.Unlock()
			return
		}
		s.cur = min(max(s.cur+step, 0), len(s.matches)-1)
		row := s.matches[s.cur].row
		s.mu.Unlock()
		view.focusMatch(s, row)
	}
	switch event.Key() {
	case tcell.KeyEscape:
		view.closeSearch(true)
	case tcell.KeyEnter:
		s.mu.Lock()
		found := s.cur >= 0
		s.mu.Unlock()
		if found {
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
			s.setQuery(s.query[:len(s.query)-1])
			view.runSearch()
		}
	case tcell.KeyCtrlU:
		s.setQuery(nil)
		view.runSearch()
	case tcell.KeyRune:
		s.setQuery(append(slices.Clone(s.query), event.Rune()))
		view.runSearch()
	}
	return true
}

// searchStatus is the right-hand side of the search line.
func (s *chatSearch) status() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var parts []string
	switch {
	case len(s.query) == 0:
		parts = append(parts, "search this chat")
	case len(s.matches) == 0 && s.busy:
		parts = append(parts, "searching…")
	case len(s.matches) == 0:
		parts = append(parts, "no matches")
	default:
		// Count from the newest: 1 of 7 is the latest match.
		parts = append(parts, fmt.Sprintf("%d of %d", len(s.matches)-s.cur, len(s.matches)))
		if s.loading {
			parts = append(parts, "loading older messages…")
		} else {
			parts = append(parts, "↑↓ older/newer · ⏎ select")
		}
	}
	return strings.Join(append(parts, "esc"), " · ")
}

// searchPills is the right side of the search line: state, then key hints.
func (s *chatSearch) searchPills() (state []pill, keys []pill) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case len(s.query) == 0:
		state = []pill{{icon: "\uf002", label: "search this chat", color: ColorPillBlue}}
	case len(s.matches) == 0 && s.busy:
		state = []pill{{icon: "\uf021", label: "searching…", color: ColorPillBlue}}
	case len(s.matches) == 0:
		state = []pill{{icon: "\uf05e", label: "no matches", color: ColorPillPink}}
	default:
		// Count from the newest: 1 of 7 is the latest match.
		state = []pill{{icon: "\uf0c9", label: fmt.Sprintf("%d of %d", len(s.matches)-s.cur, len(s.matches)), color: ColorPillGreen}}
		if s.loading {
			state = append(state, pill{icon: "\uf021", label: "loading older…", color: ColorPillBlue})
		} else {
			keys = append(keys, pill{icon: "↑↓", label: "older/newer", color: ColorPillYellow, dim: true},
				pill{icon: "⏎", label: "select", color: ColorPillYellow, dim: true})
		}
	}
	keys = append(keys, pill{icon: "esc", label: "close", color: ColorPillYellow, dim: true})
	return state, keys
}

// drawSearch draws "[] query_        [≡] 2 of 7 [↑↓] older/newer [⏎] select [esc] close".
func (view *RoomView) drawSearch(screen mauview.Screen, x, y, width int) {
	s := view.search
	view.refreshSearch(s, false) // history may have loaded meanwhile
	widget.WriteLinePadded(screen, mauview.AlignLeft, "", x, y, width, tcell.StyleDefault)
	qx := x + drawPill(screen, x, y, "\uf002", "", ColorPillPink) + 1
	query := string(s.query)
	widget.WriteLine(screen, mauview.AlignLeft, query, qx, y, max(width-(qx-x), 0), tcell.StyleDefault.Foreground(ColorPillPink))
	cursorX := qx + runewidth.StringWidth(query)
	screen.ShowCursor(cursorX, y)

	state, keys := s.searchPills()
	// Drop key hints from the left until everything fits after the query.
	for len(keys) > 0 && x+width-pillsWidth(append(append([]pill{}, state...), keys...)) <= cursorX+2 {
		keys = keys[1:]
	}
	right := append(append([]pill{}, state...), keys...)
	if rx := x + width - pillsWidth(right); rx > cursorX+2 {
		drawPills(screen, rx, y, right)
	}
}

// setQuery changes the query under the lock the search goroutine reads it with.
func (s *chatSearch) setQuery(q []rune) {
	s.mu.Lock()
	s.query = q
	s.mu.Unlock()
}

package tui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/mauview"
	"go.mau.fi/util/jsontime"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli/cmdspec"
	"github.com/whyyor/tuimessenger/internal/hicli/database"
	"github.com/whyyor/tuimessenger/internal/tui/config"
	"github.com/whyyor/tuimessenger/internal/tui/messages"
)

func TestFTSQuery(t *testing.T) {
	if got := ftsQuery(` kya  "chal" `); got != `"kya"* """chal"""*` {
		t.Errorf("got %s", got)
	}
}

func TestMergeHits(t *testing.T) {
	got := mergeHits([]searchHit{{3, 30}, {1, 10}}, []searchHit{{2, 20}, {3, 30}})
	if len(got) != 3 || got[0].row != 1 || got[1].row != 2 || got[2].row != 3 {
		t.Errorf("deduped, oldest first: %v", got)
	}
}

func TestChatSearch(t *testing.T) {
	mv, room := bubbleView(t)
	now := time.Now()
	mk := func(row database.EventRowID, sender id.UserID, text string) *messages.UIMessage {
		m := textMsg(room, sender, now.Add(time.Duration(row)*time.Minute), text)
		m.RowID = row
		return m
	}
	a := mk(10, "@peter:x", "bring the Harness")
	b := mk(11, me, "ok")
	c := mk(12, "@peter:x", "harness and chalk")
	notice := mk(13, cmdspec.FakeSender, "harness notice")
	mv.msgBuffer = []*messages.UIMessage{a, a, b, c, notice}

	if got := mv.SearchRows("HARNESS"); len(got) != 2 || got[0].row != 10 || got[1].row != 12 {
		t.Fatalf("case-insensitive, oldest first, no tuimessenger notices: %v", got)
	}

	// The local database knows an older match that isn't loaded yet.
	old := &database.Event{RowID: 2, ID: "$old:x", Timestamp: jsontime.UM(now.Add(-48 * time.Hour))}
	origSearch, origLoad := searchLocal, loadUntil
	searchLocal = func(_ *RoomView, query string) ([]*database.Event, error) {
		return []*database.Event{old}, nil
	}
	loaded := make(chan database.EventRowID, 4)
	loadUntil = func(_ *RoomView, row database.EventRowID) {
		// "Paginate" the old message into the timeline.
		om := mk(2, "@peter:x", "harness from last month")
		mv.lock.Lock()
		mv.msgBuffer = append([]*messages.UIMessage{om}, mv.msgBuffer...)
		mv.lock.Unlock()
		loaded <- row
	}
	defer func() { searchLocal, loadUntil = origSearch, origLoad }()

	cfg := &config.Config{}
	cfg.LoadKeybindings()
	view := &RoomView{Room: room, config: cfg, content: mv, input: mauview.NewInputArea()}
	mv.parent = view
	mv.selected = 11
	view.search = &chatSearch{cur: -1, prevSelected: mv.selected}
	key := func(k tcell.Key, r rune) { view.OnKeyEvent(tcell.NewEventKey(k, r, tcell.ModNone)) }
	for _, r := range "harn" {
		key(tcell.KeyRune, r)
	}
	// Wait for the local search to land: 3 matches, newest selected.
	deadline := time.Now().Add(2 * time.Second)
	for {
		view.search.mu.Lock()
		n := len(view.search.matches)
		view.search.mu.Unlock()
		if n == 3 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if st := view.search.status(); st[:6] != "1 of 3" {
		t.Fatalf("status %q, want 1 of 3", st)
	}
	if mv.SelectedRow() != 12 {
		t.Errorf("newest match first, selected %d", mv.SelectedRow())
	}
	key(tcell.KeyUp, 0)
	key(tcell.KeyUp, 0)
	select {
	case row := <-loaded:
		if row != 2 {
			t.Errorf("loaded history for row %d", row)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("an unloaded match should page history back")
	}
	time.Sleep(50 * time.Millisecond)
	if mv.SelectedRow() != 2 {
		t.Errorf("after loading, the old match is selected: %d", mv.SelectedRow())
	}
	// Escape restores where we were.
	key(tcell.KeyEscape, 0)
	if view.search != nil || mv.SelectedRow() != 11 {
		t.Errorf("escape: search %v, selected %d", view.search, mv.SelectedRow())
	}
	// Enter keeps the match selected, in visual mode.
	view.search = &chatSearch{cur: -1}
	for _, r := range "chalk" {
		key(tcell.KeyRune, r)
	}
	key(tcell.KeyEnter, 0)
	if view.search != nil || !view.selecting || mv.SelectedRow() != 12 {
		t.Errorf("enter: search %v, selecting %v, selected %d", view.search, view.selecting, mv.SelectedRow())
	}
}

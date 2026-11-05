package tui

import (
	"testing"

	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/tui/messages"
)

func msgRow(row database.EventRowID) *messages.UIMessage {
	return &messages.UIMessage{Event: &database.Event{RowID: row}}
}

func TestVisualRange(t *testing.T) {
	a, b, c := msgRow(1), msgRow(2), msgRow(3)
	date := &messages.UIMessage{Event: &database.Event{}, IsService: true}
	// a spans 2 lines, then a date line, b spans 3, c spans 1.
	view := &MessageView{msgBuffer: []*messages.UIMessage{a, a, date, b, b, b, c}}

	if _, _, ok := view.rangeLinesLocked(); ok {
		t.Fatal("no anchor: no range")
	}
	// Anchor on c, cursor moved up to a: range covers all of a through c.
	view.selected = 3
	view.ToggleAnchor()
	view.selected = 1
	lo, hi, ok := view.rangeLinesLocked()
	if !ok || lo != 0 || hi != 6 {
		t.Fatalf("range lines: got %d..%d ok=%v, want 0..6", lo, hi, ok)
	}
	got := view.RangeMessages()
	if len(got) != 3 || got[0] != a || got[1] != b || got[2] != c {
		t.Errorf("range messages: got %d, want a, b, c in order without the date line", len(got))
	}
	// Narrow the range to b..c.
	view.selected = 2
	if lo, hi, _ = view.rangeLinesLocked(); lo != 3 || hi != 6 {
		t.Errorf("narrowed: got %d..%d, want 3..6", lo, hi)
	}
	view.ToggleAnchor()
	if view.HasAnchor() {
		t.Error("second v should drop the range")
	}
}

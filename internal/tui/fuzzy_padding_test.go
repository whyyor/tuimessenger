package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/lithammer/fuzzysearch/fuzzy"
	"go.mau.fi/gomuks/pkg/rpc/store"
)

func TestFuzzyResultsSymmetricMargins(t *testing.T) {
	entry := &store.RoomListEntry{Name: "Internal Acme's", Bridge: "whatsapp"}
	entry.UnreadMessages = 1
	fs := &FuzzySearchModal{
		roomList: []*store.RoomListEntry{entry},
		matches:  fuzzy.Ranks{{OriginalIndex: 0}},
	}
	const w = 30
	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(w, 1)
	(&fuzzyResultsView{parent: fs}).Draw(sim)
	bg := func(x int) tcell.Color {
		_, _, st, _ := sim.GetContent(x, 0)
		_, b, _ := st.Decompose()
		return b
	}
	if bg(0) == ColorBarBackground || bg(w-1) == ColorBarBackground {
		t.Error("first and last columns should both be empty margins")
	}
	if bg(1) != ColorBarBackground || bg(w-2) != ColorBarBackground {
		t.Error("the bar should span columns 1..w-2")
	}
	if r, _, _, _ := sim.GetContent(w-3, 0); r != '●' {
		t.Error("unread dot sits one cell inside the bar's right edge")
	}
}

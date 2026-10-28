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

func TestSwitcherIconNotBold(t *testing.T) {
	entry := &store.RoomListEntry{Name: "Internal Acme's", Bridge: "whatsapp"}
	fs := &FuzzySearchModal{roomList: []*store.RoomListEntry{entry}, matches: fuzzy.Ranks{{OriginalIndex: 0}}}
	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(30, 1)
	(&fuzzyResultsView{parent: fs}).Draw(sim)
	attrs := func(x int) tcell.AttrMask {
		_, _, st, _ := sim.GetContent(x, 0)
		_, _, a := st.Decompose()
		return a
	}
	if attrs(3)&tcell.AttrBold != 0 {
		t.Error("the selected row's icon should not be bold")
	}
	if attrs(5)&tcell.AttrBold == 0 {
		t.Error("the selected row's name stays bold")
	}
}

func TestSwitcherIconGlows(t *testing.T) {
	entries := []*store.RoomListEntry{{Name: "a", Bridge: "whatsapp"}, {Name: "b", Bridge: "whatsapp"}}
	fs := &FuzzySearchModal{roomList: entries, matches: fuzzy.Ranks{{OriginalIndex: 0}, {OriginalIndex: 1}}}
	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(30, 2)
	(&fuzzyResultsView{parent: fs}).Draw(sim)
	fg := func(y int) tcell.Color {
		_, _, st, _ := sim.GetContent(3, y)
		f, _, _ := st.Decompose()
		return f
	}
	_, brand := BridgeIconColor("whatsapp")
	// The selected (best) match is the bottom row.
	if fg(0) != brand {
		t.Errorf("other rows keep the brand colour: %v", fg(0))
	}
	if want := mixColor(brand, tcell.ColorWhite, iconGlow); fg(1) != want || want == brand {
		t.Errorf("the selected icon is lit: got %v want %v", fg(1), want)
	}
	if mixColor(tcell.ColorDefault, tcell.ColorWhite, 0.5) != tcell.ColorDefault {
		t.Error("default colours stay default")
	}
}

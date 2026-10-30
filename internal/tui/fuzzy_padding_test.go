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
	// The panel pads the edges now, so the bar runs edge to edge inside it.
	if bg(0) != ColorBarBackground || bg(w-1) != ColorBarBackground {
		t.Error("the bar should span the whole inner width")
	}
	if r, _, _, _ := sim.GetContent(0, 0); r != '▌' {
		t.Error("the accent sits in column 0, like the prompt's ›")
	}
	if r, _, _, _ := sim.GetContent(w-2, 0); r != '●' {
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
	if attrs(2)&tcell.AttrBold != 0 {
		t.Error("the selected row's icon should not be bold")
	}
	if attrs(4)&tcell.AttrBold == 0 {
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
		_, _, st, _ := sim.GetContent(2, y)
		f, _, _ := st.Decompose()
		return f
	}
	_, brand := BridgeIconColor("whatsapp")
	// The selected (best) match is the bottom row.
	if fg(0) != brand {
		t.Errorf("other rows keep the brand colour: %v", fg(0))
	}
	if want := vivid(brand); fg(1) != want || want == brand {
		t.Errorf("the selected icon is lit: got %v want %v", fg(1), want)
	}
	if got := vivid(tcell.NewHexColor(0x25d366)); got != tcell.NewHexColor(0x00ff5f) {
		t.Errorf("WhatsApp green should go neon and stay green: %06x", got.Hex())
	}
	// Pure blue reads dark on the bar, so blues keep some lightness.
	if r, g, b := vivid(tcell.NewHexColor(0x5865f2)).RGB(); 0.2126*float64(r)+0.7152*float64(g)+0.0722*float64(b) < 0.3*255 {
		t.Errorf("blues must not go dark: %d,%d,%d", r, g, b)
	}
	if got := vivid(tcell.NewHexColor(0xf5f5f7)); got.Hex()&0xff0000 < 0xf00000 {
		t.Errorf("near-greys stay light: %06x", got.Hex())
	}
	if vivid(tcell.ColorDefault) != tcell.ColorDefault {
		t.Error("default colours stay default")
	}
}

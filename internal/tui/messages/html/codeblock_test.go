package html

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/mauview"
)

func TestCodeBlockPaddingAndEdges(t *testing.T) {
	text := NewTextEntity("x := 1")
	text.Style = tcell.StyleDefault.Background(CodeBackground)
	block := NewCodeBlockEntity([]Entity{text}, tcell.StyleDefault.Background(CodeBackground))
	block.CalculateBuffer(30, 0, DrawContext{})
	if block.Height() != 3 {
		t.Fatalf("one line plus two half-row edges, got %d", block.Height())
	}
	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(30, 3)
	block.Draw(&mauview.ProxyScreen{Parent: sim, Width: 30, Height: 3}, DrawContext{})
	cell := func(x, y int) (rune, tcell.Color, tcell.Color) {
		r, _, st, _ := sim.GetContent(x, y)
		fg, bg, _ := st.Decompose()
		return r, fg, bg
	}
	if r, fg, _ := cell(0, 0); r != '▄' || fg != CodeBackground {
		t.Errorf("top edge: half a row of the block colour, got %q", r)
	}
	if r, fg, _ := cell(0, 2); r != '▀' || fg != CodeBackground {
		t.Errorf("bottom edge: half a row of the block colour, got %q", r)
	}
	if r, _, bg := cell(codePadX, 1); r != 'x' || bg != CodeBackground {
		t.Errorf("code starts after the side padding, got %q", r)
	}
	if r, _, bg := cell(0, 1); r != ' ' || bg != CodeBackground {
		t.Error("the padding is the block colour")
	}
}

func TestCodeColoursFromTheTerminalPalette(t *testing.T) {
	for classic, pro := range monokaiPro {
		if classic == pro.Hex() {
			t.Errorf("%06x should map to a Monokai Pro shade", classic)
		}
	}
}

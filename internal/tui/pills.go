package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"go.mau.fi/mauview"

	"go.mau.fi/gomuks/tui/widget"
)

// Accent colours for pills and highlights (Kanagawa, like the rest of the theme).
var (
	ColorPillYellow = tcell.NewHexColor(0xe6c384)
	ColorPillPink   = tcell.NewHexColor(0xd27e99)
	ColorPillGreen  = tcell.NewHexColor(0x98bb6c)
	ColorPillBlue   = tcell.NewHexColor(0x7e9cd8)
	ColorPillViolet = tcell.NewHexColor(0xa8a9eb)
	// ColorOnPill is dark text on a coloured block.
	ColorOnPill = tcell.NewHexColor(0x1f1f28)
	// ColorHighlight is the selected-row background (pastel purple) and
	// ColorOnHighlight the text on it, matching the message selection.
	ColorHighlight   = tcell.NewHexColor(0x8d70c2)
	ColorOnHighlight = tcell.ColorWhite
)

// pillWidth is how many cells drawPill uses.
func pillWidth(icon, label string) int {
	w := runewidth.StringWidth(icon) + 2
	if label != "" {
		w += runewidth.StringWidth(label) + 1
	}
	return w
}

// drawPill draws "[ icon ] label": the icon on a coloured block, the label in
// that colour on the terminal's own background. Returns the width used.
func drawPill(screen mauview.Screen, x, y int, icon, label string, color tcell.Color) int {
	return drawPillStyled(screen, x, y, icon, label, color, tcell.StyleDefault.Foreground(color))
}

func drawPillStyled(screen mauview.Screen, x, y int, icon, label string, color tcell.Color, labelStyle tcell.Style) int {
	block := tcell.StyleDefault.Background(color).Foreground(ColorOnPill)
	iw := runewidth.StringWidth(icon) + 2
	widget.WriteLinePadded(screen, mauview.AlignLeft, " "+icon+" ", x, y, iw, block)
	if label == "" {
		return iw
	}
	lw := runewidth.StringWidth(label)
	widget.WriteLine(screen, mauview.AlignLeft, label, x+iw+1, y, lw, labelStyle)
	return iw + 1 + lw
}

// pill is one entry in a row of pills; dim draws the label grey (key hints).
type pill struct {
	icon, label string
	color       tcell.Color
	dim         bool
}

// pillsWidth is the width of pills drawn with one space between them.
func pillsWidth(pills []pill) int {
	w := 0
	for i, p := range pills {
		if i > 0 {
			w++
		}
		w += pillWidth(p.icon, p.label)
	}
	return w
}

// drawPills draws pills left to right from x, one space apart.
func drawPills(screen mauview.Screen, x, y int, pills []pill) {
	for _, p := range pills {
		labelStyle := tcell.StyleDefault.Foreground(p.color)
		if p.dim {
			labelStyle = tcell.StyleDefault.Foreground(ColorStatusText)
		}
		x += drawPillStyled(screen, x, y, p.icon, p.label, p.color, labelStyle) + 1
	}
}

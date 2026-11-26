package tui

import (
	"github.com/gdamore/tcell/v2"
	"go.mau.fi/mauview"

	"github.com/whyyor/tuimessenger/internal/tui/widget"
)

// drawPanelTitle writes a panel's title, bold in color, on its first row: panels have no border, so the title is
// what names them.
func drawPanelTitle(screen mauview.Screen, x, y, width int, title string, color tcell.Color) {
	if width <= 0 {
		return
	}
	widget.WriteLine(screen, mauview.AlignLeft, title, x, y, width, tcell.StyleDefault.Foreground(color).Bold(true))
}

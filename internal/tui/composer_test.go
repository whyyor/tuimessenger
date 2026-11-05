package tui

import (
	"strings"
	"testing"

	"go.mau.fi/mauview"
)

// The box height comes from PrepareDraw's wrap; Draw wraps again at the
// input screen's width. Both must agree, or line 1 scrolls out of view.
func TestComposerMeasuresAtDrawWidth(t *testing.T) {
	const roomWidth = 40
	input := mauview.NewInputArea()
	// Fits on one line at roomWidth, needs two at the narrower draw width.
	input.SetText(strings.Repeat("x", roomWidth-inputOffsetX))

	input.PrepareDraw(roomWidth - inputOffsetX)
	measured := input.GetTextHeight()

	fb := newFakeScreen(roomWidth-inputOffsetX, measured)
	input.Draw(fb)
	if drawn := input.GetTextHeight(); drawn != measured {
		t.Fatalf("measured %d lines but drew %d: the first line would scroll away", measured, drawn)
	}
	if measured != 2 {
		t.Errorf("expected the text to wrap to 2 lines, got %d", measured)
	}
}

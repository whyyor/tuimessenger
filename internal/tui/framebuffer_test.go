package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/mauview"
)

// fakeScreen is a minimal real screen that counts SetContent calls.
type fakeScreen struct {
	frameBuffer
	sets int
}

func newFakeScreen(w, h int) *fakeScreen {
	fs := &fakeScreen{}
	fs.w, fs.h = w, h
	fs.cells = make([]frameCell, w*h)
	fs.Fill(' ', tcell.StyleDefault)
	return fs
}

func (fs *fakeScreen) SetContent(x, y int, mainc rune, combc []rune, style tcell.Style) {
	fs.sets++
	fs.frameBuffer.SetContent(x, y, mainc, combc, style)
}

func (fs *fakeScreen) Colors() int                { return 256 }
func (fs *fakeScreen) CharacterSet() string       { return "UTF-8" }
func (fs *fakeScreen) CanDisplay(rune, bool) bool { return true }
func (fs *fakeScreen) HasKey(tcell.Key) bool      { return true }

// clearingComponent mimics mauview's Flex: clear everything, then draw.
type clearingComponent struct {
	text string
}

func (c *clearingComponent) Draw(screen mauview.Screen) {
	screen.Clear()
	for i, r := range c.text {
		screen.SetContent(i, 0, r, []rune{0x0305}, tcell.StyleDefault)
	}
	screen.ShowCursor(len(c.text), 0)
}
func (c *clearingComponent) OnKeyEvent(mauview.KeyEvent) bool     { return false }
func (c *clearingComponent) OnPasteEvent(mauview.PasteEvent) bool { return false }
func (c *clearingComponent) OnMouseEvent(mauview.MouseEvent) bool { return false }

func TestBufferedRootOnlySendsChangedCells(t *testing.T) {
	real := newFakeScreen(20, 3)
	comp := &clearingComponent{text: "hello"}
	root := newBufferedRoot(comp)

	root.Draw(real)
	if real.sets != 5 {
		t.Fatalf("first frame: expected 5 cells sent, got %d", real.sets)
	}

	real.sets = 0
	root.Draw(real)
	if real.sets != 0 {
		t.Fatalf("identical frame despite component clear: expected 0 cells sent, got %d", real.sets)
	}

	real.sets = 0
	comp.text = "hellp"
	root.Draw(real)
	if real.sets != 1 {
		t.Fatalf("one changed cell: expected 1 cell sent, got %d", real.sets)
	}
	if !real.cursorShown || real.cursorX != 5 {
		t.Fatalf("cursor not forwarded: shown=%v x=%d", real.cursorShown, real.cursorX)
	}

	// A terminal-side clear (resume after suspend) must repaint the content.
	real.Fill(' ', tcell.StyleDefault)
	real.sets = 0
	root.Draw(real)
	if real.sets != 5 {
		t.Fatalf("after real screen clear: expected 5 cells repainted, got %d", real.sets)
	}
}

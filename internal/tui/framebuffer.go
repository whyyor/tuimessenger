package tui

import (
	"slices"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"go.mau.fi/mauview"
)

// mauview's Flex clears the whole screen every frame, and tcell 2.9 decides
// whether a cell is dirty by comparing it with the just-cleared buffer, so
// every frame rewrote the entire screen (~17 KB per keystroke with images on
// screen). The cursor is hidden for that whole write, so the terminal often
// presented frames without it, which reads as cursor flicker.
//
// bufferedRoot lets the component tree draw into an in-memory grid, clears
// and all, then hands tcell only the cells that differ from what it already
// holds. Comparing against the real screen rather than a private shadow copy
// means a terminal-side clear (resume after suspend, resize) repaints
// correctly without any invalidation bookkeeping.
type bufferedRoot struct {
	inner mauview.Component
	buf   frameBuffer
}

func newBufferedRoot(inner mauview.Component) *bufferedRoot {
	return &bufferedRoot{inner: inner}
}

func (br *bufferedRoot) Draw(screen mauview.Screen) {
	br.buf.begin(screen)
	br.inner.Draw(&br.buf)
	br.buf.flush(screen)
}

func (br *bufferedRoot) OnKeyEvent(event mauview.KeyEvent) bool {
	return br.inner.OnKeyEvent(event)
}

func (br *bufferedRoot) OnPasteEvent(event mauview.PasteEvent) bool {
	return br.inner.OnPasteEvent(event)
}

func (br *bufferedRoot) OnMouseEvent(event mauview.MouseEvent) bool {
	return br.inner.OnMouseEvent(event)
}

func (br *bufferedRoot) Focus() {
	if f, ok := br.inner.(mauview.Focusable); ok {
		f.Focus()
	}
}

func (br *bufferedRoot) Blur() {
	if f, ok := br.inner.(mauview.Focusable); ok {
		f.Blur()
	}
}

type frameCell struct {
	main  rune
	comb  []rune
	style tcell.Style
}

// frameBuffer is an in-memory mauview.Screen for one frame.
type frameBuffer struct {
	real        mauview.Screen
	cells       []frameCell
	w, h        int
	style       tcell.Style
	cursorX     int
	cursorY     int
	cursorShown bool
}

func (fb *frameBuffer) begin(real mauview.Screen) {
	fb.real = real
	w, h := real.Size()
	if w != fb.w || h != fb.h || fb.cells == nil {
		fb.w, fb.h = w, h
		fb.cells = make([]frameCell, w*h)
	}
	fb.style = tcell.StyleDefault
	fb.cursorShown = false
	fb.Fill(' ', tcell.StyleDefault)
}

func (fb *frameBuffer) flush(real mauview.Screen) {
	for y := 0; y < fb.h; y++ {
		for x := 0; x < fb.w; x++ {
			c := &fb.cells[y*fb.w+x]
			rm, rc, rs, _ := real.GetContent(x, y)
			if rm != c.main || rs != c.style || !slices.Equal(rc, c.comb) {
				real.SetContent(x, y, c.main, c.comb, c.style)
			}
		}
	}
	if fb.cursorShown {
		real.ShowCursor(fb.cursorX, fb.cursorY)
	} else {
		real.HideCursor()
	}
}

func (fb *frameBuffer) Clear() {
	fb.Fill(' ', fb.style)
}

func (fb *frameBuffer) Fill(r rune, style tcell.Style) {
	for i := range fb.cells {
		fb.cells[i] = frameCell{main: r, style: style}
	}
}

func (fb *frameBuffer) SetStyle(style tcell.Style) {
	fb.style = style
}

func (fb *frameBuffer) SetCell(x, y int, style tcell.Style, ch ...rune) {
	if len(ch) > 0 {
		fb.SetContent(x, y, ch[0], ch[1:], style)
	}
}

func (fb *frameBuffer) GetContent(x, y int) (rune, []rune, tcell.Style, int) {
	if x < 0 || y < 0 || x >= fb.w || y >= fb.h {
		return ' ', nil, tcell.StyleDefault, 1
	}
	c := fb.cells[y*fb.w+x]
	width := runewidth.RuneWidth(c.main)
	if width == 0 || c.main < ' ' {
		return ' ', c.comb, c.style, 1
	}
	return c.main, c.comb, c.style, width
}

func (fb *frameBuffer) SetContent(x, y int, mainc rune, combc []rune, style tcell.Style) {
	if x < 0 || y < 0 || x >= fb.w || y >= fb.h {
		return
	}
	var comb []rune
	if len(combc) > 0 {
		comb = slices.Clone(combc)
	}
	fb.cells[y*fb.w+x] = frameCell{main: mainc, comb: comb, style: style}
}

func (fb *frameBuffer) ShowCursor(x, y int) {
	fb.cursorX, fb.cursorY, fb.cursorShown = x, y, true
}

func (fb *frameBuffer) HideCursor() {
	fb.cursorShown = false
}

func (fb *frameBuffer) Size() (int, int) {
	return fb.w, fb.h
}

func (fb *frameBuffer) Colors() int {
	return fb.real.Colors()
}

func (fb *frameBuffer) CharacterSet() string {
	return fb.real.CharacterSet()
}

func (fb *frameBuffer) CanDisplay(r rune, checkFallbacks bool) bool {
	return fb.real.CanDisplay(r, checkFallbacks)
}

func (fb *frameBuffer) HasKey(k tcell.Key) bool {
	return fb.real.HasKey(k)
}

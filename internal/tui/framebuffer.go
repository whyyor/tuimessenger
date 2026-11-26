package tui

import (
	"slices"
	"sync/atomic"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"go.mau.fi/mauview"

	"github.com/whyyor/tuimessenger/internal/tui/widget"
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

// repaintNext asks the next frame to repaint every cell; see RequestRepaint.
var repaintNext atomic.Bool

// RequestRepaint makes the next frame rewrite the whole terminal. The diff
// renderer only sends changed cells, so anything the terminal drew in a
// different place than tcell expected would otherwise stay on screen.
func RequestRepaint() {
	repaintNext.Store(true)
}

func (br *bufferedRoot) Draw(screen mauview.Screen) {
	br.buf.begin(screen)
	br.inner.Draw(&br.buf)
	br.buf.flush(screen)
	if repaintNext.Swap(false) {
		if s, ok := screen.(interface{ Sync() }); ok {
			s.Sync()
		}
	}
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
	tail  bool // right half of a two-column emoji (widget.WideTail)
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
	locked      []bool // cells locked in the real screen as an emoji's right half; tcell drops locks on resize
	// bg is each cell's panel background (see paintBackground): what a cell
	// that keeps the terminal's own background (ColorDefault) gets instead.
	bg []tcell.Color
}

func (fb *frameBuffer) begin(real mauview.Screen) {
	fb.real = real
	w, h := real.Size()
	if w != fb.w || h != fb.h || fb.cells == nil {
		fb.w, fb.h = w, h
		fb.cells = make([]frameCell, w*h)
		fb.locked = make([]bool, w*h)
		fb.bg = make([]tcell.Color, w*h)
	}
	clear(fb.bg) // ColorDefault is 0
	fb.style = tcell.StyleDefault
	fb.cursorShown = false
	fb.Fill(' ', tcell.StyleDefault)
}

// cellLocker is tcell's LockRegion: a locked cell is never drawn, and tcell moves the cursor explicitly to the
// next cell it draws.
type cellLocker interface {
	LockRegion(x, y, width, height int, lock bool)
}

func (fb *frameBuffer) flush(real mauview.Screen) {
	locker, canLock := real.(cellLocker)
	for y := 0; y < fb.h; y++ {
		for x := 0; x < fb.w; x++ {
			i := y*fb.w + x
			c := &fb.cells[i]
			style := themedOn(c.style, fb.bg[i])
			if c.tail && canLock {
				// The emoji to the left covers this column on screen; tcell must not draw over it.
				if !fb.locked[i] {
					real.SetContent(x, y, ' ', nil, style)
					locker.LockRegion(x, y, 1, 1, true)
					fb.locked[i] = true
				}
				continue
			}
			if fb.locked[i] {
				locker.LockRegion(x, y, 1, 1, false)
				fb.locked[i] = false
			}
			main, comb := c.main, c.comb
			if !canLock && x+1 < fb.w && fb.cells[i+1].tail {
				comb = nil // can't keep tcell off the next column: back to the one-column symbol
			}
			rm, rc, rs, _ := real.GetContent(x, y)
			if rm != main || rs != style || !slices.Equal(rc, comb) {
				real.SetContent(x, y, main, comb, style)
			}
		}
	}
	if fb.cursorShown {
		real.ShowCursor(fb.cursorX, fb.cursorY)
	} else {
		real.HideCursor()
	}
}

// themed paints the theme's background and text where a cell uses the terminal's own (ColorDefault), so a theme
// covers the whole screen; the tuimessenger theme leaves both to the terminal.
func themed(style tcell.Style) tcell.Style {
	return themedOn(style, tcell.ColorDefault)
}

// themedOn is themed for a cell that lies on a panel of colour panel (ColorDefault: on no panel), which then
// replaces the screen's background.
func themedOn(style tcell.Style, panel tcell.Color) tcell.Style {
	fg, bg, _ := style.Decompose()
	switch {
	case bg != tcell.ColorDefault:
	case panel != tcell.ColorDefault:
		style = style.Background(panel)
	case ColorBackground != tcell.ColorDefault:
		style = style.Background(ColorBackground)
	}
	if fg == tcell.ColorDefault && ColorText != tcell.ColorDefault {
		style = style.Foreground(ColorText)
	}
	return style
}

// paintBackground gives the rectangle x, y, w×h of screen the background color wherever what's drawn there keeps
// the terminal's own (ColorDefault): panels and message cards get their colour without every widget on them
// knowing about it. A later call paints over an earlier one. screen is the frame buffer or a ProxyScreen on it;
// on any other screen (tests, measuring) nothing happens.
func paintBackground(screen mauview.Screen, x, y, w, h int, color tcell.Color) {
	for {
		switch s := screen.(type) {
		case *mauview.ProxyScreen:
			x, y, w, h = clipRect(x, y, w, h, s.Width, s.Height)
			x, y, screen = x+s.OffsetX, y+s.OffsetY, s.Parent
		case *frameBuffer:
			x, y, w, h = clipRect(x, y, w, h, s.w, s.h)
			for row := y; row < y+h; row++ {
				line := s.bg[row*s.w : (row+1)*s.w]
				for col := x; col < x+w; col++ {
					line[col] = color
				}
			}
			return
		default:
			return
		}
	}
}

// clipRect cuts x, y, w×h down to what lies inside 0, 0, width×height.
func clipRect(x, y, w, h, width, height int) (int, int, int, int) {
	x0, y0 := max(x, 0), max(y, 0)
	x1, y1 := min(x+w, width), min(y+h, height)
	return x0, y0, max(x1-x0, 0), max(y1-y0, 0)
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
	if c.tail {
		return widget.WideTail, nil, c.style, 1
	}
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
	if mainc == widget.WideTail {
		fb.setWideTail(x, y, style)
		return
	}
	// ❤️ is 2 columns in Ghostty but 1 to tcell, so unless the caller gave it two (widget.WriteEmojiLine) it's drawn
	// as the text ❤, which took the text's colour (grey in reactions). Red keeps it a heart; on a coloured background
	// (selection, pills) red could vanish, so not there.
	if _, bg, _ := style.Decompose(); mainc == textHeart && bg == tcell.ColorDefault {
		style = style.Foreground(emojiHeartRed)
	}
	mainc, comb, blankNext := terminalSafe(mainc, combc)
	if x > 0 && isMismeasuredMark(mainc) {
		// Ghostty stacks the mark on the previous letter with zero advance;
		// put it there and leave this column blank so both agree on widths.
		prev := &fb.cells[y*fb.w+x-1]
		prev.comb = append(slices.Clone(prev.comb), mainc)
		prev.comb = append(prev.comb, comb...)
		mainc, comb = ' ', nil
	}
	fb.cells[y*fb.w+x] = frameCell{main: mainc, comb: comb, style: style}
	if blankNext && x+1 < fb.w {
		fb.cells[y*fb.w+x+1] = frameCell{main: ' ', style: style}
	}
}

// setWideTail makes the emoji left of x the real two-column one: it gets its variation selector back (terminalSafe
// dropped it), and x becomes its right half, which flush locks in tcell so it's never drawn over the emoji.
func (fb *frameBuffer) setWideTail(x, y int, style tcell.Style) {
	if x > 0 {
		if prev := &fb.cells[y*fb.w+x-1]; !prev.tail && runewidth.RuneWidth(prev.main) == 1 {
			prev.comb = []rune{emojiVS}
		}
	}
	fb.cells[y*fb.w+x] = frameCell{main: ' ', style: style, tail: true}
}

// terminalSafe rewrites a cell so tcell and the terminal agree on its width.
// tcell measures each codepoint on its own, but Ghostty joins emoji
// sequences into one glyph: 🧍🏻‍♂️ is 5 columns to tcell and 2 on screen,
// ⚜️ is 1 and 2. Every later cell on the row then lands off by the
// difference, and the diff renderer never cleans up after it. Dropping ZWJ
// and the emoji variation selector and blanking skin-tone modifiers keeps
// each piece a separate glyph of the width tcell expects. A blanked
// modifier was 2 columns wide, so the caller blanks the next cell too.
func terminalSafe(mainc rune, combc []rune) (rune, []rune, bool) {
	blankNext := false
	switch {
	case isSkinTone(mainc):
		mainc, blankNext = ' ', true
	case mainc == zwj || mainc == emojiVS:
		mainc = ' '
	}
	var comb []rune
	for _, r := range combc {
		if r != zwj && r != emojiVS && !isSkinTone(r) {
			comb = append(comb, r)
		}
	}
	return mainc, comb, blankNext
}

const (
	zwj       = '\u200d'
	emojiVS   = '\ufe0f'
	textHeart = '\u2764'
)

// emojiHeartRed is the red of the ❤️ emoji, the same in every theme.
var emojiHeartRed = tcell.NewHexColor(0xe0245e)

// isMismeasuredMark reports a non-spacing mark that go-runewidth gives a full
// column although terminals draw it with zero width, e.g. Devanagari े ु ं ्.
// Marks it already treats as zero-width (Latin accents) never reach a cell
// of their own, so they're unaffected.
func isMismeasuredMark(r rune) bool {
	return unicode.Is(unicode.Mn, r) && runewidth.RuneWidth(r) == 1
}

func isSkinTone(r rune) bool {
	return r >= 0x1f3fb && r <= 0x1f3ff
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

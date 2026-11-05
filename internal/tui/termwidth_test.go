package tui

import (
	"testing"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"go.mau.fi/mauview"

	"go.mau.fi/gomuks/tui/widget"
)

func TestTerminalSafe(t *testing.T) {
	if m, _, blank := terminalSafe(0x1f3fb, nil); m != ' ' || !blank {
		t.Errorf("skin tone: got %q blankNext=%v", m, blank)
	}
	if m, c, _ := terminalSafe('⚜', []rune{emojiVS}); m != '⚜' || len(c) != 0 {
		t.Errorf("⚜️: got %q %q, want the bare symbol", m, c)
	}
	if m, _, _ := terminalSafe(zwj, nil); m != ' ' {
		t.Errorf("lone ZWJ: got %q", m)
	}
	// Ordinary combining marks (Devanagari vowel signs) must survive.
	if _, c, _ := terminalSafe('र', []rune{'\u093f'}); len(c) != 1 {
		t.Errorf("Devanagari mark dropped: %q", c)
	}
}

// Ghostty advances zero columns for a non-spacing mark, so a cell starting
// with one would pull the rest of the row left. Draws the real chat name and
// checks none survive as a cell of their own and the text after keeps tcell's column.
func TestDevanagariMarksDontShiftTheRow(t *testing.T) {
	name := "प्रतीक सागर"
	fb := newFakeScreen(30, 1)
	widget.WriteLine(fb, mauview.AlignLeft, name+"X", 0, 0, 30, tcell.StyleDefault)

	gotX := -1
	for x := 0; x < 30; x++ {
		c := fb.cells[x]
		if unicode.Is(unicode.Mn, c.main) {
			t.Errorf("cell %d starts with non-spacing mark %U", x, c.main)
		}
		if c.main == 'X' {
			gotX = x
		}
	}
	if want := 11; gotX != want {
		t.Errorf("X at column %d, want %d", gotX, want)
	}
	if first := fb.cells[0]; first.main != 'प' || len(first.comb) != 1 || first.comb[0] != '\u094d' {
		t.Errorf("virama should stack on प: got %q + %q", first.main, first.comb)
	}
}

// Draws the real room name that misaligned rows and checks nothing the
// terminal would join reaches tcell, and the text after it keeps its column.
func TestJoinedEmojiDontShiftTheRow(t *testing.T) {
	name := "Family👫🧍🏻\u200d♂️"
	fb := newFakeScreen(40, 1)
	widget.WriteLine(fb, mauview.AlignLeft, name+"X", 0, 0, 40, tcell.StyleDefault)
	// tcell places runes one at a time; with nothing left for the terminal to
	// join, the terminal advances the same way, so this is the on-screen column.
	wantX := 0
	for _, r := range name {
		wantX += runewidth.RuneWidth(r)
	}

	gotX := -1
	for x := 0; x < 40; x++ {
		c := fb.cells[x]
		for _, r := range append([]rune{c.main}, c.comb...) {
			if r == zwj || r == emojiVS || isSkinTone(r) {
				t.Errorf("cell %d still carries %U", x, r)
			}
		}
		if c.main == 'X' {
			gotX = x
		}
	}
	if gotX != wantX {
		t.Errorf("X at column %d, want %d", gotX, wantX)
	}
}

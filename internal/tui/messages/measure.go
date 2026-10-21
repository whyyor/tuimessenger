package messages

import (
	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

// measuring is set while a message is drawn off-screen to find its width;
// kitty images then skip talking to the terminal.
var measuring bool

// measureScreen records the rightmost used column of each row.
type measureScreen struct {
	w, h  int
	right []int
}

func (s *measureScreen) mark(x, y, width int) {
	if y >= 0 && y < s.h && x >= 0 && x < s.w {
		s.right[y] = max(s.right[y], min(x+width, s.w))
	}
}

func (s *measureScreen) SetContent(x, y int, mainc rune, _ []rune, style tcell.Style) {
	_, bg, _ := style.Decompose()
	if mainc != ' ' && mainc != 0 || bg != tcell.ColorDefault {
		s.mark(x, y, max(runewidth.RuneWidth(mainc), 1))
	}
}

func (s *measureScreen) SetCell(x, y int, style tcell.Style, ch ...rune) {
	if len(ch) > 0 {
		s.SetContent(x, y, ch[0], ch[1:], style)
	}
}

func (s *measureScreen) GetContent(int, int) (rune, []rune, tcell.Style, int) {
	return ' ', nil, tcell.StyleDefault, 1
}

func (s *measureScreen) Clear()                     {}
func (s *measureScreen) Fill(rune, tcell.Style)     {}
func (s *measureScreen) SetStyle(tcell.Style)       {}
func (s *measureScreen) ShowCursor(int, int)        {}
func (s *measureScreen) HideCursor()                {}
func (s *measureScreen) Size() (int, int)           { return s.w, s.h }
func (s *measureScreen) Colors() int                { return 256 }
func (s *measureScreen) CharacterSet() string       { return "UTF-8" }
func (s *measureScreen) CanDisplay(rune, bool) bool { return true }
func (s *measureScreen) HasKey(tcell.Key) bool      { return true }

type measured struct {
	reactions     string
	content, last int
	valid         bool
	gen           int
}

// Measure returns the widest row and the last row's width when drawn at
// width, after CalculateBuffer(width). Cached until the layout changes.
func (msg *UIMessage) Measure(width int) (content, last int) {
	reactions := FormatReactions(msg.Event.Reactions)
	m := &msg.measured
	if m.valid && m.gen == msg.bufferGen && m.reactions == reactions {
		return m.content, m.last
	}
	h := msg.Height()
	if h <= 0 {
		return 0, 0
	}
	screen := &measureScreen{w: width, h: h, right: make([]int, h)}
	selected := msg.IsSelected
	msg.IsSelected = false
	measuring = true
	msg.Draw(screen)
	measuring = false
	msg.IsSelected = selected
	for _, r := range screen.right {
		content = max(content, r)
	}
	last = screen.right[h-1]
	*m = measured{reactions: reactions, content: content, last: last, valid: true, gen: msg.bufferGen}
	return content, last
}

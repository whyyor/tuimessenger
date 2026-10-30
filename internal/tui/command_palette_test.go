package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"go.mau.fi/gomuks/tui/config"
)

func TestKeyHintPrefersCtrl(t *testing.T) {
	bindings := map[config.Keybind]string{
		{Mod: tcell.ModAlt, Key: tcell.KeyRune, Ch: 'r'}:   "resync",
		{Mod: tcell.ModCtrl, Key: tcell.KeyCtrlR, Ch: 'r'}: "resync",
	}
	if got := keyHint(bindings, "resync"); got != "Ctrl+r" {
		t.Errorf("got %q, want Ctrl+r", got)
	}
	if got := keyHint(bindings, "missing"); got != "" {
		t.Errorf("got %q for unbound action", got)
	}
}

func TestSplitQuery(t *testing.T) {
	for _, c := range []struct {
		in       string
		section  paletteSection
		text     string
		filtered bool
	}{
		{"@peter", sectionChats, "peter", true},
		{">mute", sectionCommands, "mute", true},
		{"?", sectionKeys, "", true},
		{"resync", 0, "resync", false},
	} {
		s, text, f := splitQuery(c.in)
		if s != c.section || text != c.text || f != c.filtered {
			t.Errorf("%q: got %v %q %v", c.in, s, text, f)
		}
	}
}

func TestMatchScoreOrder(t *testing.T) {
	prefix := matchScore("Mute this chat", "mute")
	word := matchScore("Mark chat read", "chat")
	sub := matchScore("Resync", "sync")
	fz := matchScore("Resync with server", "rws")
	none := matchScore("Quit", "zzz")
	if !(prefix < word && word < sub && sub < fz) || none != -1 {
		t.Errorf("prefix=%d word=%d sub=%d fuzzy=%d none=%d", prefix, word, sub, fz, none)
	}
}

func testPalette() *CommandPalette {
	p := &CommandPalette{}
	p.items = []*paletteItem{
		{section: sectionCommands, title: "Resync with server"},
		{section: sectionCommands, title: "Mute this chat"},
		{section: sectionKeys, title: "Command palette"},
	}
	for i := range 10 {
		p.items = append(p.items, &paletteItem{section: sectionChats, title: fmt.Sprintf("Chat %d", i)})
	}
	p.refilter()
	return p
}

func TestPaletteRowsAndNavigation(t *testing.T) {
	p := testPalette()
	// Headers for all three sections, chats capped without @.
	headers, chats := 0, 0
	for _, r := range p.rows {
		if r.item == nil {
			headers++
		} else if r.item.section == sectionChats {
			chats++
		}
	}
	if headers != 3 || chats != paletteChatsUnfiltered {
		t.Fatalf("headers=%d chats=%d", headers, chats)
	}
	if p.rows[p.selected].item == nil || p.rows[p.selected].item.title != "Resync with server" {
		t.Fatalf("first item should be preselected, got row %d", p.selected)
	}
	// Up from the first item wraps to the last, skipping headers.
	p.move(-1)
	if p.rows[p.selected].item == nil || p.selected != len(p.rows)-1 {
		t.Errorf("wrap: selected %d of %d", p.selected, len(p.rows))
	}
	// @ shows every chat, no headers.
	p.query = []rune("@chat")
	p.refilter()
	if len(p.rows) != 10 || p.rows[0].item == nil {
		t.Errorf("@ filter: %d rows", len(p.rows))
	}
	// Typing ranks within a section.
	p.query = []rune("mute")
	p.refilter()
	if p.rows[p.selected].item.title != "Mute this chat" {
		t.Errorf("query should select the match, got %q", p.rows[p.selected].item.title)
	}
	p.query = []rune("qqqq")
	p.refilter()
	if len(p.rows) != 0 || p.selected != -1 {
		t.Errorf("no match should leave no rows")
	}
}

func TestPaletteSelectionLikeSwitcher(t *testing.T) {
	p := &CommandPalette{}
	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(50, 1)
	p.drawRow(sim, paletteRow{item: &paletteItem{title: "Resync with server", detail: "Ctrl+r"}}, true, 1, 0, 46)
	cell := func(x int) (rune, tcell.Color, tcell.Color) {
		r, _, st, _ := sim.GetContent(x, 0)
		fg, bg, _ := st.Decompose()
		return r, fg, bg
	}
	if r, fg, bg := cell(2); r != '▌' || fg != ColorPaletteAccent || bg != ColorBarBackground {
		t.Errorf("a green ▌ on the dark bar: %q %v %v", r, fg, bg)
	}
	if _, _, bg := cell(45); bg != ColorBarBackground {
		t.Error("the grey bar runs to the last inner column")
	}
	for _, x := range []int{1, 46} {
		if _, _, bg := cell(x); bg == ColorBarBackground {
			t.Errorf("column %d stays an empty margin", x)
		}
	}
	if _, fg, _ := cell(4); fg == ColorOnHighlight {
		t.Error("text keeps its own colour, not white-on-purple")
	}
}

func TestBorderLighterThanSelectionBar(t *testing.T) {
	luma := func(c tcell.Color) int32 { r, g, b := c.RGB(); return r + g + b }
	if luma(ColorBorder) <= luma(ColorBarBackground) {
		t.Errorf("border %06x should be lighter than the selection bar %06x", ColorBorder.Hex(), ColorBarBackground.Hex())
	}
}

func TestPaletteIsAGreenPanel(t *testing.T) {
	p := testPalette()
	const w, h = 70, 24
	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(w, h)
	p.Draw(sim)
	lines := screenText(sim, w, h)
	text := strings.Join(lines, "\n")
	for _, gone := range []string{"┌", "└", "╭", "│", "─"} {
		if strings.Contains(text, gone) {
			t.Errorf("no border or rule (%q):\n%s", gone, text)
		}
	}
	at := func(x, y int) (rune, tcell.Color, tcell.Color) {
		r, _, st, _ := sim.GetContent(x, y)
		fg, bg, _ := st.Decompose()
		return r, fg, bg
	}
	col := len([]rune(lines[p.y][:strings.Index(lines[p.y], "Command palette")]))
	if _, fg, bg := at(col, p.y); bg != ColorPalettePill || fg != ColorOnPalettePill || ColorPalettePill != ColorMonokaiGreen {
		t.Errorf("a Monokai green pill with black text: %v on %v", fg, bg)
	}
	if centre := p.x + p.w/2; col < centre-12 || col > centre {
		t.Errorf("the pill is centred: col %d, panel %d+%d", col, p.x, p.w)
	}
	if _, _, bg := at(p.x, p.y); bg != ColorPanel {
		t.Error("panel background around the pill")
	}
	if strings.TrimSpace(lines[p.y+1]) != "" {
		t.Errorf("a blank row under the pill: %q", lines[p.y+1])
	}
	if r, _, _ := at(p.x+1, p.listY+1); r != '▌' {
		t.Error("the first item is selected, with the ▌ one column in")
	}
	prompt := p.listY + p.listH + 1
	if strings.TrimSpace(lines[prompt-1]) != "" || !strings.Contains(lines[prompt], "\U000f0349") {
		t.Errorf("a blank row, then the prompt at the bottom: %q / %q", lines[prompt-1], lines[prompt])
	}
	if _, _, bg := at(p.x, prompt+1); bg != ColorPanel || strings.TrimSpace(lines[prompt+1]) != "" || prompt+2 != p.y+p.h {
		t.Error("one padding row under the prompt, then the panel ends")
	}
}

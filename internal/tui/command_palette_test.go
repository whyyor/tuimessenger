package tui

import (
	"fmt"
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

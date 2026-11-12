package tui

import (
	"strings"
	"testing"
	"time"

	"codeberg.org/tslocum/cbind"
	"github.com/gdamore/tcell/v2"
	"go.mau.fi/mauview"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
	"github.com/whyyor/tuimessenger/internal/rpc/client"
	"github.com/whyyor/tuimessenger/internal/rpc/store"
	"github.com/whyyor/tuimessenger/internal/tui/config"
	"github.com/whyyor/tuimessenger/internal/tui/messages"
)

func decodeKey(t *testing.T, s string) config.Keybind {
	t.Helper()
	mod, key, ch, err := cbind.Decode(s)
	if err != nil {
		t.Fatal(err)
	}
	return config.Keybind{Mod: mod, Key: key, Ch: ch}
}

// However a terminal (or a test) encodes a key, it finds its binding.
func TestKeyOf(t *testing.T) {
	for _, c := range []struct {
		event *tcell.EventKey
		want  string
	}{
		{tcell.NewEventKey(tcell.KeyRune, 'R', tcell.ModShift), "R"},
		{tcell.NewEventKey(tcell.KeyRune, 'R', tcell.ModNone), "R"},
		{tcell.NewEventKey(tcell.KeyCtrlE, 0, tcell.ModNone), "Ctrl+e"},
		{tcell.NewEventKey(tcell.KeyRune, 0x05, tcell.ModNone), "Ctrl+e"},
		{tcell.NewEventKey(tcell.KeyRune, 0x1e, tcell.ModNone), "Ctrl+^"},
		{tcell.NewEventKey(tcell.KeyRune, '?', tcell.ModNone), "?"},
		{tcell.NewEventKey(tcell.KeyF1, 0, tcell.ModNone), "F1"},
		{tcell.NewEventKey(tcell.KeyRune, 'k', tcell.ModAlt), "Alt+k"},
	} {
		if got := keyOf(c.event); got != decodeKey(t, c.want) {
			t.Errorf("%s: got %+v, want %+v", c.want, got, decodeKey(t, c.want))
		}
	}
}

func keyConfig(t *testing.T, yours config.RawKeybindings) *config.Config {
	t.Helper()
	cfg := &config.Config{Dir: t.TempDir()}
	cfg.LoadKeybindings()
	cfg.ApplyKeybindings(yours)
	return cfg
}

// A single character you bind (here ?) acts on the home screen, in visual mode and from an empty message box;
// while you write it's typed.
func TestTypedCharOnlyWhenNotTyping(t *testing.T) {
	cfg := keyConfig(t, config.RawKeybindings{Main: map[string]string{"?": "help"}})
	view := &MainView{config: cfg}
	question := tcell.NewEventKey(tcell.KeyRune, '?', tcell.ModNone)
	if view.mainAction(question) != "help" {
		t.Error("home screen: ? is help")
	}
	room := &RoomView{config: cfg, input: mauview.NewInputArea()}
	view.currentRoom = room
	if view.mainAction(question) != "help" {
		t.Error("empty message box: ? is help")
	}
	room.input.SetText("what")
	if view.mainAction(question) != "" {
		t.Error("while writing, ? is typed")
	}
	room.selecting = true
	if view.mainAction(question) != "help" {
		t.Error("visual mode: ? is help even with a draft")
	}
	room.selecting = false
	if view.mainAction(tcell.NewEventKey(tcell.KeyF1, 0, tcell.ModNone)) != "help" {
		t.Error("F1 is help while writing too")
	}
	if view.mainAction(tcell.NewEventKey(tcell.KeyCtrlK, 0, tcell.ModNone)) != "search_rooms" {
		t.Error("Ctrl keys act while writing")
	}
}

// Rebinding a visual-mode key really moves it: c copies, y no longer does.
func TestRebindVisualCopy(t *testing.T) {
	copied := make(chan string, 2)
	orig := writeClipboard
	writeClipboard = func(text, _ string) error { copied <- text; return nil }
	defer func() { writeClipboard = orig }()

	mv, room := bubbleView(t)
	mv.config = keyConfig(t, config.RawKeybindings{Visual: map[string]string{"c": "copy", "y": "none"}})
	now := time.Date(2026, 10, 22, 9, 0, 0, 0, time.Local)
	msgs := []*messages.UIMessage{textMsg(room, "@peter:x", now, "first"), textMsg(room, me, now.Add(time.Minute), "second")}
	for i, m := range msgs {
		m.RowID = database.EventRowID(i + 1)
		mv.msgBuffer = append(mv.msgBuffer, m)
	}
	view := &RoomView{Room: room, config: mv.config, content: mv, input: mauview.NewInputArea(), selecting: true}
	mv.parent = view
	mv.selected = msgs[1].RowID
	key := func(r rune) bool { return view.OnKeyEvent(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone)) }

	if key('y') || !view.selecting {
		t.Error("y is unbound now: nothing happens")
	}
	key('v')
	mv.selected = msgs[0].RowID
	key('c')
	select {
	case got := <-copied:
		if !strings.Contains(got, "first") || !strings.Contains(got, "second") {
			t.Errorf("c copies the range: %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("c didn't copy")
	}
}

func helpItem(items []*paletteItem, title string) *paletteItem {
	for _, item := range items {
		if item.title == title {
			return item
		}
	}
	return nil
}

// Help shows your keys: rebinds, unbound actions, Ghostty's Ctrl+m and Ctrl+;, fixed keys and every command.
func TestHelpListsYourKeys(t *testing.T) {
	cfg := keyConfig(t, config.RawKeybindings{Visual: map[string]string{"c": "copy", "y": "none"}})
	items := helpItems(cfg)
	if items[0].group != "Everywhere" {
		t.Errorf("starts with the keys that work everywhere: %q", items[0].group)
	}
	if got := helpItem(items, config.ActionDoc("visual", "copy")).detail; got != "c" {
		t.Errorf("copy shows your key: %q", got)
	}
	if got := helpItem(items, config.ActionDoc("main", "quit")).detail; got != "not set" {
		t.Errorf("an action without a key says so: %q", got)
	}
	if got := helpItem(items, config.ActionDoc("room", "mark_read")).detail; got != "Ctrl+m" {
		t.Errorf("Ctrl+^ shows as what you press: %q", got)
	}
	if got := helpItem(items, config.ActionDoc("main", "help")).detail; got != "F1" {
		t.Errorf("help's own keys: %q", got)
	}
	for section, actions := range config.Actions {
		for _, a := range actions {
			if helpItem(items, a.Doc) == nil {
				t.Errorf("%s.%s missing from help", section, a.Name)
			}
		}
	}
	if helpItem(items, "Browse chats") == nil || helpItem(items, "New line") == nil {
		t.Error("fixed keys are listed")
	}
	found := false
	for _, item := range items {
		found = found || (item.group == "Commands" && strings.HasPrefix(item.detail, "/mute"))
	}
	if !found {
		t.Error("commands are listed")
	}
}

// Help is searchable by what a key does or by the key.
func TestHelpModal(t *testing.T) {
	cfg := keyConfig(t, config.RawKeybindings{})
	room := &RoomView{config: cfg, input: mauview.NewInputArea()}
	view := &MainView{config: cfg, currentRoom: room, matrix: &client.Client{Store: store.NewStore()}}
	view.ShowHelp()
	p, ok := view.modal.(*CommandPalette)
	if !ok || !p.grouped || p.rows[0].header != "Everywhere" {
		t.Fatalf("help opens grouped: %T", view.modal)
	}
	p.query = []rune("Ctrl+f")
	p.refilter()
	if p.rows[p.selected].item.title != config.ActionDoc("room", "search") {
		t.Errorf("searching a key finds its action: %q", p.rows[p.selected].item.title)
	}
}

// ? is no shortcut by default: it's always typed (help is F1 and Ctrl+p).
func TestQuestionMarkIsTyped(t *testing.T) {
	cfg := keyConfig(t, config.RawKeybindings{})
	view := &MainView{config: cfg}
	if got := view.mainAction(tcell.NewEventKey(tcell.KeyRune, '?', tcell.ModNone)); got != "" {
		t.Errorf("? should be typed, not %q", got)
	}
}

// Ctrl+p's key hints follow your keys.
func TestPaletteHintsFollowRebinds(t *testing.T) {
	main := aliasMain(t)
	main.config = keyConfig(t, config.RawKeybindings{Room: map[string]string{"Ctrl+f": "none", "Alt+/": "search"}})
	main.currentRoom = &RoomView{config: main.config, input: mauview.NewInputArea(), Room: main.matrix.GetRoom("!peter:x"), parent: main}
	found := false
	for _, item := range (&CommandPalette{parent: main}).commandItems() {
		if item.title == "Search this chat" {
			found = true
			if item.detail != "Alt+/" {
				t.Errorf("search hint follows your key: %q", item.detail)
			}
		}
	}
	if !found {
		t.Fatal("no Search this chat with a chat open")
	}
	for _, item := range keyItems(main.config) {
		if item.section != sectionKeys {
			t.Errorf("Ctrl+p's ? section has only keys: %q", item.title)
		}
	}
}

package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"

	"go.mau.fi/gomuks/pkg/hicli/cmdspec"
	"go.mau.fi/gomuks/tui/config"
)

func TestUnreadCommandAndKey(t *testing.T) {
	found := false
	for _, cmd := range LocalCommands {
		if cmd.Command == CmdUnread {
			parsed, err := cmd.ParseInput(cmdspec.FakeGomuksSender, cmdSigils, "/unread")
			if err != nil || parsed == nil || parsed.MSC4391BotCommand.Command != CmdUnread {
				t.Fatalf("/unread: %v", err)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("/unread not registered")
	}
	cfg := &config.Config{}
	cfg.LoadKeybindings()
	if got := cfg.Keybindings.Main[config.Keybind{Mod: tcell.ModAlt, Key: tcell.KeyRune, Ch: 'u'}]; got != "mark_unread" {
		t.Errorf("Alt+u is %q", got)
	}
	if hint := keyHint(cfg.Keybindings.Main, "mark_unread"); hint != "Alt+u" {
		t.Errorf("palette hint %q", hint)
	}
}

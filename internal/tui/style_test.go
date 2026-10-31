package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"go.mau.fi/gomuks/tui/config"
	"go.mau.fi/gomuks/tui/messages"
)

// keepStyle restores every themable colour after a test that changes them.
func keepStyle(t *testing.T) {
	t.Helper()
	saved := map[*tcell.Color]tcell.Color{}
	for _, c := range styleColors() {
		for _, target := range c.targets {
			saved[target] = *target
		}
	}
	oldPinned := pinnedPrefs
	pinnedPrefs = map[string]bool{}
	t.Cleanup(func() {
		for target, color := range saved {
			*target = color
		}
		rebuildToastColors()
		pinnedPrefs = oldPinned
	})
}

func TestStyleColors(t *testing.T) {
	keepStyle(t)
	var prefs config.UserPreferences
	problems := applyStyle([]byte(`
colors:
  selection: "#123456"
  sidebar_selection: "#ff0000"
  toast_success: green
  panel: nope
  sparkles: "#ffffff"
`), &prefs)
	if messages.SelectionBackground != tcell.NewHexColor(0x123456) || ColorHighlight != tcell.NewHexColor(0x123456) {
		t.Error("selection sets the message and popup selection")
	}
	if ColorSelectionBackground != tcell.NewHexColor(0xff0000) || ColorTitlePill != ColorSelectionBackground {
		t.Error("the switcher pill follows the sidebar selection unless set")
	}
	if ColorPalettePill != tcell.ColorGreen || ColorPaletteAccent != tcell.ColorGreen || toastColors[toastSuccess] != tcell.ColorGreen {
		t.Error("named colours work; the palette pill and its ▌ follow toast_success; toasts are rebuilt")
	}
	joined := strings.Join(problems, "\n")
	if len(problems) != 2 || !strings.Contains(joined, "colors.panel") || !strings.Contains(joined, "unknown colour sparkles") {
		t.Errorf("bad and unknown colours are reported: %q", problems)
	}
}

func TestStylePreferences(t *testing.T) {
	keepStyle(t)
	prefs := config.UserPreferences{TimestampStyle: "absolute", HideAliases: true}
	problems := applyStyle([]byte(`
preferences:
  file_picker: yazi
  sidebar_on_start: true
  timestamp_style: relative
  sidebar_on_start_typo: true
  classic_layout: maybe
`), &prefs)
	if prefs.FilePicker != "yazi" || !prefs.SidebarOnStart || prefs.TimestampStyle != "" {
		t.Errorf("preferences override terminal.yaml: %+v", prefs)
	}
	if !prefs.HideAliases {
		t.Error("keys the file doesn't set keep terminal.yaml's value")
	}
	if !pinnedPrefs["file_picker"] || pinnedPrefs["hide_aliases"] || pinnedPrefs["classic_layout"] {
		t.Errorf("only valid keys from the file are pinned: %v", pinnedPrefs)
	}
	joined := strings.Join(problems, "\n")
	if !strings.Contains(joined, "unknown preference sidebar_on_start_typo") || !strings.Contains(joined, "preferences.classic_layout") {
		t.Errorf("unknown keys and wrong types are reported: %q", problems)
	}
}

func TestStyleFileOptional(t *testing.T) {
	keepStyle(t)
	cfg := &config.Config{Dir: t.TempDir()}
	if problems := applyStyleFile(cfg); problems != nil {
		t.Errorf("no gomuks.yaml is fine: %q", problems)
	}
	_ = os.WriteFile(filepath.Join(cfg.Dir, styleFileName), []byte("preferences:\n  file_picker: yazi\n"), 0o600)
	if problems := applyStyleFile(cfg); problems != nil || cfg.Preferences.FilePicker != "yazi" {
		t.Errorf("read from the config dir: %q %q", problems, cfg.Preferences.FilePicker)
	}
}

func TestStyleReadableDefaults(t *testing.T) {
	keepStyle(t)
	prefs := config.UserPreferences{FilePicker: "yazi", TimestampStyle: "off"}
	if problems := applyStyle([]byte("preferences:\n  file_picker: finder\n  timestamp_style: relative\n"), &prefs); problems != nil {
		t.Fatal(problems)
	}
	if prefs.FilePicker != "" || prefs.TimestampStyle != "" {
		t.Errorf("finder and relative mean the defaults: %+v", prefs)
	}
}

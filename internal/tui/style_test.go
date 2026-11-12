package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeberg.org/tslocum/cbind"
	"github.com/gdamore/tcell/v2"

	"github.com/whyyor/tuimessenger/internal/tui/config"
	"github.com/whyyor/tuimessenger/internal/tui/messages"
	"github.com/whyyor/tuimessenger/internal/tui/themes"
)

// keepStyle restores every themable colour after a test that changes them, and points tuimessenger.yaml at a
// temporary file so tests never touch yours.
func keepStyle(t *testing.T) string {
	t.Helper()
	saved := map[*tcell.Color]tcell.Color{}
	for _, targets := range colorTargets() {
		for _, target := range targets {
			saved[target] = *target
		}
	}
	oldPinned, oldColors, oldTheme, oldMode := pinnedPrefs, userColors, currentTheme, currentThemeMode
	pinnedPrefs, userColors = map[string]bool{}, map[string]string{}
	path := filepath.Join(t.TempDir(), "config", "tuimessenger.yaml")
	t.Setenv("TUIMESSENGER_CONFIG", path)
	t.Cleanup(func() {
		for target, color := range saved {
			*target = color
		}
		rebuildToastColors()
		pinnedPrefs, userColors, currentTheme, currentThemeMode = oldPinned, oldColors, oldTheme, oldMode
	})
	return path
}

// applyYAML applies a tuimessenger.yaml with this content to a fresh config.
func applyYAML(t *testing.T, path, content string) (*config.Config, []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Dir: t.TempDir()}
	cfg.LoadKeybindings()
	return cfg, applyStyleFile(cfg)
}

func TestStyleColors(t *testing.T) {
	path := keepStyle(t)
	_, problems := applyYAML(t, path, `
colors:
  selection: "#123456"
  sidebar_selection: "#ff0000"
  toast_success: green
  panel: nope
  sparkles: "#ffffff"
`)
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
	if len(problems) != 2 || !strings.Contains(joined, "line 6: colors.panel") || !strings.Contains(joined, `unknown colour "sparkles"`) {
		t.Errorf("bad and unknown colours are reported with their line: %q", problems)
	}
}

func TestStylePreferences(t *testing.T) {
	path := keepStyle(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(path, []byte(`
preferences:
  file_picker: yazi
  sidebar_on_start: true
  timestamp_style: relative
  sidebar_on_strat: true
  classic_layout: maybe
  image_protocol: auto
  backspace2_removes_word: true
`), 0o600)
	cfg := &config.Config{Dir: t.TempDir(), Preferences: config.UserPreferences{TimestampStyle: "absolute", HideAliases: true, ImageProtocol: "ansi"}}
	cfg.LoadKeybindings()
	problems := applyStyleFile(cfg)
	prefs := cfg.Preferences
	if prefs.FilePicker != "yazi" || !prefs.SidebarOnStart || prefs.TimestampStyle != "" || prefs.ImageProtocol != "" {
		t.Errorf("preferences override terminal.yaml, readable names mean the defaults: %+v", prefs)
	}
	if !cfg.Backspace2RemovesWord {
		t.Error("the backspace settings are preferences too")
	}
	if !prefs.HideAliases {
		t.Error("keys the file doesn't set keep terminal.yaml's value")
	}
	if !pinnedPrefs["file_picker"] || pinnedPrefs["hide_aliases"] || pinnedPrefs["classic_layout"] {
		t.Errorf("only valid keys from the file are pinned: %v", pinnedPrefs)
	}
	joined := strings.Join(problems, "\n")
	if !strings.Contains(joined, `unknown preference "sidebar_on_strat" (did you mean "sidebar_on_start"?)`) ||
		!strings.Contains(joined, "preferences.classic_layout is true or false") {
		t.Errorf("unknown keys (with a suggestion) and wrong types are reported: %q", problems)
	}
}

// Without any tuimessenger.yaml, a commented template is written, and it loads with no problems.
func TestStyleTemplate(t *testing.T) {
	path := keepStyle(t)
	cfg := &config.Config{Dir: t.TempDir()}
	cfg.LoadKeybindings()
	if problems := applyStyleFile(cfg); problems != nil {
		t.Errorf("no tuimessenger.yaml is fine: %q", problems)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the template should be written: %v", err)
	}
	if !strings.HasPrefix(string(data), "# yaml-language-server: $schema="+config.SchemaURL) {
		t.Error("the template points editors at the schema")
	}
	if problems := applyStyleFile(cfg); problems != nil || currentTheme != themes.Default {
		t.Errorf("the template changes nothing: %q %s", problems, currentTheme)
	}
	_ = os.WriteFile(path, []byte("preferences:\n  file_picker: yazi\n"), 0o600)
	if _ = applyStyleFile(cfg); string(must(os.ReadFile(path))) != "preferences:\n  file_picker: yazi\n" {
		t.Error("an existing file is never overwritten")
	}
}

func must(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}

// The old file next to terminal.yaml still works; ~/.config's wins over it.
func TestStyleLegacyFile(t *testing.T) {
	path := keepStyle(t)
	cfg := &config.Config{Dir: t.TempDir()}
	cfg.LoadKeybindings()
	_ = os.WriteFile(filepath.Join(cfg.Dir, styleFileName), []byte("preferences:\n  file_picker: yazi\n  hide_aliases: true\ncolors:\n  accent: \"#111111\"\n"), 0o600)
	if problems := applyStyleFile(cfg); problems != nil || cfg.Preferences.FilePicker != "yazi" || ColorComposerBar != tcell.NewHexColor(0x111111) {
		t.Errorf("the old file is read: %q %+v", problems, cfg.Preferences)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("no template while the old file exists")
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte("preferences:\n  file_picker: finder\ncolors:\n  accent: \"#222222\"\n"), 0o600)
	cfg.Preferences = config.UserPreferences{}
	if problems := applyStyleFile(cfg); problems != nil {
		t.Fatal(problems)
	}
	if cfg.Preferences.FilePicker != "" || !cfg.Preferences.HideAliases || ColorComposerBar != tcell.NewHexColor(0x222222) {
		t.Errorf("the new file wins, the old one fills in the rest: %+v %v", cfg.Preferences, ColorComposerBar)
	}
}

func hex(c tcell.Color) string { return fmt.Sprintf("#%06x", c.Hex()) }

// A theme sets the colours, yours go on top, and going back to tuimessenger restores every built-in colour.
func TestThemes(t *testing.T) {
	path := keepStyle(t)
	_, problems := applyYAML(t, path, "theme: tokyonight\ncolors:\n  border: \"#010203\"\n")
	if problems != nil || currentTheme != "tokyonight" {
		t.Fatalf("%q %s", problems, currentTheme)
	}
	if hex(ColorComposerBar) != "#82aaff" || hex(ColorSelectionBackground) != "#ff966c" || hex(ColorPanel) != "#1e2030" {
		t.Errorf("tokyonight's colours: accent %s sidebar %s panel %s", hex(ColorComposerBar), hex(ColorSelectionBackground), hex(ColorPanel))
	}
	if ColorTitlePill != ColorSelectionBackground || ColorPaletteAccent != ColorPalettePill {
		t.Error("colours that follow another follow the theme's")
	}
	if hex(ColorBorder) != "#010203" {
		t.Error("your colours win over the theme's")
	}
	if !pinnedPrefs["theme"] {
		t.Error("a theme in the file is pinned")
	}
	userColors = map[string]string{}
	if err := setTheme(themes.Default, themes.Dark); err != nil {
		t.Fatal(err)
	}
	for key, color := range builtinColors {
		if *colorTargets()[key][0] != color {
			t.Errorf("%s not restored", key)
		}
	}
	if err := setTheme("catppuccin", themes.Light); err != nil || hex(ColorComposerBar) != "#1e66f5" {
		t.Errorf("light mode: %v %s", err, hex(ColorComposerBar))
	}
}

func TestUnknownTheme(t *testing.T) {
	path := keepStyle(t)
	_, problems := applyYAML(t, path, "theme: tokionight\n")
	if len(problems) != 1 || !strings.Contains(problems[0], `(did you mean "tokyonight"?)`) || currentTheme != themes.Default {
		t.Errorf("a typo in the theme is reported and the default kept: %q %s", problems, currentTheme)
	}
}

// Every colour the file can set has variables to set, and the documented defaults are the real ones.
func TestColorOptionsMatchVariables(t *testing.T) {
	targets := colorTargets()
	if len(targets) != len(config.ColorOptions) {
		t.Errorf("%d colour variables, %d documented colours", len(targets), len(config.ColorOptions))
	}
	for _, c := range config.ColorOptions {
		if targets[c.Key] == nil {
			t.Errorf("colors.%s sets nothing", c.Key)
		} else if c.Default == "default" && builtinColors[c.Key] != tcell.ColorDefault {
			t.Errorf("colors.%s: documented default is the terminal's own, real %s", c.Key, hex(builtinColors[c.Key]))
		} else if c.Default != "default" && hex(builtinColors[c.Key]) != c.Default {
			t.Errorf("colors.%s: documented default %s, real %s", c.Key, c.Default, hex(builtinColors[c.Key]))
		}
	}
}

func TestStyleKeybindings(t *testing.T) {
	path := keepStyle(t)
	cfg, problems := applyYAML(t, path, `
keybindings:
  main:
    'Ctrl+n': next_room
    'ctrl+s': none
    'Hyper+q': next_room
    'Ctrl+g': launch_rocket
`)
	kb := func(s string) config.Keybind {
		mod, key, ch, err := cbind.Decode(s)
		if err != nil {
			t.Fatal(err)
		}
		return config.Keybind{Mod: mod, Key: key, Ch: ch}
	}
	if cfg.Keybindings.Main[kb("Ctrl+n")] != "next_room" {
		t.Error("your binding is added")
	}
	if _, ok := cfg.Keybindings.Main[kb("Ctrl+s")]; ok {
		t.Error("none removes the default Ctrl+s")
	}
	if cfg.Keybindings.Main[kb("Ctrl+k")] != "search_rooms" {
		t.Error("other defaults stay")
	}
	joined := strings.Join(problems, "\n")
	if len(problems) != 2 || !strings.Contains(joined, `"Hyper+q" isn't a key`) || !strings.Contains(joined, `unknown main action "launch_rocket"`) {
		t.Errorf("bad keys and actions are reported, not a crash: %q", problems)
	}
}

// A theme paints the terminal's own background and text everywhere; colours set on purpose stay.
func TestThemeBackground(t *testing.T) {
	keepStyle(t)
	plain := tcell.StyleDefault
	if themed(plain) != plain {
		t.Error("the tuimessenger theme leaves the terminal's background and text alone")
	}
	if err := setTheme("catppuccin", themes.Dark); err != nil {
		t.Fatal(err)
	}
	if hex(ColorBackground) != "#1e1e2e" || ColorText == tcell.ColorDefault {
		t.Fatalf("catppuccin's background and text: %s %v", hex(ColorBackground), ColorText)
	}
	fg, bg, _ := themed(plain).Decompose()
	if bg != ColorBackground || fg != ColorText {
		t.Errorf("default cells take the theme's: fg %v bg %v", fg, bg)
	}
	pill := tcell.StyleDefault.Background(tcell.ColorRed).Foreground(tcell.ColorWhite)
	if themed(pill) != pill {
		t.Error("explicit colours stay")
	}
	if err := setTheme(themes.Default, themes.Dark); err != nil {
		t.Fatal(err)
	}
	if ColorBackground != tcell.ColorDefault || themed(plain) != plain {
		t.Error("back to tuimessenger: the terminal's own again")
	}
}

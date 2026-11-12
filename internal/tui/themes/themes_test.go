package themes

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var colorValue = regexp.MustCompile(`^(#[0-9a-f]{6}|default)$`)

// Every bundled theme resolves in both modes to a colour for every key.
func TestBuiltinThemesResolve(t *testing.T) {
	names := Builtin()
	if names[0] != Default || len(names) != 34 {
		t.Fatalf("want tuimessenger + 33 opencode themes, got %d: %v", len(names), names)
	}
	for _, name := range names[1:] {
		for _, mode := range []Mode{Dark, Light} {
			c, err := Load(name, mode, "")
			if err != nil {
				t.Errorf("%s (%s): %v", name, mode, err)
				continue
			}
			if len(c) != 28 {
				t.Errorf("%s (%s): %d colours, want 28", name, mode, len(c))
			}
			for key, v := range c {
				if !colorValue.MatchString(v) {
					t.Errorf("%s (%s): %s = %q", name, mode, key, v)
				}
			}
		}
	}
}

// Colours match opencode's own resolution of the theme files (values worked out from the JSON independently).
func TestResolvesLikeOpencode(t *testing.T) {
	for _, c := range []struct {
		theme                          string
		mode                           Mode
		accent, sidebar, panel, inline string
	}{
		{"tokyonight", Dark, "#82aaff", "#ff966c", "#1e2030", "#c3e88d"},
		{"tokyonight", Light, "#2e7de9", "#b15c00", "#d5d6db", "#587539"},
		{"catppuccin", Dark, "#89b4fa", "#f5c2e7", "#181825", "#a6e3a1"},
		{"github", Light, "#0969da", "#1b7c83", "#f6f8fa", "#bf3989"},
	} {
		got, err := Load(c.theme, c.mode, "")
		if err != nil {
			t.Fatal(err)
		}
		want := Colors{"accent": c.accent, "sidebar_selection": c.sidebar, "panel": c.panel, "inline_code": c.inline}
		for key, v := range want {
			if got[key] != v {
				t.Errorf("%s %s %s: got %s, want %s", c.theme, c.mode, key, got[key], v)
			}
		}
		// Text on the selection is the theme's background (opencode's rule for selected rows).
		if bg, _ := Load(c.theme, c.mode, ""); bg["selection_text"] == "" {
			t.Errorf("%s: no selection text", c.theme)
		}
	}
	if c, _ := Load("tokyonight", Dark, ""); c["selection_text"] != "#1a1b26" {
		t.Errorf("selection text should be tokyonight's background, got %s", c["selection_text"])
	}
}

func TestDefaultThemeIsBuiltinColours(t *testing.T) {
	c, err := Load(Default, Dark, "")
	if err != nil || len(c) != 0 {
		t.Errorf("tuimessenger keeps the built-in colours: %v %v", c, err)
	}
	if c, err := Load("", Dark, ""); err != nil || len(c) != 0 {
		t.Errorf("no theme = default: %v %v", c, err)
	}
}

func writeTheme(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name+".json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A theme file of yours: defs, references to other slots, dark/light, transparent, ANSI numbers.
func TestCustomTheme(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "mine", `{
		"defs": {"blue": "#0000ff", "night": "#101010"},
		"theme": {
			"primary": {"dark": "blue", "light": "#00f"},
			"accent": "primary",
			"text": 15,
			"textMuted": "#888888",
			"background": "night",
			"backgroundPanel": "none",
			"success": 2
		}
	}`)
	if !slices.Contains(Available(dir), "mine") {
		t.Errorf("custom theme not listed: %v", Available(dir))
	}
	c, err := Load("mine", Dark, dir)
	if err != nil {
		t.Fatal(err)
	}
	checks := map[string]string{
		"accent": "#0000ff", "sidebar_selection": "#0000ff", "bar_text": "#ffffff", "panel": "default",
		"toast_success": "#008000", "selection_text": "#101010", "toast_error": "#0000ff", "dim_text": "#888888",
	}
	for key, want := range checks {
		if c[key] != want {
			t.Errorf("%s: got %s, want %s", key, c[key], want)
		}
	}
	if light, _ := Load("mine", Light, dir); light["accent"] != "#0000ff" {
		t.Errorf("light: %s", light["accent"])
	}
}

// A file of yours wins over a built-in theme with the same name.
func TestCustomOverridesBuiltin(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "nord", `{"theme":{"primary":"#123456","accent":"#123456","text":"#ffffff","background":"#000000"}}`)
	if c, _ := Load("nord", Dark, dir); c["accent"] != "#123456" {
		t.Errorf("your nord should win: %s", c["accent"])
	}
}

func TestBadThemes(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "loop", `{"theme":{"primary":"accent","accent":"primary","text":"#fff","background":"#000"}}`)
	writeTheme(t, dir, "missing", `{"theme":{"primary":"nope","accent":"#fff","text":"#fff","background":"#000"}}`)
	writeTheme(t, dir, "partial", `{"theme":{"primary":"#fff"}}`)
	writeTheme(t, dir, "broken", `{"theme":`)
	for name, want := range map[string]string{"loop": "loop", "missing": `"nope"`, "partial": "missing", "broken": "unexpected end"} {
		if _, err := Load(name, Dark, dir); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want an error about %s", name, err, want)
		}
	}
	if _, err := Load("no-such-theme", Dark, dir); err == nil {
		t.Error("an unknown theme must be an error")
	}
}

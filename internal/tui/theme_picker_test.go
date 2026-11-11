package tui

import (
	"testing"

	"github.com/whyyor/tuimessenger/internal/rpc/client"
	"github.com/whyyor/tuimessenger/internal/rpc/store"
	"github.com/whyyor/tuimessenger/internal/tui/config"
	"github.com/whyyor/tuimessenger/internal/tui/themes"
)

func pickerView(t *testing.T) *MainView {
	t.Helper()
	keepStyle(t)
	if err := setTheme(themes.Default, themes.Dark); err != nil {
		t.Fatal(err)
	}
	return &MainView{matrix: &client.Client{Store: store.NewStore()}, config: &config.Config{Dir: t.TempDir()}}
}

func selectedTheme(p *CommandPalette) string { return p.rows[p.selected].item.title }

func builtinRestored(t *testing.T) {
	t.Helper()
	for key, color := range builtinColors {
		if *colorTargets()[key][0] != color {
			t.Errorf("%s not restored", key)
		}
	}
}

func TestThemePickerPreviewsAndUndoes(t *testing.T) {
	view := pickerView(t)
	p := NewThemePicker(view)
	if selectedTheme(p) != themes.Default || len(p.rows) != len(themes.Builtin()) || p.rows[0].item == nil {
		t.Fatalf("opens on the current theme, every theme, no headers: %s, %d rows", selectedTheme(p), len(p.rows))
	}
	p.move(1)
	if currentTheme != selectedTheme(p) || currentTheme == themes.Default {
		t.Errorf("moving previews the theme: on screen %s, selected %s", currentTheme, selectedTheme(p))
	}
	p.query = []rune("tokyo")
	p.refilter()
	if currentTheme != "tokyonight" || hex(ColorComposerBar) != "#82aaff" {
		t.Errorf("typing narrows and previews: %s %s", currentTheme, hex(ColorComposerBar))
	}
	p.cancel()
	if currentTheme != themes.Default {
		t.Errorf("escape puts the old theme back, got %s", currentTheme)
	}
	builtinRestored(t)
	if view.config.Theme != "" {
		t.Error("a cancelled preview isn't saved")
	}
}

func TestThemePickerKeeps(t *testing.T) {
	view := pickerView(t)
	p := NewThemePicker(view)
	p.query = []rune("nord")
	p.refilter()
	p.rows[p.selected].item.run()
	if currentTheme != "nord" || view.config.Theme != "nord" || view.config.ThemeMode != "dark" {
		t.Errorf("enter keeps and saves it: %s %q %q", currentTheme, view.config.Theme, view.config.ThemeMode)
	}
	if NewThemePicker(view).rows[0].item.detail == "current" && selectedTheme(NewThemePicker(view)) != "nord" {
		t.Error("reopened, the picker starts on the kept theme")
	}
}

func TestThemeModeToggle(t *testing.T) {
	view := pickerView(t)
	view.previewTheme("catppuccin", themes.Dark)
	view.toggleThemeMode()
	if currentThemeMode != themes.Light || hex(ColorComposerBar) != "#1e66f5" || view.config.ThemeMode != "light" {
		t.Errorf("light catppuccin: %s %s %q", currentThemeMode, hex(ColorComposerBar), view.config.ThemeMode)
	}
	view.toggleThemeMode()
	if currentThemeMode != themes.Dark || hex(ColorComposerBar) != "#89b4fa" {
		t.Errorf("back to dark: %s", hex(ColorComposerBar))
	}
}

package tui

import (
	"fmt"
	"slices"

	"github.com/whyyor/tuimessenger/internal/tui/config"
	"github.com/whyyor/tuimessenger/internal/tui/themes"
)

// NewThemePicker is Ctrl+p → Theme: every theme, previewed as the selection moves. Enter keeps it (saved like
// other Ctrl+p changes), Escape puts the old one back.
func NewThemePicker(view *MainView) *CommandPalette {
	original, mode := currentTheme, currentThemeMode
	custom := themes.Custom(config.ThemesDir())
	p := &CommandPalette{parent: view, flat: true, title: " \uf53f Theme ", hint: themePickerHint(len(userColors))}
	for _, name := range themes.Available(config.ThemesDir()) {
		detail := ""
		switch {
		case name == original:
			detail = "current"
		case slices.Contains(custom, name):
			detail = "yours"
		}
		p.items = append(p.items, &paletteItem{section: sectionCommands, title: name, detail: detail,
			run: func() { view.keepTheme(name, mode) }})
	}
	p.onMove = func(item *paletteItem) { view.previewTheme(item.title, mode) }
	p.onCancel = func() { view.previewTheme(original, mode) }
	p.refilter()
	if i := slices.IndexFunc(p.rows, func(r paletteRow) bool { return r.item != nil && r.item.title == original }); i >= 0 {
		p.selected = i
	}
	return p
}

// themePickerHint is the picker's hint; it warns when your colours cover the theme's, which otherwise looks like
// nothing changes.
func themePickerHint(yourColors int) string {
	switch yourColors {
	case 0:
		return "↑↓ preview  ⏎ keep  esc undo"
	case 1:
		return "1 colour in " + styleFileName + " wins over themes"
	}
	return fmt.Sprintf("%d colours in %s win over themes", yourColors, styleFileName)
}

// previewTheme puts a theme on screen without saving it.
func (view *MainView) previewTheme(name string, mode themes.Mode) {
	if name == currentTheme && mode == currentThemeMode {
		return
	}
	if err := setTheme(name, mode); err != nil {
		toasts.Show(toastError, err.Error())
		return
	}
	view.matrix.ForgetRendering()
	RequestRepaint()
}

// keepTheme saves the chosen theme, like other Ctrl+p changes.
func (view *MainView) keepTheme(name string, mode themes.Mode) {
	view.previewTheme(name, mode)
	view.config.Theme, view.config.ThemeMode = name, string(mode)
	pinnedNote("theme")
	view.config.Save() // inline: a small file, and the next toggle mustn't race it
	toasts.Show(toastSuccess, "Theme: "+name)
}

// toggleThemeMode switches the theme between its dark and light colours.
func (view *MainView) toggleThemeMode() {
	mode := themes.Light
	if currentThemeMode == themes.Light {
		mode = themes.Dark
	}
	view.previewTheme(currentTheme, mode)
	view.config.ThemeMode = string(mode)
	pinnedNote("theme_mode")
	view.config.Save()
	toasts.Show(toastSuccess, "Theme mode: "+string(mode))
}

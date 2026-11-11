package tui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"

	"github.com/whyyor/tuimessenger/internal/tui/config"
	"github.com/whyyor/tuimessenger/internal/tui/messages"
	"github.com/whyyor/tuimessenger/internal/tui/messages/html"
	"github.com/whyyor/tuimessenger/internal/tui/themes"
)

// styleFileName is tuimessenger.yaml, your settings file (see config.UserFilePath); an older copy next to
// terminal.yaml still works, and the one in ~/.config wins over it.
const styleFileName = config.UserFileName

// colorTargets are the variables each colour key sets.
func colorTargets() map[string][]*tcell.Color {
	return map[string][]*tcell.Color{
		"accent":                 {&ColorComposerBar},
		"selection":              {&messages.SelectionBackground, &ColorHighlight},
		"selection_text":         {&messages.SelectionForeground, &ColorOnHighlight},
		"sidebar_selection":      {&ColorSelectionBackground},
		"sidebar_selection_text": {&ColorSelectionText},
		"panel":                  {&ColorPanel},
		"selection_bar":          {&ColorBarBackground},
		"border":                 {&ColorBorder},
		"dim_text":               {&ColorStatusText},
		"bar_text":               {&ColorBarText},
		"switcher_pill":          {&ColorTitlePill},
		"switcher_pill_text":     {&ColorOnTitlePill},
		"palette_pill":           {&ColorPalettePill},
		"palette_accent":         {&ColorPaletteAccent},
		"palette_pill_text":      {&ColorOnPalettePill},
		"toast_success":          {&ColorMonokaiGreen},
		"toast_info":             {&ColorMonokaiPurple},
		"toast_warning":          {&ColorMonokaiYellow},
		"toast_error":            {&ColorMonokaiRed},
		"unread_badge":           {&ColorUnreadBadge},
		"unread_mention":         {&ColorUnreadHighlight},
		"pill_yellow":            {&ColorPillYellow},
		"pill_pink":              {&ColorPillPink},
		"pill_green":             {&ColorPillGreen},
		"pill_blue":              {&ColorPillBlue},
		"pill_violet":            {&ColorPillViolet},
		"pill_text":              {&ColorOnPill},
		"code_background":        {&html.CodeBackground},
		"inline_code":            {&html.InlineCodeText},
	}
}

// builtinColors are the tuimessenger theme's colours: the variables' values before any theme or file changed them.
var builtinColors = func() map[string]tcell.Color {
	out := map[string]tcell.Color{}
	for key, targets := range colorTargets() {
		out[key] = *targets[0]
	}
	return out
}()

// pinnedPrefs are the preference keys tuimessenger.yaml sets; Ctrl+p toggles of those only last until restart.
// "theme" is there when the file sets the theme.
var pinnedPrefs = map[string]bool{}

// userColors are tuimessenger.yaml's colours, kept so a theme change re-applies them on top.
var userColors = map[string]string{}

// currentTheme is the theme on screen, and the mode it uses.
var currentTheme, currentThemeMode = themes.Default, themes.Dark

// applyColors sets every colour: the built-in ones, then the theme's, then yours, then the ones that follow another
// colour unless the theme or you set them. It starts from scratch, so switching themes leaves nothing behind.
func applyColors(theme themes.Colors, yours map[string]string) {
	targets := colorTargets()
	set := map[string]tcell.Color{}
	apply := func(values map[string]string) {
		for key, raw := range values {
			if color, err := config.ParseColor(raw); err == nil && targets[key] != nil {
				set[key] = color
			}
		}
	}
	apply(theme)
	apply(yours)
	for key, color := range builtinColors {
		value, ok := set[key]
		if !ok {
			value = color
		}
		for _, target := range targets[key] {
			*target = value
		}
	}
	for _, c := range config.ColorOptions {
		if _, ok := set[c.Key]; ok || c.Follows == "" {
			continue
		}
		if parent, ok := set[c.Follows]; ok {
			set[c.Key] = parent
			for _, target := range targets[c.Key] {
				*target = parent
			}
		}
	}
	rebuildToastColors()
}

// setTheme loads a theme and puts it on screen with your colours on top.
func setTheme(name string, mode themes.Mode) error {
	colors, err := themes.Load(name, mode, config.ThemesDir())
	if err != nil {
		return err
	}
	applyColors(colors, userColors)
	currentTheme, currentThemeMode = name, mode
	return nil
}

// applyStyleFile reads tuimessenger.yaml (the one next to terminal.yaml, then ~/.config's, which wins), writes the
// commented template when there's neither, and applies theme, colours, preferences and keys. It returns the
// problems found (shown as a toast once the UI is up).
func applyStyleFile(cfg *config.Config) []string {
	legacy := filepath.Join(cfg.Dir, styleFileName)
	paths := []string{legacy, config.UserFilePath()}
	found := false
	var problems []string
	theme, mode := cfg.Theme, cfg.ThemeMode
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		} else if err != nil {
			problems = append(problems, shortPath(path)+": "+err.Error())
			continue
		}
		found = true
		uf, fileProblems := config.ParseUserFile(data, config.ThemesDir())
		fileProblems = append(fileProblems, cfg.ApplyPreferences(uf)...)
		for _, p := range fileProblems {
			problems = append(problems, shortPath(path)+": "+p)
		}
		cfg.ApplyKeybindings(uf.Keybindings)
		for key := range uf.Preferences {
			pinnedPrefs[key] = true
		}
		for key, value := range uf.Colors {
			userColors[key] = value
		}
		if uf.Set["theme"] {
			theme = uf.Theme
			pinnedPrefs["theme"] = true
		}
		if uf.Set["theme_mode"] {
			mode = uf.ThemeMode
			pinnedPrefs["theme_mode"] = true
		}
	}
	if !found {
		_ = config.WriteTemplate(config.UserFilePath())
	}
	if theme == "" {
		theme = themes.Default
	}
	if err := setTheme(theme, themeMode(mode)); err != nil {
		problems = append(problems, err.Error())
		_ = setTheme(themes.Default, themeMode(mode))
	}
	return problems
}

func themeMode(mode string) themes.Mode {
	if mode == string(themes.Light) {
		return themes.Light
	}
	return themes.Dark
}

// shortPath writes a path under your home folder as ~/….
func shortPath(path string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

func rebuildToastColors() {
	toastColors[toastInfo] = ColorMonokaiPurple
	toastColors[toastSuccess] = ColorMonokaiGreen
	toastColors[toastWarning] = ColorMonokaiYellow
	toastColors[toastError] = ColorMonokaiRed
}

// pinnedNote warns that a Ctrl+p change won't survive a restart.
func pinnedNote(key string) {
	if pinnedPrefs[key] {
		toasts.Show(toastWarning, "Changed until restart: "+styleFileName+" sets "+key)
	}
}

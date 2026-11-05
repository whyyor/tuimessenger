package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/gdamore/tcell/v2"
	"gopkg.in/yaml.v3"

	"go.mau.fi/gomuks/tui/config"
	"go.mau.fi/gomuks/tui/messages"
	"go.mau.fi/gomuks/tui/messages/html"
)

// styleFileName is your own settings file next to terminal.yaml: colours and
// preferences that win over terminal.yaml at startup. gomuks only reads it,
// so it can live in a dotfiles repo (terminal.yaml holds the password).
const styleFileName = "gomuks.yaml"

// styleColor is one themable colour: the variables it sets, and the colour
// it follows when the file doesn't set it.
type styleColor struct {
	key     string
	targets []*tcell.Color
	follows string
}

// styleColors lists every colour gomuks.yaml can set, in file order.
func styleColors() []styleColor {
	return []styleColor{
		{key: "accent", targets: []*tcell.Color{&ColorComposerBar}},
		{key: "selection", targets: []*tcell.Color{&messages.SelectionBackground, &ColorHighlight}},
		{key: "selection_text", targets: []*tcell.Color{&messages.SelectionForeground, &ColorOnHighlight}},
		{key: "sidebar_selection", targets: []*tcell.Color{&ColorSelectionBackground}},
		{key: "sidebar_selection_text", targets: []*tcell.Color{&ColorSelectionText}},
		{key: "panel", targets: []*tcell.Color{&ColorPanel}},
		{key: "selection_bar", targets: []*tcell.Color{&ColorBarBackground}},
		{key: "border", targets: []*tcell.Color{&ColorBorder}},
		{key: "dim_text", targets: []*tcell.Color{&ColorStatusText}},
		{key: "bar_text", targets: []*tcell.Color{&ColorBarText}},
		{key: "switcher_pill", targets: []*tcell.Color{&ColorTitlePill}, follows: "sidebar_selection"},
		{key: "switcher_pill_text", targets: []*tcell.Color{&ColorOnTitlePill}, follows: "sidebar_selection_text"},
		{key: "palette_pill", targets: []*tcell.Color{&ColorPalettePill}, follows: "toast_success"},
		{key: "palette_accent", targets: []*tcell.Color{&ColorPaletteAccent}, follows: "palette_pill"},
		{key: "palette_pill_text", targets: []*tcell.Color{&ColorOnPalettePill}},
		{key: "toast_success", targets: []*tcell.Color{&ColorMonokaiGreen}},
		{key: "toast_info", targets: []*tcell.Color{&ColorMonokaiPurple}},
		{key: "toast_warning", targets: []*tcell.Color{&ColorMonokaiYellow}},
		{key: "toast_error", targets: []*tcell.Color{&ColorMonokaiRed}},
		{key: "unread_badge", targets: []*tcell.Color{&ColorUnreadBadge}},
		{key: "unread_mention", targets: []*tcell.Color{&ColorUnreadHighlight}},
		{key: "pill_yellow", targets: []*tcell.Color{&ColorPillYellow}},
		{key: "pill_pink", targets: []*tcell.Color{&ColorPillPink}},
		{key: "pill_green", targets: []*tcell.Color{&ColorPillGreen}},
		{key: "pill_blue", targets: []*tcell.Color{&ColorPillBlue}},
		{key: "pill_violet", targets: []*tcell.Color{&ColorPillViolet}},
		{key: "pill_text", targets: []*tcell.Color{&ColorOnPill}},
		{key: "code_background", targets: []*tcell.Color{&html.CodeBackground}},
		{key: "inline_code", targets: []*tcell.Color{&html.InlineCodeText}},
	}
}

// pinnedPrefs are the preference keys gomuks.yaml sets; Ctrl+p toggles of
// those only last until restart.
var pinnedPrefs = map[string]bool{}

type styleFile struct {
	Colors      map[string]string `yaml:"colors"`
	Preferences yaml.Node         `yaml:"preferences"`
}

// applyStyleFile reads gomuks.yaml from the config dir, if there is one, and
// returns the problems found (shown as a toast once the UI is up).
func applyStyleFile(cfg *config.Config) []string {
	data, err := os.ReadFile(filepath.Join(cfg.Dir, styleFileName))
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return []string{err.Error()}
	}
	return applyStyle(data, &cfg.Preferences)
}

func applyStyle(data []byte, prefs *config.UserPreferences) []string {
	var file styleFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return []string{"can't read it: " + err.Error()}
	}
	var problems []string
	// Colours, then the ones that follow another colour unless set.
	set := map[string]tcell.Color{}
	known := map[string]bool{}
	for _, c := range styleColors() {
		known[c.key] = true
		raw, ok := file.Colors[c.key]
		if !ok {
			continue
		}
		color, err := parseStyleColor(raw)
		if err != nil {
			problems = append(problems, fmt.Sprintf("colors.%s: %v", c.key, err))
			continue
		}
		set[c.key] = color
		for _, target := range c.targets {
			*target = color
		}
	}
	for key := range file.Colors {
		if !known[key] {
			problems = append(problems, "unknown colour "+key)
		}
	}
	for _, c := range styleColors() {
		if _, ok := set[c.key]; ok || c.follows == "" {
			continue
		}
		if parent, ok := set[c.follows]; ok {
			set[c.key] = parent
			for _, target := range c.targets {
				*target = parent
			}
		}
	}
	rebuildToastColors()

	// Preferences: only the keys present override terminal.yaml.
	if file.Preferences.Kind == yaml.MappingNode {
		valid := prefKeys()
		for i := 0; i+1 < len(file.Preferences.Content); i += 2 {
			key, value := file.Preferences.Content[i].Value, file.Preferences.Content[i+1]
			if !slices.Contains(valid, key) {
				problems = append(problems, "unknown preference "+key)
				continue
			}
			// Readable names for the empty defaults.
			if (key == "timestamp_style" && value.Value == "relative") || (key == "file_picker" && value.Value == "finder") {
				value.Value = ""
			}
			if err := decodePref(prefs, key, value); err != nil {
				problems = append(problems, fmt.Sprintf("preferences.%s: %v", key, err))
				continue
			}
			pinnedPrefs[key] = true
		}
	}
	return problems
}

// decodePref sets one yaml-tagged field of the preferences.
func decodePref(prefs *config.UserPreferences, key string, value *yaml.Node) error {
	v := reflect.ValueOf(prefs).Elem()
	for i := 0; i < v.NumField(); i++ {
		if strings.Split(v.Type().Field(i).Tag.Get("yaml"), ",")[0] == key {
			return value.Decode(v.Field(i).Addr().Interface())
		}
	}
	return fmt.Errorf("not a preference")
}

func prefKeys() []string {
	t := reflect.TypeOf(config.UserPreferences{})
	var keys []string
	for i := 0; i < t.NumField(); i++ {
		if tag := strings.Split(t.Field(i).Tag.Get("yaml"), ",")[0]; tag != "" && tag != "-" {
			keys = append(keys, tag)
		}
	}
	return keys
}

// parseStyleColor accepts "#rrggbb" or a colour name such as "red".
func parseStyleColor(raw string) (tcell.Color, error) {
	raw = strings.TrimSpace(raw)
	if raw == "default" {
		return tcell.ColorDefault, nil
	}
	color := tcell.GetColor(raw)
	if color == tcell.ColorDefault {
		return color, fmt.Errorf("%q isn't a colour (use \"#rrggbb\" or a name)", raw)
	}
	return color, nil
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

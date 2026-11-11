package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"codeberg.org/tslocum/cbind"
	"github.com/gdamore/tcell/v2"
	"github.com/lithammer/fuzzysearch/fuzzy"
	"gopkg.in/yaml.v3"

	"github.com/whyyor/tuimessenger/internal/tui/themes"
)

// UserFileName is your settings file: theme, colours, preferences and keys. tuimessenger only reads it (apart from
// writing a commented template when there's none), so it can live in your dotfiles.
const UserFileName = "tuimessenger.yaml"

// SchemaURL is the JSON Schema editors use to check and complete tuimessenger.yaml.
const SchemaURL = "https://raw.githubusercontent.com/whyyor/tuimessenger/main/docs/config.schema.json"

// DocsURL explains every setting.
const DocsURL = "https://github.com/whyyor/tuimessenger/blob/main/docs/config.md"

// UserFilePath is $TUIMESSENGER_CONFIG, else $XDG_CONFIG_HOME/tuimessenger/tuimessenger.yaml, else
// ~/.config/tuimessenger/tuimessenger.yaml (on macOS too, like most terminal apps).
func UserFilePath() string {
	if path := os.Getenv("TUIMESSENGER_CONFIG"); path != "" {
		return path
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "tuimessenger", UserFileName)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "tuimessenger", UserFileName)
}

// ThemesDir holds your theme files (opencode's format), next to tuimessenger.yaml.
func ThemesDir() string {
	return filepath.Join(filepath.Dir(UserFilePath()), "themes")
}

// Option is one setting, as the schema, the docs, the template and the checks describe it.
type Option struct {
	Section string // "" (top level), "preferences" or "colors"
	Key     string
	Type    string // "boolean", "string" or "color"
	Enum    []string
	Default string // as written in the file
	Doc     string
	Follows string // colours: unset, it takes this colour's value
}

// ColorOptions are the colours tuimessenger.yaml can set, with the tuimessenger theme's values.
var ColorOptions = []Option{
	{Key: "accent", Default: "#a8a9eb", Doc: "Composer bar and prompts."},
	{Key: "selection", Default: "#8d70c2", Doc: "Background of the selected message and emoji."},
	{Key: "selection_text", Default: "#ffffff", Doc: "Text on the selection."},
	{Key: "sidebar_selection", Default: "#f92672", Doc: "The selected chat in the sidebar."},
	{Key: "sidebar_selection_text", Default: "#fff1f3", Doc: "Text of the selected chat."},
	{Key: "panel", Default: "#0e0e13", Doc: "Background of Ctrl+k, Ctrl+p and other popups."},
	{Key: "selection_bar", Default: "#1f1f28", Doc: "The selected row in Ctrl+k and Ctrl+p."},
	{Key: "border", Default: "#54546d", Doc: "Popup borders."},
	{Key: "dim_text", Default: "#8a8a94", Doc: "Status line, details and aliases."},
	{Key: "bar_text", Default: "#c8c9c5", Doc: "Text in bars and popups."},
	{Key: "switcher_pill", Default: "#f92672", Follows: "sidebar_selection", Doc: "Ctrl+k's title pill and its ▌."},
	{Key: "switcher_pill_text", Default: "#fff1f3", Follows: "sidebar_selection_text", Doc: "Text on Ctrl+k's pill."},
	{Key: "palette_pill", Default: "#adda78", Follows: "toast_success", Doc: "Ctrl+p's title pill."},
	{Key: "palette_accent", Default: "#adda78", Follows: "palette_pill", Doc: "Ctrl+p's ▌ and magnifier."},
	{Key: "palette_pill_text", Default: "#000000", Doc: "Text on Ctrl+p's pill."},
	{Key: "toast_success", Default: "#adda78", Doc: "Success toasts."},
	{Key: "toast_info", Default: "#a8a9eb", Doc: "Info toasts."},
	{Key: "toast_warning", Default: "#f9cc6c", Doc: "Warning toasts."},
	{Key: "toast_error", Default: "#fd6883", Doc: "Error toasts."},
	{Key: "unread_badge", Default: "#7fb4ca", Doc: "Unread counts."},
	{Key: "unread_mention", Default: "#e6c384", Doc: "Unread counts with a mention of you."},
	{Key: "pill_yellow", Default: "#e6c384", Doc: "Pills such as the Ctrl+m hint."},
	{Key: "pill_pink", Default: "#d27e99", Doc: "The search pill."},
	{Key: "pill_green", Default: "#98bb6c", Doc: "Green pills."},
	{Key: "pill_blue", Default: "#7e9cd8", Doc: "Blue pills."},
	{Key: "pill_violet", Default: "#a8a9eb", Doc: "The peek pill."},
	{Key: "pill_text", Default: "#1f1f28", Doc: "Text on pills."},
	{Key: "code_background", Default: "#1a1a22", Doc: "Code blocks and inline code."},
	{Key: "inline_code", Default: "#f38d70", Doc: "Inline code text."},
}

// TopOptions are the top-level settings besides colors, preferences and keybindings.
func TopOptions() []Option {
	return []Option{
		{Key: "theme", Type: "string", Enum: themes.Builtin(), Default: themes.Default,
			Doc: "Colour theme: tuimessenger, any of opencode's themes, or one of yours (themes/<name>.json next to this file, in opencode's format). Ctrl+p → Theme previews them live."},
		{Key: "theme_mode", Type: "string", Enum: []string{"dark", "light"}, Default: "dark",
			Doc: "Use a theme's dark or light colours (most opencode themes have both)."},
	}
}

// PreferenceOptions are the preferences tuimessenger.yaml can set, in file order.
func PreferenceOptions() []Option {
	var opts []Option
	add := func(t reflect.Type) {
		for i := range t.NumField() {
			f := t.Field(i)
			key := strings.Split(f.Tag.Get("yaml"), ",")[0]
			if f.Tag.Get("doc") == "" || f.Tag.Get("user") == "-" {
				continue
			}
			o := Option{Section: "preferences", Key: key, Doc: f.Tag.Get("doc"), Default: f.Tag.Get("default")}
			switch f.Type.Kind() {
			case reflect.Bool:
				o.Type = "boolean"
				if o.Default == "" {
					o.Default = "false"
				}
			default:
				o.Type = "string"
			}
			if enum := f.Tag.Get("enum"); enum != "" {
				o.Enum = strings.Split(enum, ",")
			}
			opts = append(opts, o)
		}
	}
	add(reflect.TypeOf(UserPreferences{}))
	add(reflect.TypeOf(Config{}))
	return opts
}

// Options is every setting.
func Options() []Option {
	opts := TopOptions()
	opts = append(opts, PreferenceOptions()...)
	for _, c := range ColorOptions {
		c.Section, c.Type = "colors", "color"
		opts = append(opts, c)
	}
	return opts
}

// KeybindingSections are the keybindings tables and what they're for.
var KeybindingSections = []Option{
	{Key: "main", Doc: "Everywhere (unless a popup is open)."},
	{Key: "room", Doc: "In a chat."},
	{Key: "visual", Doc: "Selecting messages (visual mode)."},
	{Key: "modal", Doc: "Popups and lists."},
}

// KeybindingActions are the actions a keybindings table can use, from the default bindings.
func KeybindingActions() map[string][]string {
	var raw RawKeybindings
	_ = yaml.Unmarshal([]byte(DefaultKeybindings), &raw)
	out := map[string][]string{}
	for section, m := range map[string]map[string]string{"main": raw.Main, "room": raw.Room, "visual": raw.Visual, "modal": raw.Modal} {
		var actions []string
		for _, a := range m {
			if !slices.Contains(actions, a) {
				actions = append(actions, a)
			}
		}
		slices.Sort(actions)
		out[section] = actions
	}
	return out
}

// UserFile is tuimessenger.yaml, parsed.
type UserFile struct {
	Theme       string
	ThemeMode   string
	Colors      map[string]string
	Preferences map[string]*yaml.Node
	Keybindings RawKeybindings
	// Set are the keys present, as "theme", "preferences.file_picker", "colors.accent"…
	Set map[string]bool
}

// unbind removes a default binding: "Ctrl+s": none.
const unbind = "none"

// ParseUserFile reads tuimessenger.yaml and checks it against the options. Problems (unknown keys with a "did you
// mean", wrong types, bad values) are returned; the rest still applies.
func ParseUserFile(data []byte, themesDir string) (*UserFile, []string) {
	uf := &UserFile{Colors: map[string]string{}, Preferences: map[string]*yaml.Node{}, Set: map[string]bool{}}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return uf, []string{"can't read it: " + err.Error()}
	}
	if len(root.Content) == 0 {
		return uf, nil
	}
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return uf, []string{"it should be a list of settings (key: value)"}
	}
	var problems []string
	bad := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	top := []string{"theme", "theme_mode", "colors", "preferences", "keybindings"}
	for i := 0; i+1 < len(doc.Content); i += 2 {
		key, value := doc.Content[i].Value, doc.Content[i+1]
		line := doc.Content[i].Line
		switch key {
		case "theme":
			uf.Theme = value.Value
			if !slices.Contains(themes.Available(themesDir), uf.Theme) {
				bad("line %d: theme %q doesn't exist%s", line, uf.Theme, didYouMean(uf.Theme, themes.Available(themesDir)))
				uf.Theme = ""
				continue
			}
		case "theme_mode":
			if value.Value != "dark" && value.Value != "light" {
				bad("line %d: theme_mode is dark or light, not %q", line, value.Value)
				continue
			}
			uf.ThemeMode = value.Value
		case "colors":
			problems = append(problems, uf.parseColors(value)...)
		case "preferences":
			problems = append(problems, uf.parsePreferences(value)...)
		case "keybindings":
			problems = append(problems, uf.parseKeybindings(value)...)
		default:
			bad("line %d: unknown setting %q%s", line, key, didYouMean(key, top))
			continue
		}
		uf.Set[key] = true
	}
	return uf, problems
}

func mapping(node *yaml.Node, name string) ([]string, bool) {
	if node.Kind != yaml.MappingNode {
		return []string{fmt.Sprintf("line %d: %s should be a list of key: value", node.Line, name)}, false
	}
	return nil, true
}

func (uf *UserFile) parseColors(node *yaml.Node) []string {
	problems, ok := mapping(node, "colors")
	if !ok {
		return problems
	}
	var known []string
	for _, c := range ColorOptions {
		known = append(known, c.Key)
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i].Value, node.Content[i+1].Value
		line := node.Content[i].Line
		if !slices.Contains(known, key) {
			problems = append(problems, fmt.Sprintf("line %d: unknown colour %q%s", line, key, didYouMean(key, known)))
			continue
		}
		if _, err := ParseColor(value); err != nil {
			problems = append(problems, fmt.Sprintf("line %d: colors.%s: %v", line, key, err))
			continue
		}
		uf.Colors[key] = value
		uf.Set["colors."+key] = true
	}
	return problems
}

func (uf *UserFile) parsePreferences(node *yaml.Node) []string {
	problems, ok := mapping(node, "preferences")
	if !ok {
		return problems
	}
	opts := PreferenceOptions()
	var known []string
	for _, o := range opts {
		known = append(known, o.Key)
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i].Value, node.Content[i+1]
		line := node.Content[i].Line
		idx := slices.IndexFunc(opts, func(o Option) bool { return o.Key == key })
		if idx < 0 {
			problems = append(problems, fmt.Sprintf("line %d: unknown preference %q%s", line, key, didYouMean(key, known)))
			continue
		}
		o := opts[idx]
		if o.Type == "boolean" {
			var b bool
			if value.Decode(&b) != nil {
				problems = append(problems, fmt.Sprintf("line %d: preferences.%s is true or false, not %q", line, key, value.Value))
				continue
			}
		} else if o.Enum != nil && !slices.Contains(o.Enum, value.Value) {
			problems = append(problems, fmt.Sprintf("line %d: preferences.%s is one of %s, not %q", line, key, strings.Join(o.Enum, ", "), value.Value))
			continue
		}
		uf.Preferences[key] = value
		uf.Set["preferences."+key] = true
	}
	return problems
}

func (uf *UserFile) parseKeybindings(node *yaml.Node) []string {
	problems, ok := mapping(node, "keybindings")
	if !ok {
		return problems
	}
	actions := KeybindingActions()
	var sections []string
	for _, s := range KeybindingSections {
		sections = append(sections, s.Key)
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		section, table := node.Content[i].Value, node.Content[i+1]
		line := node.Content[i].Line
		if !slices.Contains(sections, section) {
			problems = append(problems, fmt.Sprintf("line %d: unknown keybindings table %q%s", line, section, didYouMean(section, sections)))
			continue
		}
		if p, ok := mapping(table, "keybindings."+section); !ok {
			problems = append(problems, p...)
			continue
		}
		parsed := map[string]string{}
		for j := 0; j+1 < len(table.Content); j += 2 {
			key, action := table.Content[j].Value, table.Content[j+1].Value
			kline := table.Content[j].Line
			if _, _, _, err := cbind.Decode(key); err != nil {
				problems = append(problems, fmt.Sprintf("line %d: %q isn't a key (e.g. Ctrl+k, Alt+Enter, F1)", kline, key))
				continue
			}
			if action != unbind && !slices.Contains(actions[section], action) {
				problems = append(problems, fmt.Sprintf("line %d: unknown %s action %q%s", kline, section, action, didYouMean(action, actions[section])))
				continue
			}
			parsed[key] = action
		}
		switch section {
		case "main":
			uf.Keybindings.Main = parsed
		case "room":
			uf.Keybindings.Room = parsed
		case "visual":
			uf.Keybindings.Visual = parsed
		case "modal":
			uf.Keybindings.Modal = parsed
		}
	}
	return problems
}

// didYouMean suggests the closest known name for a typo, or nothing.
func didYouMean(got string, known []string) string {
	best, bestDist := "", 4
	for _, k := range known {
		if d := fuzzy.LevenshteinDistance(strings.ToLower(got), k); d < bestDist {
			best, bestDist = k, d
		}
	}
	if best == "" {
		return ""
	}
	return fmt.Sprintf(" (did you mean %q?)", best)
}

// ParseColor accepts "#rrggbb", a colour name such as "red", or "default" (the terminal's own colour).
func ParseColor(raw string) (tcell.Color, error) {
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

// ApplyPreferences sets the preferences and keybindings tuimessenger.yaml has; they win over terminal.yaml.
func (config *Config) ApplyPreferences(uf *UserFile) []string {
	var problems []string
	for key, value := range uf.Preferences {
		v := *value
		// Readable names for the empty defaults.
		if (key == "timestamp_style" && v.Value == "relative") || (key == "file_picker" && v.Value == "finder") ||
			((key == "inline_url_mode" || key == "image_protocol") && v.Value == "auto") {
			v.Value = ""
		}
		if err := decodeField(&config.Preferences, key, &v); err == nil {
			continue
		}
		if err := decodeField(config, key, &v); err != nil {
			problems = append(problems, fmt.Sprintf("preferences.%s: %v", key, err))
		}
	}
	return problems
}

// decodeField sets the yaml-tagged field key of target.
func decodeField(target any, key string, value *yaml.Node) error {
	v := reflect.ValueOf(target).Elem()
	for i := range v.NumField() {
		if strings.Split(v.Type().Field(i).Tag.Get("yaml"), ",")[0] == key {
			return value.Decode(v.Field(i).Addr().Interface())
		}
	}
	return fmt.Errorf("not a setting")
}

// MergeKeybindings puts your bindings over the defaults; "none" removes a default one.
func MergeKeybindings(base *RawKeybindings, yours RawKeybindings) {
	merge := func(dst *map[string]string, src map[string]string) {
		if len(src) == 0 {
			return
		}
		if *dst == nil {
			*dst = map[string]string{}
		}
		for key, action := range src {
			// Keys are compared as parsed, so "ctrl+k" replaces the default "Ctrl+k".
			for existing := range *dst {
				if sameKey(existing, key) {
					delete(*dst, existing)
				}
			}
			if action != unbind {
				(*dst)[key] = action
			}
		}
	}
	merge(&base.Main, yours.Main)
	merge(&base.Room, yours.Room)
	merge(&base.Visual, yours.Visual)
	merge(&base.Modal, yours.Modal)
}

func sameKey(a, b string) bool {
	ma, ka, ca, errA := cbind.Decode(a)
	mb, kb, cb, errB := cbind.Decode(b)
	return errA == nil && errB == nil && ma == mb && ka == kb && ca == cb
}

// Template is the commented tuimessenger.yaml written when there's none: every setting, commented out at its default.
func Template() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# yaml-language-server: $schema=%s\n", SchemaURL)
	b.WriteString("# tuimessenger settings. tuimessenger only reads this file, so it can live in your dotfiles.\n")
	b.WriteString("# Uncomment a line to change it; restart tuimessenger to apply. Everything: " + DocsURL + "\n")
	b.WriteString("# Check it with: tuimessenger config\n\n")
	for _, o := range TopOptions() {
		writeComment(&b, "", o)
		fmt.Fprintf(&b, "# %s: %s\n\n", o.Key, o.Default)
	}
	b.WriteString("# Your preferences win over the ones Ctrl+p saves.\n# preferences:\n")
	for _, o := range PreferenceOptions() {
		writeComment(&b, "  ", o)
		fmt.Fprintf(&b, "#   %s: %s\n", o.Key, o.Default)
	}
	b.WriteString("\n# Colours (\"#rrggbb\", a name, or default) win over the theme's.\n# colors:\n")
	for _, c := range ColorOptions {
		fmt.Fprintf(&b, "#   %s: \"%s\" # %s\n", c.Key, c.Default, strings.TrimSuffix(c.Doc, "."))
	}
	b.WriteString("\n# Keys: key: action, or key: none to remove a default. Press ? or F1 in tuimessenger for all of them.\n")
	b.WriteString("# keybindings:\n#   main:\n#     'Ctrl+n': next_room\n#   room:\n#     'Alt+v': none\n")
	return b.String()
}

func writeComment(b *strings.Builder, indent string, o Option) {
	doc := o.Doc
	if o.Enum != nil && o.Key != "theme" {
		doc += " One of: " + strings.Join(o.Enum, ", ") + "."
	}
	for _, line := range wrap(doc, 100) {
		fmt.Fprintf(b, "# %s# %s\n", indent, line)
	}
}

func wrap(s string, width int) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		if line != "" && len(line)+1+len(word) > width {
			lines = append(lines, line)
			line = word
		} else if line == "" {
			line = word
		} else {
			line += " " + word
		}
	}
	return append(lines, line)
}

// WriteTemplate creates tuimessenger.yaml at path with the template if there's no file there yet.
func WriteTemplate(path string) error {
	if _, err := os.Lstat(path); err == nil || !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(Template()), 0o644)
}

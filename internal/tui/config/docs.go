package config

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/whyyor/tuimessenger/internal/tui/themes"
)

// Schema is tuimessenger.yaml's JSON Schema (docs/config.schema.json), built from the options so it can't drift.
func Schema() []byte {
	obj := func(desc string, props map[string]any) map[string]any {
		return map[string]any{"type": "object", "description": desc, "additionalProperties": false, "properties": props}
	}
	option := func(o Option) map[string]any {
		p := map[string]any{"description": o.Doc}
		switch o.Type {
		case "boolean":
			p["type"] = "boolean"
			p["default"] = o.Default == "true"
		case "color":
			p["type"] = "string"
			p["default"] = o.Default
			p["pattern"] = `^(#[0-9a-fA-F]{6}|[a-z]+)$`
		default:
			p["type"] = "string"
			p["default"] = o.Default
			if o.Enum != nil {
				p["enum"] = o.Enum
			}
		}
		return p
	}
	top := map[string]any{}
	for _, o := range TopOptions() {
		p := option(o)
		if o.Key == "theme" {
			// Your own themes are allowed too, so the built-in names are suggestions, not the only values.
			delete(p, "enum")
			p["anyOf"] = []any{map[string]any{"enum": o.Enum}, map[string]any{"type": "string"}}
		}
		top[o.Key] = p
	}
	prefs := map[string]any{}
	for _, o := range PreferenceOptions() {
		prefs[o.Key] = option(o)
	}
	colors := map[string]any{}
	for _, c := range ColorOptions {
		c.Type = "color"
		colors[c.Key] = option(c)
	}
	actions := KeybindingActions()
	keys := map[string]any{}
	for _, s := range KeybindingSections {
		keys[s.Key] = map[string]any{
			"type":        "object",
			"description": s.Doc + ` Key (e.g. "Ctrl+k", "Alt+Enter", "F1") to action; "none" removes a default binding.`,
			"additionalProperties": map[string]any{
				"type": "string",
				"enum": append(slices.Clone(actions[s.Key]), unbind),
			},
		}
	}
	top["preferences"] = obj("Preferences; they win over the ones Ctrl+p saves.", prefs)
	top["colors"] = obj(`Colours ("#rrggbb", a name, or default for the terminal's own); they win over the theme's.`, colors)
	top["keybindings"] = obj("Keys, over the defaults. F1 (or Ctrl+p → Keys and commands) shows them all.", keys)
	schema := map[string]any{
		"$schema":              "http://json-schema.org/draft-07/schema#",
		"$id":                  SchemaURL,
		"title":                "tuimessenger.yaml",
		"description":          "tuimessenger settings: " + DocsURL,
		"type":                 "object",
		"additionalProperties": false,
		"properties":           top,
	}
	data, _ := json.MarshalIndent(schema, "", "  ")
	return append(data, '\n')
}

// DocsStart and DocsEnd mark the generated reference in docs/config.md.
const (
	DocsStart = "<!-- generated: options start (UPDATE_DOCS=1 go test ./internal/tui/config/) -->"
	DocsEnd   = "<!-- generated: options end -->"
)

func cell(s string) string { return strings.ReplaceAll(s, "|", `\|`) }

// OptionsMarkdown is the reference part of docs/config.md: every setting, theme and default key.
func OptionsMarkdown() string {
	var b strings.Builder
	b.WriteString("### Top level\n\n| Setting | Default | Description |\n|---|---|---|\n")
	for _, o := range TopOptions() {
		doc := o.Doc
		if o.Key != "theme" && o.Enum != nil {
			doc += " One of: `" + strings.Join(o.Enum, "`, `") + "`."
		}
		fmt.Fprintf(&b, "| `%s` | `%s` | %s |\n", o.Key, o.Default, cell(doc))
	}
	b.WriteString("\n### `preferences`\n\n| Preference | Default | Values | Description |\n|---|---|---|---|\n")
	for _, o := range PreferenceOptions() {
		values := "`true`, `false`"
		if o.Type != "boolean" {
			values = "`" + strings.Join(o.Enum, "`, `") + "`"
		}
		fmt.Fprintf(&b, "| `%s` | `%s` | %s | %s |\n", o.Key, o.Default, values, cell(o.Doc))
	}
	b.WriteString("\n### `colors`\n\nThe defaults are the `tuimessenger` theme's. A colour marked *follows* takes the other colour's value when " +
		"neither the theme nor you set it.\n\n| Colour | Default | Description |\n|---|---|---|\n")
	for _, c := range ColorOptions {
		doc := c.Doc
		if c.Follows != "" {
			doc += " *Follows `" + c.Follows + "`.*"
		}
		fmt.Fprintf(&b, "| `%s` | `%s` | %s |\n", c.Key, c.Default, cell(doc))
	}
	b.WriteString("\n### Themes\n\n")
	var names []string
	for _, n := range themes.Builtin() {
		names = append(names, "`"+n+"`")
	}
	b.WriteString(strings.Join(names, ", ") + "\n")
	b.WriteString("\n### `keybindings`\n\nEvery action and its default keys, by table. Keys are written like `Ctrl+k`, `Alt+Enter`, " +
		"`Shift+Tab`, `F1`, `Space` or a single character; a single character only acts when you aren't typing. `Ctrl+m` " +
		"and `Ctrl+;` are bound as `Ctrl+^` and `Ctrl+_`, which Ghostty sends for them with a keybind.\n")
	var raw RawKeybindings
	_ = yaml.Unmarshal([]byte(DefaultKeybindings), &raw)
	tables := map[string]map[string]string{"main": raw.Main, "room": raw.Room, "visual": raw.Visual, "modal": raw.Modal}
	for _, s := range KeybindingSections {
		fmt.Fprintf(&b, "\n#### `%s`: %s\n\n| Action | Default keys | What it does |\n|---|---|---|\n", s.Key, strings.TrimSuffix(s.Doc, "."))
		byAction := map[string][]string{}
		for key, action := range tables[s.Key] {
			byAction[action] = append(byAction[action], "`"+key+"`")
		}
		for _, a := range Actions[s.Key] {
			slices.Sort(byAction[a.Name])
			keys := strings.Join(byAction[a.Name], ", ")
			if keys == "" {
				keys = "*none*"
			}
			fmt.Fprintf(&b, "| `%s` | %s | %s |\n", a.Name, keys, cell(a.Doc))
		}
	}
	b.WriteString("\n#### Fixed keys\n\nThese can't be changed.\n\n| Where | Keys | What it does |\n|---|---|---|\n")
	for _, f := range FixedKeys {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", f.Group, cell(f.Keys), cell(f.Doc))
	}
	return b.String()
}

// ReplaceGenerated puts the generated reference between the markers of docs/config.md.
func ReplaceGenerated(doc string) (string, error) {
	start, end := strings.Index(doc, DocsStart), strings.Index(doc, DocsEnd)
	if start < 0 || end < start {
		return "", fmt.Errorf("docs/config.md needs the markers %q and %q", DocsStart, DocsEnd)
	}
	return doc[:start+len(DocsStart)] + "\n\n" + OptionsMarkdown() + "\n" + doc[end:], nil
}

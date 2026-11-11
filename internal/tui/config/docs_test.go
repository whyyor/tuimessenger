package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	schemaPath = "../../../docs/config.schema.json"
	docsPath   = "../../../docs/config.md"
)

// The schema and the docs' reference are generated from the options; UPDATE_DOCS=1 rewrites them.
func TestSchemaAndDocsUpToDate(t *testing.T) {
	update := os.Getenv("UPDATE_DOCS") != ""
	schema := Schema()
	if update {
		if err := os.WriteFile(schemaPath, schema, 0o644); err != nil {
			t.Fatal(err)
		}
	} else if old, err := os.ReadFile(schemaPath); err != nil || string(old) != string(schema) {
		t.Errorf("docs/config.schema.json is out of date: run UPDATE_DOCS=1 go test ./internal/tui/config/ (%v)", err)
	}
	doc, err := os.ReadFile(docsPath)
	if err != nil {
		t.Fatal(err)
	}
	want, err := ReplaceGenerated(string(doc))
	if err != nil {
		t.Fatal(err)
	}
	if update {
		if err = os.WriteFile(docsPath, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
	} else if want != string(doc) {
		t.Error("docs/config.md's reference is out of date: run UPDATE_DOCS=1 go test ./internal/tui/config/")
	}
}

func TestSchemaHasEveryOption(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			Properties map[string]any `json:"properties"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(Schema(), &schema); err != nil {
		t.Fatal(err)
	}
	for _, o := range Options() {
		if o.Section == "" {
			if _, ok := schema.Properties[o.Key]; !ok {
				t.Errorf("schema lacks %s", o.Key)
			}
		} else if _, ok := schema.Properties[o.Section].Properties[o.Key]; !ok {
			t.Errorf("schema lacks %s.%s", o.Section, o.Key)
		}
	}
	for _, s := range KeybindingSections {
		if _, ok := schema.Properties["keybindings"].Properties[s.Key]; !ok {
			t.Errorf("schema lacks keybindings.%s", s.Key)
		}
	}
}

// Every documented preference is real, has a description, and the unused ones stay out.
func TestPreferenceOptions(t *testing.T) {
	keys := map[string]bool{}
	for _, o := range PreferenceOptions() {
		keys[o.Key] = true
		if o.Doc == "" || o.Default == "" {
			t.Errorf("%s needs a description and default", o.Key)
		}
		if o.Type == "string" && o.Enum != nil && !strings.Contains(strings.Join(o.Enum, ","), o.Default) {
			t.Errorf("%s: default %s isn't one of %v", o.Key, o.Default, o.Enum)
		}
	}
	for _, unused := range []string{"hide_user_list", "hide_room_list", "disable_typing_notifs", "disable_markdown", "disable_html", "notify_sound"} {
		if keys[unused] {
			t.Errorf("%s does nothing and shouldn't be offered", unused)
		}
	}
	for _, used := range []string{"sidebar_on_start", "file_picker", "timestamp_style", "image_protocol", "backspace2_removes_word"} {
		if !keys[used] {
			t.Errorf("%s missing", used)
		}
	}
}

// The template is all comments: it loads with no problems and changes nothing.
func TestTemplateIsClean(t *testing.T) {
	uf, problems := ParseUserFile([]byte(Template()), t.TempDir())
	if problems != nil || len(uf.Set) != 0 {
		t.Errorf("template: %q %v", problems, uf.Set)
	}
	// Every setting at its documented default is valid.
	var b strings.Builder
	for _, o := range TopOptions() {
		b.WriteString(o.Key + ": " + o.Default + "\n")
	}
	b.WriteString("preferences:\n")
	for _, o := range PreferenceOptions() {
		b.WriteString("  " + o.Key + ": " + o.Default + "\n")
	}
	b.WriteString("colors:\n")
	for _, c := range ColorOptions {
		b.WriteString("  " + c.Key + ": \"" + c.Default + "\"\n")
	}
	uf, problems = ParseUserFile([]byte(b.String()), t.TempDir())
	if problems != nil || len(uf.Preferences) != len(PreferenceOptions()) || len(uf.Colors) != len(ColorOptions) {
		t.Errorf("every default is valid: %q\n%s", problems, b.String())
	}
	if !strings.Contains(Template(), "#   sidebar_on_start: false") || !strings.Contains(Template(), "# theme: tuimessenger") {
		t.Error("the template lists the settings at their defaults")
	}
}

func TestParseUserFileProblems(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "mine.json"), []byte(`{"theme":{}}`), 0o644)
	uf, problems := ParseUserFile([]byte(`theme: mine
thme_mode: dark
theme_mode: dim
preferences:
  file_picker: ranger
  sidebar_on_start: yes please
colors:
  acent: "#fff"
  border: "#zzzzzz"
keybindings:
  main:
    'Ctrl+n': next_rom
  sidebar:
    'j': down
`), dir)
	if uf.Theme != "mine" || !uf.Set["theme"] {
		t.Error("your own theme name is accepted")
	}
	want := []string{
		`line 2: unknown setting "thme_mode" (did you mean "theme_mode"?)`,
		`line 3: theme_mode is dark or light, not "dim"`,
		`line 5: preferences.file_picker is one of finder, yazi, not "ranger"`,
		`line 6: preferences.sidebar_on_start is true or false, not "yes please"`,
		`line 8: unknown colour "acent" (did you mean "accent"?)`,
		`line 9: colors.border:`,
		`line 12: unknown main action "next_rom" (did you mean "next_room"?)`,
		`line 13: unknown keybindings table "sidebar"`,
	}
	joined := strings.Join(problems, "\n")
	for _, w := range want {
		if !strings.Contains(joined, w) {
			t.Errorf("missing %q in:\n%s", w, joined)
		}
	}
	if len(problems) != len(want) {
		t.Errorf("%d problems, want %d:\n%s", len(problems), len(want), joined)
	}
	if _, problems = ParseUserFile([]byte("- just\n- a list\n"), dir); len(problems) != 1 {
		t.Errorf("a file that isn't settings: %q", problems)
	}
	if _, problems = ParseUserFile([]byte("theme: [\n"), dir); len(problems) != 1 || !strings.Contains(problems[0], "can't read it") {
		t.Errorf("broken YAML: %q", problems)
	}
}

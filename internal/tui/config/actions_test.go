package config

import (
	"slices"
	"testing"

	"codeberg.org/tslocum/cbind"
	"gopkg.in/yaml.v3"
)

// Every default key parses and binds a known action; every action but quit has a default key.
func TestDefaultKeybindings(t *testing.T) {
	var raw RawKeybindings
	if err := yaml.Unmarshal([]byte(DefaultKeybindings), &raw); err != nil {
		t.Fatal(err)
	}
	actions := KeybindingActions()
	bound := map[string]bool{}
	for section, table := range map[string]map[string]string{"main": raw.Main, "room": raw.Room, "visual": raw.Visual, "modal": raw.Modal} {
		for key, action := range table {
			if _, _, _, err := cbind.Decode(key); err != nil {
				t.Errorf("%s: %q doesn't parse: %v", section, key, err)
			}
			if !slices.Contains(actions[section], action) {
				t.Errorf("%s: %q binds %q, which isn't an action", section, key, action)
			}
			bound[section+"."+action] = true
		}
	}
	for section, list := range Actions {
		for _, a := range list {
			if a.Doc == "" {
				t.Errorf("%s.%s needs a description", section, a.Name)
			}
			if !bound[section+"."+a.Name] && a.Name != "quit" {
				t.Errorf("%s.%s has no default key", section, a.Name)
			}
		}
	}
}

func TestKeyLabel(t *testing.T) {
	for in, want := range map[string]string{"Ctrl+^": "Ctrl+m", "Ctrl+_": "Ctrl+;", "Ctrl+E": "Ctrl+e", "Alt+K": "Alt+k", "F1": "F1", "?": "?", "Ctrl+Enter": "Ctrl+Enter"} {
		if got := KeyLabel(in); got != want {
			t.Errorf("%s: got %s, want %s", in, got, want)
		}
	}
}

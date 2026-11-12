package tui

import (
	"slices"
	"strings"

	"codeberg.org/tslocum/cbind"

	"github.com/whyyor/tuimessenger/internal/tui/config"
)

// helpGroups are the keybindings tables, as help names them.
var helpGroups = []struct{ table, title string }{
	{"main", "Everywhere"},
	{"room", "In a chat"},
	{"visual", "Selecting messages (visual mode)"},
	{"modal", "Popups"},
}

// keysFor lists the keys bound to an action, Ctrl first, as you press them ("Ctrl+m" for Ctrl+^).
func keysFor(bindings map[config.Keybind]string, action string) []string {
	var keys []string
	for kb, act := range bindings {
		if act != action {
			continue
		}
		if enc, err := cbind.Encode(kb.Mod, kb.Key, kb.Ch); err == nil {
			keys = append(keys, config.KeyLabel(enc))
		}
	}
	slices.SortFunc(keys, func(a, b string) int {
		ca, cb := strings.HasPrefix(a, "Ctrl"), strings.HasPrefix(b, "Ctrl")
		if ca != cb {
			if ca {
				return -1
			}
			return 1
		}
		if len(a) != len(b) {
			return len(a) - len(b)
		}
		return strings.Compare(a, b)
	})
	return keys
}

// helpItems are every key (as you've configured them) and command, grouped the way help shows them.
func helpItems(cfg *config.Config) []*paletteItem {
	tables := map[string]map[config.Keybind]string{
		"main": cfg.Keybindings.Main, "room": cfg.Keybindings.Room,
		"visual": cfg.Keybindings.Visual, "modal": cfg.Keybindings.Modal,
	}
	var items []*paletteItem
	for _, g := range helpGroups {
		for _, a := range config.Actions[g.table] {
			keys := strings.Join(keysFor(tables[g.table], a.Name), "  ")
			if keys == "" {
				keys = "not set"
			}
			items = append(items, &paletteItem{section: sectionKeys, group: g.title, title: a.Doc, detail: keys})
		}
		if g.table == "modal" {
			items = append(items, fixedKeyItems("Popups")...)
		}
	}
	items = append(items, fixedKeyItems("Typing")...)
	items = append(items, fixedKeyItems("Home screen")...)
	for _, cmd := range LocalCommands {
		name := "/" + cmd.Command
		for _, alias := range cmd.Aliases {
			name += "  /" + alias
		}
		desc := ""
		if cmd.Description != nil && len(cmd.Description.Text) > 0 {
			desc = cmd.Description.Text[0].Body
		}
		items = append(items, &paletteItem{section: sectionCommands, group: "Commands", title: desc, detail: name})
	}
	return items
}

func fixedKeyItems(group string) []*paletteItem {
	var items []*paletteItem
	for _, f := range config.FixedKeys {
		if f.Group == group {
			items = append(items, &paletteItem{section: sectionKeys, group: group, title: f.Doc, detail: f.Keys})
		}
	}
	return items
}

// ShowHelp opens help: every key and command, searchable (F1, or Ctrl+p → Keys and commands).
func (view *MainView) ShowHelp() {
	p := &CommandPalette{parent: view, grouped: true, title: " \uf059 Keys and commands ", hint: "type to search  esc close",
		items: helpItems(view.config)}
	p.refilter()
	view.ShowModal(p)
}

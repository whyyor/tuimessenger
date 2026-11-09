// Package integration installs the tuimessenger agent skill where AI coding agents look for skills.
package integration

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	skillName = "tuimessenger"
	marker    = "<!-- Installed by `tuimessenger integration install`: reinstall to update, uninstall to remove. -->"
)

// Targets are the agents we know, and their user-wide skill folders in order of preference.
var Targets = map[string][]string{
	"claude":   {".claude/skills"},
	"codex":    {".agents/skills"},
	"opencode": {".config/opencode/skills", ".agents/skills"}, // opencode also reads ~/.agents/skills
}

// Names lists the targets alphabetically.
func Names() []string {
	var names []string
	for name := range Targets {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// ErrNotOurs means a skill with our name is there but wasn't installed by us.
var ErrNotOurs = errors.New("a tuimessenger skill that wasn't installed by tuimessenger is already there (use -force to replace it)")

// Install writes the skill for target under home and returns where it went.
func Install(home, target, skill string, force bool) (string, error) {
	dirs, ok := Targets[target]
	if !ok {
		return "", fmt.Errorf("unknown agent %q (one of %s)", target, strings.Join(Names(), ", "))
	}
	var lastErr error
	for _, dir := range dirs {
		path := filepath.Join(home, dir, skillName, "SKILL.md")
		if existing, err := os.ReadFile(path); err == nil && !strings.Contains(string(existing), marker) && !force {
			return path, ErrNotOurs
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			lastErr = err // e.g. a read-only, Nix-managed config folder: try the next one
			continue
		}
		if err := os.WriteFile(path, []byte(withMarker(skill)), 0o644); err != nil {
			lastErr = err
			continue
		}
		return path, nil
	}
	return "", lastErr
}

// Uninstall removes the skill from every folder of target where we installed it, and returns those paths.
func Uninstall(home, target string) ([]string, error) {
	dirs, ok := Targets[target]
	if !ok {
		return nil, fmt.Errorf("unknown agent %q (one of %s)", target, strings.Join(Names(), ", "))
	}
	var removed []string
	for _, dir := range dirs {
		skillDir := filepath.Join(home, dir, skillName)
		if data, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md")); err != nil || !strings.Contains(string(data), marker) {
			continue
		}
		if err := os.RemoveAll(skillDir); err != nil {
			return removed, err
		}
		removed = append(removed, skillDir)
	}
	return removed, nil
}

// Found is a tuimessenger skill on disk.
type Found struct {
	Path     string `json:"path"`
	Ours     bool   `json:"installed_by_tuimessenger"`
	UpToDate bool   `json:"up_to_date"`
}

// Status lists the tuimessenger skills an agent would see.
func Status(home, target, skill string) []Found {
	var found []Found
	for _, dir := range Targets[target] {
		path := filepath.Join(home, dir, skillName, "SKILL.md")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		ours := strings.Contains(string(data), marker)
		found = append(found, Found{Path: path, Ours: ours, UpToDate: ours && string(data) == withMarker(skill)})
	}
	return found
}

// withMarker puts the marker right after the frontmatter, where agents ignore it.
func withMarker(skill string) string {
	if rest, ok := strings.CutPrefix(skill, "---\n"); ok {
		if front, body, ok := strings.Cut(rest, "\n---\n"); ok {
			return "---\n" + front + "\n---\n" + marker + "\n" + body
		}
	}
	return marker + "\n" + skill
}

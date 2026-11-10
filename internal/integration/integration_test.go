package integration

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const skill = "---\nname: tuimessenger\ndescription: test\n---\n\n# tuimessenger\n"

func TestInstallUpdateUninstall(t *testing.T) {
	home := t.TempDir()
	path, err := Install(home, "claude", skill, false)
	if err != nil || path != filepath.Join(home, ".claude/skills/tuimessenger/SKILL.md") {
		t.Fatalf("install: %s, %v", path, err)
	}
	data, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(data), "---\nname: tuimessenger\ndescription: test\n---\n"+marker+"\n") {
		t.Fatalf("frontmatter must stay first:\n%s", data)
	}
	if st := Status(home, "claude", skill); len(st) != 1 || !st[0].Ours || !st[0].UpToDate {
		t.Fatalf("status = %+v", st)
	}
	if st := Status(home, "claude", skill+"new\n"); st[0].UpToDate {
		t.Fatal("an older install should not be up to date")
	}
	if _, err = Install(home, "claude", skill+"new\n", false); err != nil {
		t.Fatalf("reinstall over our own: %v", err)
	}
	removed, err := Uninstall(home, "claude")
	if err != nil || len(removed) != 1 {
		t.Fatalf("uninstall: %v, %v", removed, err)
	}
	if _, err = os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatal("skill folder still there")
	}
}

// A skill someone wrote themselves is never overwritten or removed without -force.
func TestNotOurs(t *testing.T) {
	home := t.TempDir()
	own := filepath.Join(home, ".agents/skills/tuimessenger/SKILL.md")
	_ = os.MkdirAll(filepath.Dir(own), 0o755)
	_ = os.WriteFile(own, []byte("my own"), 0o644)
	if _, err := Install(home, "codex", skill, false); !errors.Is(err, ErrNotOurs) {
		t.Fatalf("err = %v", err)
	}
	if removed, _ := Uninstall(home, "codex"); len(removed) != 0 {
		t.Fatal("removed a skill that isn't ours")
	}
	if data, _ := os.ReadFile(own); string(data) != "my own" {
		t.Fatal("own skill changed")
	}
	if _, err := Install(home, "codex", skill, true); err != nil {
		t.Fatalf("force: %v", err)
	}
}

// opencode's own folder can be read-only (Nix, home-manager): then ~/.agents/skills, which opencode also reads.
func TestOpencodeFallback(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write anywhere")
	}
	home := t.TempDir()
	ro := filepath.Join(home, ".config/opencode")
	_ = os.MkdirAll(ro, 0o755)
	_ = os.Chmod(ro, 0o555)
	t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })
	path, err := Install(home, "opencode", skill, false)
	if err != nil || path != filepath.Join(home, ".agents/skills/tuimessenger/SKILL.md") {
		t.Fatalf("install: %s, %v", path, err)
	}
	if _, err = Install(home, "cursor", skill, false); err == nil {
		t.Fatal("unknown agent accepted")
	}
}

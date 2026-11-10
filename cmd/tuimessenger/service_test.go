package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A Homebrew install is recorded by its opt link, which survives brew upgrade; anything else is left alone.
func TestBrewOptPath(t *testing.T) {
	prefix := t.TempDir()
	exe := filepath.Join(prefix, "Cellar", "tuimessenger", "0.1.0", "bin", "tuimessenger")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := brewOptPath(exe); got != exe {
		t.Errorf("without an opt link the Cellar path stays: %s", got)
	}

	if err := os.MkdirAll(filepath.Join(prefix, "opt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(prefix, "Cellar", "tuimessenger", "0.1.0"), filepath.Join(prefix, "opt", "tuimessenger")); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(prefix, "opt", "tuimessenger", "bin", "tuimessenger")
	if got := brewOptPath(exe); got != want {
		t.Errorf("got %s, want %s", got, want)
	}

	for _, other := range []string{"/Users/a/.local/opt/tuimessenger/tuimessenger", "/usr/local/bin/tuimessenger", "/x/Cellar/tuimessenger/"} {
		if got := brewOptPath(other); got != other {
			t.Errorf("%s should stay, got %s", other, got)
		}
	}
}

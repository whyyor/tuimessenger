package tui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.mau.fi/mauview"

	"go.mau.fi/gomuks/tui/config"
)

func TestReadChooserFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chooser")
	_ = os.WriteFile(path, []byte("/Users/me/Downloads/a b.pdf\r\n/Users/me/x.png\n"), 0o600)
	got, err := readChooserFile(path)
	if err != nil || !slices.Equal(got, []string{"/Users/me/Downloads/a b.pdf", "/Users/me/x.png"}) {
		t.Errorf("got %q %v", got, err)
	}
	_ = os.WriteFile(path, nil, 0o600)
	if got, _ := readChooserFile(path); len(got) != 0 {
		t.Errorf("q in yazi writes nothing: no files, got %q", got)
	}
}

func TestYaziMissingFallsBackToFinder(t *testing.T) {
	ts, _, _ := fakeToasts(t)
	t.Setenv("PATH", t.TempDir())
	old := finderPicker
	t.Cleanup(func() { finderPicker = old })
	called := false
	finderPicker = func() ([]string, error) { called = true; return nil, nil }
	cfg := &config.Config{}
	cfg.Preferences.FilePicker = "yazi"
	view := &RoomView{config: cfg, input: mauview.NewInputArea()}
	view.AttachPicked()
	if !called || !strings.Contains(current(ts), "yazi isn't installed") {
		t.Errorf("without yazi: Finder plus a warning (finder=%v, toast=%q)", called, current(ts))
	}
}

func TestPaletteTogglesFilePicker(t *testing.T) {
	_, _, _ = fakeToasts(t)
	main := aliasMain(t)
	main.config.Dir = t.TempDir()
	run := func(prefix string) {
		t.Helper()
		for _, item := range (&CommandPalette{parent: main}).commandItems() {
			if strings.HasPrefix(item.title, prefix) {
				item.run()
				return
			}
		}
		t.Fatalf("no %q in the palette", prefix)
	}
	run("File picker: Finder → yazi")
	if main.config.Preferences.FilePicker != "yazi" {
		t.Fatalf("picker = %q", main.config.Preferences.FilePicker)
	}
	run("File picker: yazi → Finder")
	if main.config.Preferences.FilePicker != "" {
		t.Errorf("back to Finder, got %q", main.config.Preferences.FilePicker)
	}
}

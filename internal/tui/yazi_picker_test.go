package tui

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"go.mau.fi/mauview"

	"github.com/whyyor/tuimessenger/internal/tui/config"
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
	dialog := systemPickerName()
	run("File picker: " + dialog + " → yazi")
	if main.config.Preferences.FilePicker != "yazi" {
		t.Fatalf("picker = %q", main.config.Preferences.FilePicker)
	}
	run("File picker: yazi → " + dialog)
	if main.config.Preferences.FilePicker != "" {
		t.Errorf("back to Finder, got %q", main.config.Preferences.FilePicker)
	}
}

// Suspend only queues work for the UI loop; the picker must wait for yazi.
func TestYaziPickerWaitsForYazi(t *testing.T) {
	dir := t.TempDir()
	yazi := filepath.Join(dir, "yazi")
	script := "#!/bin/sh\nfor a; do case $a in --chooser-file=*) f=${a#--chooser-file=};; esac; done\n" +
		"sleep 0.2\necho /Users/me/Downloads/cat.png > \"$f\"\n"
	if err := os.WriteFile(yazi, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	old := suspendTerminal
	t.Cleanup(func() { suspendTerminal = old })
	suspendTerminal = func(_ *RoomView, fn func()) {
		go func() { time.Sleep(50 * time.Millisecond); fn() }() // like the UI loop, later
	}
	got, err := (&RoomView{}).pickWithYazi(yazi)
	if err != nil || !slices.Equal(got, []string{"/Users/me/Downloads/cat.png"}) {
		t.Errorf("the pick should come back after yazi exits, got %q %v", got, err)
	}
}

// On Linux without zenity there's no system dialog: the picker uses yazi when it's installed.
func TestNoDialogUsesYazi(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("macOS always has the Finder dialog")
	}
	_, _, _ = fakeToasts(t)
	dir := t.TempDir()
	marker := filepath.Join(dir, "yazi-ran")
	script := "#!/bin/sh\n: > " + marker + "\nfor a; do case $a in --chooser-file=*) f=${a#--chooser-file=};; esac; done\n" +
		": > \"$f\"\n" // picks nothing, so nothing gets attached
	if err := os.WriteFile(filepath.Join(dir, "yazi"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir) // yazi, no zenity
	oldSuspend, oldFinder := suspendTerminal, finderPicker
	t.Cleanup(func() { suspendTerminal, finderPicker = oldSuspend, oldFinder })
	suspendTerminal = func(_ *RoomView, fn func()) { go fn() }
	finderPicker = func() ([]string, error) { t.Error("the missing dialog was tried"); return nil, nil }
	view := &RoomView{config: &config.Config{}, input: mauview.NewInputArea()}
	view.AttachPicked()
	if _, err := os.Stat(marker); err != nil {
		t.Error("yazi should have been used as the picker")
	}
}

// Panels are switched on and off from Ctrl+p, like the layout.
func TestPaletteTogglesPanels(t *testing.T) {
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
	run("Panels (")
	if !main.config.Preferences.EnablePanels {
		t.Fatal("panels should be on")
	}
	run("Panels (")
	if main.config.Preferences.EnablePanels {
		t.Error("panels should be off again")
	}
}

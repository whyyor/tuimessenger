package notification

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/whyyor/tuimessenger/assets"
)

// Unpacking only: never codesign or lsregister throwaway copies (a registered copy that's then deleted breaks the
// real app's notifications).
func TestUnpackNotifier(t *testing.T) {
	app := filepath.Join(t.TempDir(), "tuimessenger.app")
	if err := unpackNotifier(app); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(app, notifierExe))
	if err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("the notifier program must be there and executable: %v %v", err, info)
	}
	if _, err = os.Stat(filepath.Join(app, "Contents", "Info.plist")); err != nil {
		t.Error("Info.plist missing")
	}
	if _, err = os.Stat(filepath.Join(app, "Contents", "Resources", "Terminal.icns")); !os.IsNotExist(err) {
		t.Error("Apple's Terminal icon must not be shipped (not MIT)")
	}
}

func TestNotifiersUpToDate(t *testing.T) {
	old := notifierDir
	notifierDir = t.TempDir()
	t.Cleanup(func() { notifierDir = old })
	app := filepath.Join(notifierDir, appName+".app")
	if NotifiersUpToDate() {
		t.Fatal("nothing installed yet")
	}
	if err := unpackNotifier(app); err != nil {
		t.Fatal(err)
	}
	icon := filepath.Join(app, "Contents", "Resources", "icon.icns")
	if err := os.WriteFile(icon, assets.IconICNS, 0o644); err != nil {
		t.Fatal(err)
	}
	if !NotifiersUpToDate() {
		t.Error("bundled program + current icon: up to date")
	}
	// An older notifier program (e.g. the one copied from brew) gets replaced.
	if err := os.WriteFile(filepath.Join(app, notifierExe), []byte("old build"), 0o755); err != nil {
		t.Fatal(err)
	}
	if NotifiersUpToDate() {
		t.Error("a different notifier program must count as outdated")
	}
}

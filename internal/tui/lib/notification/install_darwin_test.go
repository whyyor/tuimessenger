package notification

import (
	"bytes"
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
	if NotifiersUpToDate() {
		t.Fatal("nothing installed yet")
	}
	// Lay the apps out like InstallNotifiers, without codesign/lsregister.
	for _, a := range notifierApps() {
		app := filepath.Join(notifierDir, a.name+".app")
		if err := unpackNotifier(app); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(app, "Contents", "Resources", "icon.icns"), a.icon, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if !NotifiersUpToDate() {
		t.Error("bundled program + current icons: up to date")
	}
	// Only tuimessenger.app, as before per-network apps were bundled: the network apps get built.
	if err := os.RemoveAll(filepath.Join(notifierDir, "Slack.app")); err != nil {
		t.Fatal(err)
	}
	if NotifiersUpToDate() {
		t.Error("a missing network app must count as outdated")
	}
	// An older notifier program (e.g. the one copied from brew) gets replaced.
	app := filepath.Join(notifierDir, appName+".app")
	if err := os.WriteFile(filepath.Join(app, notifierExe), []byte("old build"), 0o755); err != nil {
		t.Fatal(err)
	}
	if NotifiersUpToDate() {
		t.Error("a different notifier program must count as outdated")
	}
}

// Every network notifications are routed to (sourceBundles) has an app with its own icon, else it'd look like
// tuimessenger's.
func TestEveryNetworkHasANotifierApp(t *testing.T) {
	icons := map[string][]byte{}
	for _, a := range notifierApps() {
		icons[a.name] = a.icon
	}
	for source, name := range sourceBundles {
		icon, ok := icons[name]
		if !ok || len(icon) == 0 {
			t.Errorf("%s notifications go to %s.app, which isn't bundled", source, name)
		} else if bytes.Equal(icon, assets.IconICNS) {
			t.Errorf("%s.app has tuimessenger's icon", name)
		}
	}
}

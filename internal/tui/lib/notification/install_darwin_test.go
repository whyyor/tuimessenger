package notification

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		if err := os.WriteFile(filepath.Join(app, stampFile), []byte(stamp(a)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if !NotifiersUpToDate() {
		t.Error("stamped with the current notifier, icons and names: up to date")
	}
	// Signing rewrites the executable; that mustn't count as outdated (it rebuilt every launch).
	if err := os.WriteFile(filepath.Join(notifierDir, "Slack.app", notifierExe), []byte("re-signed"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !NotifiersUpToDate() {
		t.Error("a re-signed executable is still up to date")
	}
	// An app from before the stamp (v0.1.1), or built with another icon or name, is rebuilt once.
	if err := os.WriteFile(filepath.Join(notifierDir, "WhatsApp.app", stampFile), []byte("notifier old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if NotifiersUpToDate() {
		t.Error("a different stamp must count as outdated")
	}
	if err := os.Remove(filepath.Join(notifierDir, "WhatsApp.app", stampFile)); err != nil {
		t.Fatal(err)
	}
	if NotifiersUpToDate() {
		t.Error("no stamp (an older install) must count as outdated")
	}
}

func TestDisplayNames(t *testing.T) {
	if displayName("WhatsApp") != "WhatsApp" || displayName(appName) != "tuimessenger" {
		t.Errorf("names: %q %q", displayName("WhatsApp"), displayName(appName))
	}
	if stamp(notifierApps()[1]) == stamp(notifierApps()[2]) {
		t.Error("each app's stamp differs (icon and name)")
	}
}

// fakeNotifier is a notifier app whose program is a shell script.
func fakeNotifier(t *testing.T, app, script string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), app+".app", "Contents", "MacOS", "terminal-notifier")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

// terminal-notifier exits with 3 when the app isn't allowed: that's told apart from other failures.
func TestNotAuthorized(t *testing.T) {
	err := runNotifier(fakeNotifier(t, "WhatsApp", "exit 3"), true, nil)
	var na *NotAuthorizedError
	if !errors.As(err, &na) || na.App != "WhatsApp" {
		t.Errorf("exit 3 means not allowed: %v", err)
	}
	if err = runNotifier(fakeNotifier(t, "WhatsApp", "exit 1"), true, nil); err == nil || errors.As(err, &na) {
		t.Errorf("other failures stay plain errors: %v", err)
	}
	if err = runNotifier(fakeNotifier(t, "WhatsApp", "exit 0"), true, nil); err != nil {
		t.Errorf("success: %v", err)
	}
}

// While macOS's permission prompt waits, terminal-notifier doesn't exit: the send gives up instead of hanging.
func TestNotifierTimeout(t *testing.T) {
	old := sendTimeout
	sendTimeout = 200 * time.Millisecond
	t.Cleanup(func() { sendTimeout = old })
	start := time.Now()
	err := runNotifier(fakeNotifier(t, "Slack", "sleep 30"), true, nil)
	if err == nil || !strings.Contains(err.Error(), "didn't finish") || time.Since(start) > 5*time.Second {
		t.Errorf("a hung notifier is given up on: %v after %s", err, time.Since(start))
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

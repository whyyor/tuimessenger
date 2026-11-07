package notification

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/whyyor/tuimessenger/assets"
)

const (
	appName = "tuimessenger"
	// appID keeps the identifier the first hand-made notifier had, so macOS keeps its notification permission.
	appID      = "fr.julienxx.oss.terminal-notifier.tuimessenger"
	lsregister = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
)

// ErrNoTerminalNotifier means the notifier apps can't be built; notifications then come from Script Editor.
var ErrNoTerminalNotifier = errors.New("terminal-notifier isn't installed (brew install terminal-notifier)")

// NotifiersUpToDate reports whether tuimessenger.app exists with the current icon.
func NotifiersUpToDate() bool {
	icon, err := os.ReadFile(filepath.Join(notifierDir, appName+".app", "Contents", "Resources", "icon.icns"))
	return err == nil && bytes.Equal(icon, assets.IconICNS)
}

// brewNotifierApp finds the terminal-notifier.app that the terminal-notifier command belongs to.
func brewNotifierApp() (string, error) {
	bin, err := exec.LookPath("terminal-notifier")
	if err != nil {
		return "", ErrNoTerminalNotifier
	}
	if bin, err = filepath.EvalSymlinks(bin); err != nil {
		return "", err
	}
	app := filepath.Join(filepath.Dir(bin), "..", "terminal-notifier.app")
	if _, err = os.Stat(filepath.Join(app, "Contents", "Info.plist")); err != nil {
		return "", fmt.Errorf("no terminal-notifier.app next to %s", bin)
	}
	return filepath.Clean(app), nil
}

func runCmd(name string, args ...string) error {
	if out, err := exec.Command(name, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w: %s", filepath.Base(name), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// InstallNotifiers builds tuimessenger.app (terminal-notifier with tuimessenger's name and icon) and registers every
// notifier app with Launch Services; macOS refuses notifications from apps it doesn't know at their current path.
func InstallNotifiers() error {
	src, err := brewNotifierApp()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(notifierDir, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(notifierDir, appName+".app")
	tmp := dst + ".tmp"
	_ = os.RemoveAll(tmp)
	defer os.RemoveAll(tmp)
	if err = runCmd("ditto", src, tmp); err != nil {
		return err
	}
	res := filepath.Join(tmp, "Contents", "Resources")
	_ = os.Remove(filepath.Join(res, "Terminal.icns"))
	if err = os.WriteFile(filepath.Join(res, "icon.icns"), assets.IconICNS, 0o644); err != nil {
		return err
	}
	plist := filepath.Join(tmp, "Contents", "Info.plist")
	for key, value := range map[string]string{"CFBundleIconFile": "icon", "CFBundleIdentifier": appID, "CFBundleName": appName} {
		if err = runCmd("plutil", "-replace", key, "-string", value, plist); err != nil {
			return err
		}
	}
	// Editing the bundle broke its signature; an ad-hoc one is enough for a local app.
	if err = runCmd("codesign", "--force", "--sign", "-", tmp); err != nil {
		return err
	}
	if err = os.RemoveAll(dst); err != nil {
		return err
	}
	if err = os.Rename(tmp, dst); err != nil {
		return err
	}
	apps, _ := filepath.Glob(filepath.Join(notifierDir, "*.app"))
	for _, app := range apps {
		if err = runCmd(lsregister, "-f", "-R", app); err != nil {
			return err
		}
	}
	return nil
}

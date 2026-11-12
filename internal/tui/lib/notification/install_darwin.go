package notification

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/whyyor/tuimessenger/assets"
)

const (
	appName = "tuimessenger"
	// appIDPrefix + the app's name is the identifier the first hand-made notifiers had, so macOS keeps their
	// notification permissions.
	appIDPrefix = "fr.julienxx.oss.terminal-notifier."
	lsregister  = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
)

// notifierExe is the program inside the bundled app.
const notifierExe = "Contents/MacOS/terminal-notifier"

// notifierApp is one notification app: macOS shows a notification under its sender app's name and icon, so each
// network gets its own (and its own entry in System Settings → Notifications).
type notifierApp struct {
	name string
	icon []byte
}

// notifierApps are tuimessenger.app (for everything else) and one app per network in sourceBundles.
func notifierApps() []notifierApp {
	apps := []notifierApp{{appName, assets.IconICNS}}
	for _, name := range []string{"WhatsApp", "Telegram", "Slack", "Instagram"} {
		apps = append(apps, notifierApp{name, assets.NetworkIcon(name)})
	}
	return apps
}

// stampFile records what an installed notifier app was built from. Comparing the app's files instead can't work:
// signing rewrites the executable, so it never matches the bundled one.
const stampFile = "Contents/Resources/tuimessenger.stamp"

// displayName is what macOS shows for a notifier app: the network's name ("WhatsApp"), or "tuimessenger".
func displayName(app string) string {
	return app
}

// stamp identifies a notifier app's build: the bundled notifier, its icon and its name.
func stamp(a notifierApp) string {
	return fmt.Sprintf("notifier %x\nicon %x\nname %s\n", sha256.Sum256(assets.NotifierZip), sha256.Sum256(a.icon), displayName(a.name))
}

// NotifiersUpToDate reports whether every notifier app exists, built from the current notifier, icon and name.
func NotifiersUpToDate() bool {
	for _, a := range notifierApps() {
		got, err := os.ReadFile(filepath.Join(notifierDir, a.name+".app", stampFile))
		if err != nil || string(got) != stamp(a) {
			return false
		}
	}
	return true
}

// unpackNotifier writes the bundled terminal-notifier.app to dst (the .app folder itself).
func unpackNotifier(dst string) error {
	zr, err := zip.NewReader(bytes.NewReader(assets.NotifierZip), int64(len(assets.NotifierZip)))
	if err != nil {
		return err
	}
	const prefix = "terminal-notifier.app/"
	for _, f := range zr.File {
		rel, ok := strings.CutPrefix(f.Name, prefix)
		if !ok || rel == "" {
			continue
		}
		path := filepath.Join(dst, filepath.FromSlash(rel))
		if !strings.HasPrefix(path, filepath.Clean(dst)+string(filepath.Separator)) {
			return fmt.Errorf("bad path in the bundled notifier: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err = os.MkdirAll(path, 0o755); err != nil {
				return err
			}
			continue
		}
		mode := os.FileMode(0o644)
		if rel == notifierExe {
			mode = 0o755
		}
		if err = writeZipFile(f, path, mode); err != nil {
			return err
		}
	}
	return nil
}

func writeZipFile(f *zip.File, path string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, r); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func runCmd(name string, args ...string) error {
	if out, err := exec.Command(name, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w: %s", filepath.Base(name), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// InstallNotifiers unpacks the bundled terminal-notifier once per notifier app (tuimessenger.app and one per network,
// each with its name and icon) and registers every notifier app with Launch Services; macOS refuses notifications
// from apps it doesn't know at their current path. Files tuimessenger writes carry no quarantine flag, so Gatekeeper
// doesn't ask about them.
func InstallNotifiers() error {
	if err := os.MkdirAll(notifierDir, 0o755); err != nil {
		return err
	}
	for _, a := range notifierApps() {
		if err := installNotifierApp(a); err != nil {
			return fmt.Errorf("%s.app: %w", a.name, err)
		}
	}
	apps, _ := filepath.Glob(filepath.Join(notifierDir, "*.app"))
	for _, app := range apps {
		if err := runCmd(lsregister, "-f", "-R", app); err != nil {
			return err
		}
	}
	return nil
}

// installNotifierApp builds one notifier app next to its old copy, then swaps it in.
func installNotifierApp(a notifierApp) error {
	if len(a.icon) == 0 {
		return fmt.Errorf("no icon bundled")
	}
	dst := filepath.Join(notifierDir, a.name+".app")
	tmp := dst + ".tmp"
	_ = os.RemoveAll(tmp)
	defer os.RemoveAll(tmp)
	err := unpackNotifier(tmp)
	if err != nil {
		return err
	}
	res := filepath.Join(tmp, "Contents", "Resources")
	if err = os.WriteFile(filepath.Join(res, "icon.icns"), a.icon, 0o644); err != nil {
		return err
	}
	plist := filepath.Join(tmp, "Contents", "Info.plist")
	name := displayName(a.name)
	for key, value := range map[string]string{"CFBundleIconFile": "icon", "CFBundleIdentifier": appIDPrefix + a.name, "CFBundleName": name, "CFBundleDisplayName": name} {
		if err = runCmd("plutil", "-replace", key, "-string", value, plist); err != nil {
			return err
		}
	}
	// Editing the bundle broke its signature; an ad-hoc one is enough for a local app. The stamp is written first:
	// Resources is part of what the signature covers.
	if err = os.WriteFile(filepath.Join(tmp, stampFile), []byte(stamp(a)), 0o644); err != nil {
		return err
	}
	if err = runCmd("codesign", "--force", "--sign", "-", tmp); err != nil {
		return err
	}
	if err = os.RemoveAll(dst); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

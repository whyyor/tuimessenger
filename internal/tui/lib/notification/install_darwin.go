package notification

import (
	"archive/zip"
	"bytes"
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
	// appID keeps the identifier the first hand-made notifier had, so macOS keeps its notification permission.
	appID      = "fr.julienxx.oss.terminal-notifier.tuimessenger"
	lsregister = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
)

// notifierExe is the program inside the bundled app.
const notifierExe = "Contents/MacOS/terminal-notifier"

// NotifiersUpToDate reports whether tuimessenger.app exists with the current icon and the bundled notifier program.
func NotifiersUpToDate() bool {
	app := filepath.Join(notifierDir, appName+".app")
	icon, err := os.ReadFile(filepath.Join(app, "Contents", "Resources", "icon.icns"))
	if err != nil || !bytes.Equal(icon, assets.IconICNS) {
		return false
	}
	exe, err := os.ReadFile(filepath.Join(app, notifierExe))
	if err != nil {
		return false
	}
	bundled, err := bundledFile(notifierExe)
	return err == nil && bytes.Equal(exe, bundled)
}

// bundledFile reads one file of the bundled terminal-notifier.app.
func bundledFile(name string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(assets.NotifierZip), int64(len(assets.NotifierZip)))
	if err != nil {
		return nil, err
	}
	f, err := zr.Open("terminal-notifier.app/" + name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
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

// InstallNotifiers unpacks the bundled terminal-notifier as tuimessenger.app (with tuimessenger's name and icon) and
// registers every notifier app with Launch Services; macOS refuses notifications from apps it doesn't know at their
// current path. Files tuimessenger writes carry no quarantine flag, so Gatekeeper doesn't ask about it.
func InstallNotifiers() error {
	if err := os.MkdirAll(notifierDir, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(notifierDir, appName+".app")
	tmp := dst + ".tmp"
	_ = os.RemoveAll(tmp)
	defer os.RemoveAll(tmp)
	err := unpackNotifier(tmp)
	if err != nil {
		return err
	}
	res := filepath.Join(tmp, "Contents", "Resources")
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

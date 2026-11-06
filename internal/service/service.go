// Package service installs the backend as a background service: a launchd agent on macOS, a systemd user unit on Linux.
// It starts with your login and restarts if it stops.
package service

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Label names the service; on macOS it's also the plist's name.
const Label = "com.whyyor.tuimessenger"

// unitName is the systemd user unit.
const unitName = "tuimessenger.service"

// Spec is what the service runs.
type Spec struct {
	Executable string // absolute path of the tuimessenger binary
	DataDir    string
	LogDir     string // macOS only; systemd keeps logs in the journal
}

// run executes a command and returns its combined output; tests replace it.
var run = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

func runErr(name string, args ...string) error {
	if out, err := run(name, args...); err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func xmlEscape(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

// launchdPlist is the launchd agent: run at login, restart after a crash (not after a clean stop), 10 s apart.
func launchdPlist(spec Spec) string {
	logFile := xmlEscape(filepath.Join(spec.LogDir, "tuimessenger.log"))
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + Label + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + xmlEscape(spec.Executable) + `</string>
		<string>-data</string>
		<string>` + xmlEscape(spec.DataDir) + `</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	<key>ThrottleInterval</key>
	<integer>10</integer>
	<key>ProcessType</key>
	<string>Background</string>
	<key>StandardOutPath</key>
	<string>` + logFile + `</string>
	<key>StandardErrorPath</key>
	<string>` + logFile + `</string>
</dict>
</plist>
`
}

// systemdQuote quotes one ExecStart argument.
func systemdQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, `%`, `%%`, `$`, `$$`).Replace(s) + `"`
}

// systemdUnit is the user unit: start with the user's session, restart after a crash, 10 s apart.
func systemdUnit(spec Spec) string {
	return `[Unit]
Description=tuimessenger backend (chat bridges)
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=` + systemdQuote(spec.Executable) + ` -data ` + systemdQuote(spec.DataDir) + `
Restart=on-failure
RestartSec=10

[Install]
WantedBy=default.target
`
}

// writeFile replaces path atomically, so a crash can't leave half a file.
func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// InstalledFor reports whether a service is installed for this data folder (one without -data uses defaultDataDir).
func InstalledFor(dataDir, defaultDataDir string) bool {
	path, err := filePath()
	if err != nil {
		return false
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return runsDataDir(string(content), dataDir, defaultDataDir)
}

func runsDataDir(content, dataDir, defaultDataDir string) bool {
	dataDir, defaultDataDir = filepath.Clean(dataDir), filepath.Clean(defaultDataDir)
	if !strings.Contains(content, "-data") {
		return dataDir == defaultDataDir
	}
	return strings.Contains(content, xmlEscape(dataDir)+"<") || strings.Contains(content, systemdQuote(dataDir))
}

// Installed reports whether the service file exists.
func Installed() bool {
	path, err := filePath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

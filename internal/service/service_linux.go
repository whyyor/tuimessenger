package service

import (
	"os"
	"path/filepath"
	"strings"
)

func filePath() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "systemd", "user", unitName), nil
}

// Install writes the systemd user unit, enables it and (re)starts it.
func Install(spec Spec) error {
	path, err := filePath()
	if err != nil {
		return err
	}
	if err = writeFile(path, systemdUnit(spec)); err != nil {
		return err
	}
	for _, args := range [][]string{{"daemon-reload"}, {"enable", unitName}, {"restart", unitName}} {
		if err = runErr("systemctl", append([]string{"--user"}, args...)...); err != nil {
			return err
		}
	}
	return nil
}

// Start starts the installed unit now.
func Start() error {
	return runErr("systemctl", "--user", "start", unitName)
}

// Uninstall stops and disables the unit and removes its file.
func Uninstall() error {
	path, err := filePath()
	if err != nil {
		return err
	}
	_, _ = run("systemctl", "--user", "disable", "--now", unitName)
	if err = os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return runErr("systemctl", "--user", "daemon-reload")
}

// Status describes the service in one line.
func Status() string {
	if !Installed() {
		return "not installed"
	}
	out, _ := run("systemctl", "--user", "is-active", unitName)
	return "installed, " + strings.TrimSpace(string(out))
}

// Hint is extra advice after installing: user units stop at logout unless lingering is on (needed on a Pi).
func Hint() string {
	return "To keep it running while you're logged out (e.g. on a Pi): sudo loginctl enable-linger $USER"
}

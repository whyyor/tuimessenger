package service

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

func filePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", Label+".plist"), nil
}

func target() string { return fmt.Sprintf("gui/%d/%s", os.Getuid(), Label) }

func loaded() bool {
	_, err := run("launchctl", "print", target())
	return err == nil
}

// unload stops the agent if launchd has it (whoever installed it) and waits until it's gone.
func unload() {
	if !loaded() {
		return
	}
	_, _ = run("launchctl", "bootout", target())
	for deadline := time.Now().Add(15 * time.Second); loaded() && time.Now().Before(deadline); {
		time.Sleep(200 * time.Millisecond)
	}
}

// Install writes the launchd agent, replacing one with the same label, and starts it.
func Install(spec Spec) error {
	path, err := filePath()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(spec.LogDir, 0o700); err != nil {
		return err
	}
	unload()
	if err = writeFile(path, launchdPlist(spec)); err != nil {
		return err
	}
	return runErr("launchctl", "bootstrap", fmt.Sprintf("gui/%d", os.Getuid()), path)
}

// Start starts the installed agent now, loading it first if launchd doesn't have it.
func Start() error {
	if !loaded() {
		path, err := filePath()
		if err != nil {
			return err
		}
		return runErr("launchctl", "bootstrap", fmt.Sprintf("gui/%d", os.Getuid()), path)
	}
	return runErr("launchctl", "kickstart", target())
}

// Uninstall stops the agent and removes its file.
func Uninstall() error {
	path, err := filePath()
	if err != nil {
		return err
	}
	unload()
	if err = os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

var (
	launchdState = regexp.MustCompile(`(?m)^\s*state = (.+?)\s*$`)
	launchdPID   = regexp.MustCompile(`(?m)^\s*pid = (\d+)`)
)

// Status describes the service in one line.
func Status() string {
	path, _ := filePath()
	if !Installed() {
		return "not installed"
	}
	out, err := run("launchctl", "print", target())
	if err != nil {
		return "installed (" + path + ") but not loaded"
	}
	state := "unknown state"
	if m := launchdState.FindSubmatch(out); m != nil {
		state = string(m[1])
	}
	if m := launchdPID.FindSubmatch(out); m != nil {
		return fmt.Sprintf("installed, %s (pid %s)", state, m[1])
	}
	return "installed, " + state
}

// Hint is extra advice after installing; none on macOS.
func Hint() string { return "" }

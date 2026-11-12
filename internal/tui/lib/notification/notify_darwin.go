// gomuks - A terminal Matrix client written in Go.
// Copyright (C) 2020 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package notification

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var terminalNotifierAvailable = func() bool {
	_, err := exec.LookPath("terminal-notifier")
	return err == nil
}()

const sendScript = `on run {notifText, notifTitle}
	display notification notifText with title "tuimessenger" subtitle notifTitle
end run`

// macOS always takes a notification's icon and app name from the sending
// bundle, so InstallNotifiers puts a copy of terminal-notifier per network
// (with its icon) and tuimessenger.app here.
var notifierDir = func() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "opt", "tuimessenger", "notifiers")
}()

var sourceBundles = map[string]string{
	"whatsapp":    "WhatsApp",
	"telegram":    "Telegram",
	"slack":       "Slack",
	"slackgo":     "Slack",
	"instagram":   "Instagram",
	"instagramgo": "Instagram",
}

// notifierFor returns the terminal-notifier binary to use for a bridge: its
// own bundle if built, else the generic tuimessenger bundle, else brew's copy.
func notifierFor(source string) (path string, custom bool) {
	if notifierDir != "" {
		for _, name := range []string{sourceBundles[source], "tuimessenger"} {
			if name == "" {
				continue
			}
			bin := filepath.Join(notifierDir, name+".app", "Contents", "MacOS", "terminal-notifier")
			if _, err := os.Stat(bin); err == nil {
				return bin, true
			}
		}
	}
	if terminalNotifierAvailable {
		return "terminal-notifier", false
	}
	return "", false
}

// sendTimeout bounds one notification. While macOS's "would like to send notifications" prompt is unanswered,
// terminal-notifier doesn't exit at all (its -timeout only covers replies).
var sendTimeout = 15 * time.Second

// notAuthorizedExit is terminal-notifier's exit status when the app may not show notifications.
const notAuthorizedExit = 3

// runNotifier runs terminal-notifier, giving up after sendTimeout; "not authorized" becomes a NotAuthorizedError.
func runNotifier(bin string, custom bool, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()
	err := exec.CommandContext(ctx, bin, args...).Run()
	var exitErr *exec.ExitError
	switch {
	case ctx.Err() != nil:
		return fmt.Errorf("the notifier didn't finish within %s (is macOS asking for permission?)", sendTimeout)
	case custom && errors.As(err, &exitErr) && exitErr.ExitCode() == notAuthorizedExit:
		app := strings.TrimSuffix(filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(bin)))), ".app")
		return &NotAuthorizedError{App: displayName(app)}
	}
	return err
}

func Send(title, text string, critical, sound bool) error {
	return SendFrom("", title, text, critical, sound)
}

// SendFrom sends a notification attributed to the network it came from.
func SendFrom(source, title, text string, critical, sound bool) error {
	return SendTagged(source, "", title, text, critical, sound)
}

// SendTagged is SendFrom with a tag, so Remove can take the banner back.
func SendTagged(source, tag, title, text string, critical, sound bool) error {
	if bin, custom := notifierFor(source); bin != "" {
		var args []string
		if custom {
			// The bundle's own name ("WhatsApp", "tuimessenger") already heads the
			// banner, so the sender takes the title line.
			args = []string{"-title", title, "-message", text}
		} else {
			args = []string{"-title", "tuimessenger", "-subtitle", title, "-message", text}
		}
		if critical {
			args = append(args, "-timeout", "15")
		} else {
			args = append(args, "-timeout", "4")
		}
		if sound {
			args = append(args, "-sound", "default")
		}
		if tag != "" {
			args = append(args, "-group", tag)
		}
		return runNotifier(bin, custom, args)
	}
	cmd := exec.Command("osascript", "-", text, title)
	if stdin, err := cmd.StdinPipe(); err != nil {
		return fmt.Errorf("failed to get stdin pipe for osascript: %w", err)
	} else if _, err = stdin.Write([]byte(sendScript)); err != nil {
		return fmt.Errorf("failed to write notification script to osascript: %w", err)
	} else if err = cmd.Run(); err != nil {
		return fmt.Errorf("failed to run notification script: %w", err)
	} else if !cmd.ProcessState.Success() {
		return fmt.Errorf("notification script exited unsuccessfully")
	} else {
		return nil
	}
}

// Remove takes back a tagged notification. It must go through the same
// bundle that posted it: apps can only remove their own notifications.
func Remove(source, tag string) error {
	bin, _ := notifierFor(source)
	if bin == "" || tag == "" {
		return nil
	}
	return exec.Command(bin, "-remove", tag).Run()
}

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/whyyor/tuimessenger/internal/login"
	"github.com/whyyor/tuimessenger/internal/server"
	"github.com/whyyor/tuimessenger/internal/service"
	"github.com/whyyor/tuimessenger/internal/tui/lib/notification"
)

const serviceUsage = `Usage: tuimessenger service install|uninstall|status [-data DIR] [-logs DIR]

  install    run the backend in the background: it starts with your login and restarts if it stops
             (a launchd agent on macOS, a systemd user unit on Linux; replaces an existing one)
  uninstall  stop it and remove it
  status     is it installed and running, and does the backend answer`

// startTimeout is how long install waits for the new service's backend to answer.
const startTimeout = 90 * time.Second

func runService(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, serviceUsage)
		return 2
	}
	fs := flag.NewFlagSet("tuimessenger service "+args[0], flag.ExitOnError)
	dataDir := fs.String("data", defaultDataDir(), "tuimessenger's data folder")
	logDir := fs.String("logs", defaultLogDir(), "folder for tuimessenger.log (macOS)")
	_ = fs.Parse(args[1:])
	var err error
	switch args[0] {
	case "install":
		err = installService(*dataDir, *logDir)
	case "uninstall":
		if err = service.Uninstall(); err == nil {
			fmt.Println("Removed. The backend now only runs when you start it (or inside the TUI with Complete setup).")
		}
	case "status":
		fmt.Println("Service:", service.Status())
		if backendAnswers(context.Background()) {
			fmt.Println("Backend: answering at http://" + server.DefaultListen)
		} else {
			fmt.Println("Backend: not answering at http://" + server.DefaultListen)
		}
	default:
		fmt.Fprintln(os.Stderr, serviceUsage)
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}
	return 0
}

func installService(dataDir, logDir string) error {
	exe, err := stableExecutable()
	if err != nil {
		return err
	}
	if dataDir, err = filepath.Abs(dataDir); err != nil {
		return err
	}
	if logDir, err = filepath.Abs(logDir); err != nil {
		return err
	}
	// The service can't ask for a password, so make sure there is one. A backend that's answering already has one.
	if !backendAnswers(context.Background()) {
		if err = ensurePasswordOffline(dataDir); err != nil {
			return err
		}
	}
	fmt.Println("Installing the service…")
	if err = service.Install(service.Spec{Executable: exe, DataDir: dataDir, LogDir: logDir}); err != nil {
		return err
	}
	fmt.Print("Waiting for the backend to answer")
	ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
	defer cancel()
	for !backendAnswers(ctx) {
		if ctx.Err() != nil {
			fmt.Println()
			return fmt.Errorf("installed, but the backend didn't answer within %s; see %s",
				startTimeout, filepath.Join(logDir, "tuimessenger.log"))
		}
		fmt.Print(".")
		time.Sleep(time.Second)
	}
	fmt.Println("\nInstalled: the backend runs in the background, starts with your login and restarts if it stops.")
	if hint := service.Hint(); hint != "" {
		fmt.Println(hint)
	}
	if err = notification.InstallNotifiers(); err != nil {
		fmt.Println("Notification icons not set up:", err)
	}
	return nil
}

// stableExecutable is this binary's path, refusing temporary builds that would vanish under the service.
func stableExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return "", err
	}
	if strings.Contains(exe, "go-build") || strings.HasPrefix(exe, filepath.Clean(os.TempDir())) {
		return "", fmt.Errorf("%s is a temporary build; install tuimessenger somewhere permanent and run it from there", exe)
	}
	return brewOptPath(exe), nil
}

// brewOptPath turns Homebrew's versioned <prefix>/Cellar/tuimessenger/<version>/… into <prefix>/opt/tuimessenger/…,
// which brew upgrade repoints, so the service doesn't run a deleted version.
func brewOptPath(exe string) string {
	const cellar = "/Cellar/tuimessenger/"
	i := strings.Index(exe, cellar)
	if i < 0 {
		return exe
	}
	_, rest, ok := strings.Cut(exe[i+len(cellar):], "/")
	if !ok {
		return exe
	}
	opt := filepath.Join(exe[:i], "opt", "tuimessenger", rest)
	if _, err := os.Stat(opt); err != nil {
		return exe
	}
	return opt
}

// ensurePasswordOffline opens the stopped backend just long enough to check for (or ask for) the password.
func ensurePasswordOffline(dataDir string) error {
	ctx := context.Background()
	a, err := server.Open(ctx, server.Options{Log: zerolog.Nop(), DataDir: dataDir})
	if errors.Is(err, server.ErrLocked) {
		return fmt.Errorf("%w; it may be starting up, or running inside tuimessenger tui: close that and try again", err)
	} else if err != nil {
		return err
	}
	defer a.Close()
	return ensurePassword(ctx, a.App, login.NewTerminal(), false)
}

// backendAnswers reports whether a tuimessenger backend answers on the default address.
func backendAnswers(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+server.DefaultListen+"/_matrix/client/versions", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func runNotifiers() int {
	if err := notification.InstallNotifiers(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}
	fmt.Println("Notification apps are set up.")
	return 0
}

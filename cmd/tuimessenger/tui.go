package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/whyyor/tuimessenger/internal/tuiclient"
)

// runTUI is `tuimessenger tui`: the terminal client, with its engine running in this process.
func runTUI(args []string) int {
	fs := flag.NewFlagSet("tuimessenger tui", flag.ExitOnError)
	var opts tuiclient.Options
	fs.StringVar(&opts.DataDir, "data", defaultDataDir(), "tuimessenger's data folder (the client uses its tui subfolder)")
	fs.StringVar(&opts.Server, "server", "http://127.0.0.1:8008", "address of the tuimessenger backend")
	fs.StringVar(&opts.Username, "user", "me", "username to log in with")
	fs.StringVar(&opts.LogDir, "logs", defaultLogDir(), "folder for terminal.log and client.log")
	_ = fs.Parse(args)
	if err := tuiclient.Run(context.Background(), opts); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}
	return 0
}

// defaultLogDir is ~/Library/Logs/tuimessenger on macOS and ~/.local/state/tuimessenger elsewhere.
func defaultLogDir() string {
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Logs", "tuimessenger")
	}
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "tuimessenger")
	}
	return filepath.Join(home, ".local", "state", "tuimessenger")
}

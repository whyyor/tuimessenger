//go:build devdemo

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/whyyor/tuimessenger/internal/demo"
)

func init() {
	devCommands["demo"] = runDemo
}

const demoUsage = `Usage (dev builds with -tags devdemo only):
  tuimessenger demo serve [-data DIR] [-media DIR] [-listen ADDR]   fake backend with made-up chats
  tuimessenger demo say [-listen ADDR] CHAT TEXT                     a contact writes (e.g. maya, trek, priya)`

func runDemo(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, demoUsage)
		return 2
	}
	home, _ := os.UserHomeDir()
	fs := flag.NewFlagSet("tuimessenger demo "+args[0], flag.ExitOnError)
	dataDir := fs.String("data", filepath.Join(os.TempDir(), "tuimessenger-demo"), "the demo backend's data folder")
	mediaDir := fs.String("media", filepath.Join(home, ".cache", "tuimessenger-demo-media"), "folder with the demo photos and video (scripts/demo-media.sh)")
	listen := fs.String("listen", "127.0.0.1:18008", "address of the demo backend")
	_ = fs.Parse(args[1:])
	switch args[0] {
	case "serve":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := demo.Serve(ctx, demo.Options{Log: newLogger(false), DataDir: *dataDir, MediaDir: *mediaDir, Listen: *listen}); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return 1
		}
	case "say":
		if fs.NArg() != 2 {
			fmt.Fprintln(os.Stderr, demoUsage)
			return 2
		}
		body, _ := json.Marshal(map[string]any{"chat": fs.Arg(0), "text": fs.Arg(1)})
		resp, err := http.Post("http://"+*listen+"/_demo/say", "application/json", bytes.NewReader(body))
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return 1
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			fmt.Fprintln(os.Stderr, "Error:", resp.Status)
			return 1
		}
	default:
		fmt.Fprintln(os.Stderr, demoUsage)
		return 2
	}
	return 0
}

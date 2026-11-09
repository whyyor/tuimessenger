// Package server runs the backend (the networks' bridges and the client API), for `tuimessenger` on its own and
// inside `tuimessenger tui` for Complete setup.
package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/app"
	"github.com/whyyor/tuimessenger/internal/config"
)

// ServerName is the name of our pretend Matrix server; Owner is "you", the only user.
const (
	ServerName = "tuimessenger.localhost"
	Owner      = id.UserID("@me:" + ServerName)
)

// DefaultListen is where the client API listens unless told otherwise.
const DefaultListen = "127.0.0.1:8008"

// DefaultDataDir is ~/Library/Application Support/tuimessenger on macOS and ~/.local/share/tuimessenger elsewhere.
func DefaultDataDir() string {
	if runtime.GOOS == "darwin" {
		if dir, err := os.UserConfigDir(); err == nil {
			return filepath.Join(dir, "tuimessenger")
		}
	}
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "tuimessenger")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "tuimessenger")
}

// ErrLocked means another backend is already using the data folder.
var ErrLocked = errors.New("another tuimessenger backend is already running with this data folder")

type Options struct {
	Log      zerolog.Logger
	DataDir  string
	Networks []string // empty: the ones in config.yaml
}

// Backend is a backend that owns its data folder until Stop or Close.
type Backend struct {
	*app.App
	lock *os.File
}

// Open locks the data folder and prepares the networks; nothing connects until Start.
func Open(ctx context.Context, opts Options) (*Backend, error) {
	if err := os.MkdirAll(opts.DataDir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create data folder: %w", err)
	}
	lock, err := lockDir(opts.DataDir)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(filepath.Join(opts.DataDir, "config.yaml"))
	if err != nil {
		_ = lock.Close()
		return nil, err
	}
	// Telegram without its keys is left out rather than stopping everything; clients then offer the keys form.
	networks, wantsTelegram := cfg.EnabledNetworks(), cfg.WantsTelegram()
	if len(opts.Networks) > 0 {
		networks, wantsTelegram = cfg.Runnable(opts.Networks), slices.Contains(opts.Networks, "telegram")
	}
	if wantsTelegram && !cfg.HasTelegramKeys() {
		opts.Log.Warn().Msg("Telegram needs its app keys (my.telegram.org); add them in the TUI's Networks panel or config.yaml")
	}
	if len(networks) == 0 {
		return nil, errors.New("no networks can run: choose them with -network or networks: in config.yaml")
	}
	a, err := app.New(ctx, app.Options{
		Log: opts.Log, Config: cfg, DataDir: opts.DataDir, Networks: networks, ServerName: ServerName, Owner: Owner,
		OfferTelegramSetup: wantsTelegram && !cfg.HasTelegramKeys(),
	})
	if err != nil {
		_ = lock.Close()
		return nil, err
	}
	return &Backend{App: a, lock: lock}, nil
}

// lockDir takes backend.lock in the data folder; the lock goes away when the file is closed or the process exits.
func lockDir(dir string) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(dir, "backend.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("%w (%s)", ErrLocked, dir)
	}
	return f, nil
}

// Stop stops the networks, closes the databases and releases the data folder.
func (b *Backend) Stop() {
	b.App.Stop()
	_ = b.lock.Close()
}

// Close is Stop for a backend that was never started.
func (b *Backend) Close() {
	b.App.Close()
	_ = b.lock.Close()
}

// Serve answers the client API on listener until ctx is cancelled.
func (b *Backend) Serve(ctx context.Context, listener net.Listener) error {
	server := &http.Server{Handler: b.API, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	if err := server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

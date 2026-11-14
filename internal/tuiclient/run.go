package tuiclient

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"

	"github.com/rs/zerolog"
	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli"
	"github.com/whyyor/tuimessenger/internal/server"
	"github.com/whyyor/tuimessenger/internal/service"
	"github.com/whyyor/tuimessenger/internal/tui"
	"github.com/whyyor/tuimessenger/internal/tui/config"
)

// Options are what `tuimessenger tui` needs to start.
type Options struct {
	DataDir  string // tuimessenger's data folder; the client keeps its files in DataDir/tui
	LogDir   string // where terminal.log and client.log go
	Server   string // address of the tuimessenger backend
	Username string
}

// memoryLimit is a soft cap for the client process: near it, Go collects garbage harder and returns memory sooner.
// Live data is around 12 MB, so this only trims slack. GOMEMLIMIT in the environment overrides it.
const memoryLimit = 128 << 20

// Run starts hicli and the terminal client. When it works, it doesn't return: the TUI exits the program itself.
func Run(ctx context.Context, opts Options) error {
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(memoryLimit)
	}
	dir := filepath.Join(opts.DataDir, "tui")
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o700); err != nil {
		return err
	} else if err = os.MkdirAll(opts.LogDir, 0o700); err != nil {
		return err
	}
	if err := lockDir(dir); err != nil {
		return err
	}
	logFile, err := os.OpenFile(filepath.Join(opts.LogDir, "client.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	log := zerolog.New(logFile).With().Timestamp().Logger()
	ctx = log.WithContext(ctx)

	db, err := dbutil.NewFromConfig("tuimessenger-client", dbutil.Config{
		PoolConfig: dbutil.PoolConfig{
			Type:         "sqlite3-fk-wal",
			URI:          "file:" + filepath.Join(dir, "client.db") + "?_txlock=immediate",
			MaxOpenConns: 5,
			MaxIdleConns: 1,
		},
	}, dbutil.ZeroLogger(log.With().Str("component", "client-db").Logger()))
	if err != nil {
		return fmt.Errorf("failed to open the client database: %w", err)
	}
	hicli.InitialDeviceDisplayName = "tuimessenger" // how this login shows up in the backend's device list
	b := newBackend(log)
	// The pickle key protects encryption keys; tuimessenger has none, so a fixed value is fine.
	b.hc = hicli.New(db, nil, log.With().Str("component", "hicli").Logger(), []byte("tuimessenger"), b.onEvent)
	// Strip reply fallbacks ("> quoted original" in a reply's body) when saving, as hicli always did before the
	// option existed: the TUI shows the quoted message itself.
	b.hc.RemoveFallbacks = true

	cfg := config.NewConfigAt(filepath.Join(dir, "config"), opts.LogDir)
	cfg.Load()
	local := &localBackend{opts: opts}
	userID, err := b.hc.DB.Account.GetFirstUserID(ctx)
	if err != nil {
		return err
	}
	if userID != "" {
		// Logged in: make sure the backend answers before the engine starts, which would otherwise retry silently.
		if err = ensureBackend(ctx, b.hc, userID, cfg.Mode, local, opts); err != nil {
			return err
		}
	}
	err = b.hc.Start(ctx, userID, nil)
	if err != nil {
		local.Stop()
	}
	if errors.Is(err, mautrix.MUnknownToken) {
		return fmt.Errorf("the saved login isn't valid anymore; delete %s and run this again", filepath.Join(dir, "client.db"))
	} else if err != nil {
		return fmt.Errorf("can't start the client (is tuimessenger running at %s?): %w", opts.Server, err)
	}
	ui := tui.New()
	ui.Config = cfg
	ui.Backend = b
	ui.Local, ui.OnExit = local, local.Stop
	ui.LoggedIn = b.hc.IsLoggedIn() // without a login, the TUI opens on the setup screen
	ui.Setup = tui.SetupDefaults{Server: opts.Server, Username: opts.Username}
	ui.Run()
	return nil
}

// ensureBackend brings this machine's backend up when it isn't answering (the service, or the backend inside the TUI
// for Complete setup), and otherwise says clearly what's wrong instead of hanging.
func ensureBackend(ctx context.Context, hc *hicli.HiClient, userID id.UserID, mode string, local *localBackend, opts Options) error {
	acc, err := hc.DB.Account.Get(ctx, userID)
	if err != nil || acc == nil {
		return err
	}
	base := acc.HomeserverURL
	if err = tui.CheckBackend(ctx, base); !tui.IsUnreachable(err) {
		return nil // answering, or answering wrongly: the engine reports that itself
	}
	installed := service.InstalledFor(opts.DataDir, server.DefaultDataDir())
	if !canStartHere(mode, installed, base, opts.Server) {
		return fmt.Errorf("no tuimessenger backend is answering at %s; check that it's running and reachable", base)
	}
	fmt.Println("Starting the backend…")
	if _, err = local.Start(ctx); err != nil {
		return fmt.Errorf("can't start the backend: %w", err)
	}
	return nil
}

// canStartHere decides whether a backend that isn't answering is this machine's to start: Complete setup, or a
// machine set up before modes existed that has the service installed; never for a backend elsewhere.
func canStartHere(mode string, serviceInstalled bool, base, localServer string) bool {
	if strings.TrimRight(base, "/") != strings.TrimRight(localServer, "/") {
		return false
	}
	return mode == tui.ModeComplete || (mode == "" && serviceInstalled)
}

// lockFile stays open for the whole run: if it were garbage collected, Go would close it and release the lock.
var lockFile *os.File

// lockDir makes sure only one client uses the folder; the lock goes away when the process exits.
func lockDir(dir string) error {
	f, err := os.OpenFile(filepath.Join(dir, "lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return errors.New("tuimessenger tui is already running")
	}
	lockFile = f
	return nil
}

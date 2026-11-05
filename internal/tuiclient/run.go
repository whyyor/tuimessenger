package tuiclient

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"syscall"

	"github.com/rs/zerolog"
	"go.mau.fi/util/dbutil"
	"golang.org/x/term"
	"maunium.net/go/mautrix"

	"github.com/whyyor/tuimessenger/internal/hicli"
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
	b := newBackend(log)
	// The pickle key protects encryption keys; tuimessenger has none, so a fixed value is fine.
	b.hc = hicli.New(db, nil, log.With().Str("component", "hicli").Logger(), []byte("tuimessenger"), b.onEvent)

	userID, err := b.hc.DB.Account.GetFirstUserID(ctx)
	if err != nil {
		return err
	}
	err = b.hc.Start(ctx, userID, nil)
	if errors.Is(err, mautrix.MUnknownToken) {
		return fmt.Errorf("the saved login isn't valid anymore; delete %s and run this again", filepath.Join(dir, "client.db"))
	} else if err != nil {
		return fmt.Errorf("can't start the client (is tuimessenger running at %s?): %w", opts.Server, err)
	}
	if !b.hc.IsLoggedIn() {
		if err = logIn(ctx, b.hc, opts); err != nil {
			return err
		}
	}

	ui := tui.NewGomuksTUI()
	ui.Config = config.NewConfigAt(filepath.Join(dir, "config"), opts.LogDir)
	ui.Backend = b
	ui.Run()
	return nil
}

// logIn asks for the password once; hicli keeps the session in its database after that.
func logIn(ctx context.Context, hc *hicli.HiClient, opts Options) error {
	fmt.Printf("Log in to tuimessenger at %s as %s.\nPassword: ", opts.Server, opts.Username)
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return err
	}
	if err = hc.LoginPassword(ctx, opts.Server, opts.Username, string(password)); err != nil {
		return fmt.Errorf("login failed: %w", err)
	}
	return nil
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

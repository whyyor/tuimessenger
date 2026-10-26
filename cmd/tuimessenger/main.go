// Command tuimessenger runs chat bridges locally.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	"go.mau.fi/util/dbutil"
	_ "go.mau.fi/util/dbutil/litestream" // registers the SQLite drivers (needs cgo)
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/commands"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/bridges"
	"github.com/whyyor/tuimessenger/internal/login"
	"github.com/whyyor/tuimessenger/internal/matrixconn"
	"github.com/whyyor/tuimessenger/internal/media"
	"github.com/whyyor/tuimessenger/internal/store"
)

// serverName is the name of our pretend Matrix server; ownerMXID is "you", the only user.
const (
	serverName = "tuimessenger.localhost"
	ownerMXID  = id.UserID("@me:" + serverName)
)

func main() {
	network := flag.String("network", "", "network to run, one of: "+strings.Join(bridges.Available(), ", "))
	dataDir := flag.String("data", "data", "folder for databases and sessions")
	addLogin := flag.Bool("login", false, "add a new login even if one already exists")
	debug := flag.Bool("debug", false, "show debug logs")
	flag.Parse()

	log := newLogger(*debug)
	if *network == "" {
		fmt.Fprintln(os.Stderr, "Please choose a network with -network. Available:", bridges.Available())
		os.Exit(2)
	}

	// ctx is cancelled when you press Ctrl+C, which tells everything to stop.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, log, *network, *dataDir, *addLogin); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal().Err(err).Msg("Stopped with an error")
	}
}

func run(ctx context.Context, log zerolog.Logger, name, dataDir string, addLogin bool) error {
	network, err := bridges.New(name)
	if err != nil {
		return err
	}
	dir := filepath.Join(dataDir, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create data folder: %w", err)
	}
	db, err := openDB(filepath.Join(dir, "bridge.db"), log)
	if err != nil {
		return err
	}
	defer db.Close()

	st, err := store.Open(ctx, filepath.Join(dataDir, "store.db"), log)
	if err != nil {
		return err
	}
	defer st.Close()
	med, err := media.New(filepath.Join(dataDir, "media"))
	if err != nil {
		return err
	}

	log = log.With().Str("network", name).Logger()
	matrix := matrixconn.New(log, st, med, name, serverName, ownerMXID)
	br := bridgev2.NewBridge(networkid.BridgeID(name), db, log, &network.Config, matrix, network.Connector, commands.NewProcessor)
	if err := br.Start(ctx); err != nil {
		return fmt.Errorf("failed to start bridge: %w", err)
	}
	defer br.StopWithTimeout(5 * time.Second)

	user, err := br.GetUserByMXID(ctx, ownerMXID)
	if err != nil {
		return err
	}
	if addLogin || len(user.GetUserLogins()) == 0 {
		return loginOnce(ctx, br, user)
	}

	log.Info().Msg("Running. Press Ctrl+C to stop.")
	<-ctx.Done()
	return nil
}

// loginOnce adds one login and exits; the next normal start connects it.
func loginOnce(ctx context.Context, br *bridgev2.Bridge, user *bridgev2.User) error {
	term := login.NewTerminal()
	flowID, err := term.ChooseFlow(br.Network.GetLoginFlows())
	if err != nil {
		return err
	}
	proc, err := br.Network.CreateLogin(ctx, user, flowID)
	if err != nil {
		return fmt.Errorf("failed to start login: %w", err)
	}
	userLogin, err := term.Run(ctx, proc)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}
	fmt.Printf("\nLogged in as %q. Run the same command again (without -login) to connect.\n", userLogin.RemoteName)
	return nil
}

func openDB(path string, log zerolog.Logger) (*dbutil.Database, error) {
	db, err := dbutil.NewFromConfig("tuimessenger", dbutil.Config{
		PoolConfig: dbutil.PoolConfig{
			Type:         "sqlite3-fk-wal-fullsync",
			URI:          "file:" + path + "?_txlock=immediate",
			MaxOpenConns: 5,
			MaxIdleConns: 1,
		},
	}, dbutil.ZeroLogger(log.With().Str("component", "db").Logger()))
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	return db, nil
}

func newLogger(debug bool) zerolog.Logger {
	level := zerolog.InfoLevel
	if debug {
		level = zerolog.DebugLevel
	}
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.TimeOnly}).
		Level(level).With().Timestamp().Logger()
	zlog.Logger = logger // some libraries log through zerolog's global logger
	return logger
}

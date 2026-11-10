// Command tuimessenger runs chat bridges locally, with a Matrix client API for the tuimessenger client on top.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	"golang.org/x/term"
	"maunium.net/go/mautrix/bridgev2"

	"github.com/whyyor/tuimessenger/internal/app"
	"github.com/whyyor/tuimessenger/internal/bridges"
	"github.com/whyyor/tuimessenger/internal/login"
	"github.com/whyyor/tuimessenger/internal/server"
)

// ownerMXID is "you", the only user.
const ownerMXID = server.Owner

type flags struct {
	networks    []string
	dataDir     string
	listen      string
	login       string
	setPassword bool
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "tui":
			os.Exit(runTUI(os.Args[2:]))
		case "agent":
			os.Exit(runAgent(os.Args[2:]))
		case "service":
			os.Exit(runService(os.Args[2:]))
		case "notifiers":
			os.Exit(runNotifiers())
		case "version", "-version", "--version":
			os.Exit(runVersion())
		}
		if run, ok := devCommands[os.Args[1]]; ok {
			os.Exit(run(os.Args[2:]))
		}
	}
	networks := flag.String("network", "", "comma-separated networks to run (default: networks in config.yaml, else all that can run): "+strings.Join(bridges.Available(), ", "))
	var f flags
	flag.StringVar(&f.dataDir, "data", defaultDataDir(), "folder for databases, sessions, media and config.yaml")
	flag.StringVar(&f.listen, "listen", server.DefaultListen, "address the Matrix client API listens on")
	flag.StringVar(&f.login, "login", "", "log in to this network (one of -network) before running")
	flag.BoolVar(&f.setPassword, "set-password", false, "change the password clients log in with")
	debug := flag.Bool("debug", false, "show debug logs")
	flag.Parse()

	log := newLogger(*debug)
	for _, name := range strings.Split(*networks, ",") {
		if name = strings.TrimSpace(name); name != "" {
			f.networks = append(f.networks, name)
		}
	}
	// ctx is cancelled when you press Ctrl+C, which tells everything to stop.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, log, f); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal().Err(err).Msg("Stopped with an error")
	}
}

func run(ctx context.Context, log zerolog.Logger, f flags) error {
	a, err := server.Open(ctx, server.Options{Log: log, DataDir: f.dataDir, Networks: f.networks})
	if err != nil {
		return err
	}
	term := login.NewTerminal()
	if err = ensurePassword(ctx, a.App, term, f.setPassword); err != nil {
		a.Close()
		return err
	}
	if err = a.Start(ctx); err != nil {
		a.Stop()
		return err
	}
	defer a.Stop()

	if f.login != "" {
		br := a.Bridge(f.login)
		if br == nil {
			return fmt.Errorf("-login %s isn't one of the networks being run", f.login)
		}
		if err = addLogin(ctx, br, term); err != nil {
			return err
		}
	}
	return serve(ctx, log, a, f.listen)
}

// ensurePassword asks for the client password the first time (or when -set-password is given).
func ensurePassword(ctx context.Context, a *app.App, term *login.Terminal, change bool) error {
	has, err := a.API.HasPassword(ctx)
	if err != nil || (has && !change) {
		return err
	}
	if !isTerminal() {
		return errors.New("no client password is set yet: run tuimessenger once in a terminal to choose one")
	}
	fmt.Println("Choose the password tuimessenger will log in with (user \"" + ownerMXID.Localpart() + "\").")
	for {
		first, err := term.AskSecret("Password: ")
		if err != nil {
			return err
		}
		second, err := term.AskSecret("Again: ")
		if err != nil {
			return err
		}
		if first != second {
			fmt.Println("The two didn't match, try again.")
			continue
		}
		if err = a.API.SetPassword(ctx, first); err != nil {
			fmt.Println(err)
			continue
		}
		return nil
	}
}

func addLogin(ctx context.Context, br *bridgev2.Bridge, term *login.Terminal) error {
	user, err := br.GetUserByMXID(ctx, ownerMXID)
	if err != nil {
		return err
	}
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
	fmt.Printf("\nLogged in to %s as %q.\n", br.ID, userLogin.RemoteName)
	return nil
}

// serve runs the client API until Ctrl+C.
func serve(ctx context.Context, log zerolog.Logger, a *server.Backend, addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	log.Info().Msgf("Ready. In tuimessenger, log in to http://%s as %s. Press Ctrl+C to stop.", listener.Addr(), ownerMXID.Localpart())
	return a.Serve(ctx, listener)
}

func defaultDataDir() string { return server.DefaultDataDir() }

func isTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func newLogger(debug bool) zerolog.Logger {
	level := zerolog.InfoLevel
	if debug {
		level = zerolog.DebugLevel
	}
	// No colour codes when the output goes to a file (the service's log), only in a terminal.
	noColor := !term.IsTerminal(int(os.Stderr.Fd()))
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.TimeOnly, NoColor: noColor}).
		Level(level).With().Timestamp().Logger()
	zlog.Logger = logger // some libraries log through zerolog's global logger
	return logger
}

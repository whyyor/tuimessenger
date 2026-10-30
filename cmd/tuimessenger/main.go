// Command tuimessenger runs chat bridges locally, with a Matrix client API for gomuks on top.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	"golang.org/x/term"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/app"
	"github.com/whyyor/tuimessenger/internal/bridges"
	"github.com/whyyor/tuimessenger/internal/login"
)

// serverName is the name of our pretend Matrix server; ownerMXID is "you", the only user.
const (
	serverName = "tuimessenger.localhost"
	ownerMXID  = id.UserID("@me:" + serverName)
)

type flags struct {
	networks    []string
	dataDir     string
	listen      string
	login       string
	setPassword bool
}

func main() {
	networks := flag.String("network", "", "comma-separated networks to run: "+strings.Join(bridges.Available(), ", "))
	var f flags
	flag.StringVar(&f.dataDir, "data", "data", "folder for databases, sessions and media")
	flag.StringVar(&f.listen, "listen", "127.0.0.1:8008", "address the Matrix client API listens on")
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
	if len(f.networks) == 0 {
		fmt.Fprintln(os.Stderr, "Please choose networks with -network. Available:", bridges.Available())
		os.Exit(2)
	}

	// ctx is cancelled when you press Ctrl+C, which tells everything to stop.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, log, f); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal().Err(err).Msg("Stopped with an error")
	}
}

func run(ctx context.Context, log zerolog.Logger, f flags) error {
	a, err := app.New(ctx, app.Options{
		Log: log, DataDir: f.dataDir, Networks: f.networks, ServerName: serverName, Owner: ownerMXID,
	})
	if err != nil {
		return err
	}
	term := login.NewTerminal()
	if err = ensurePassword(ctx, a, term, f.setPassword); err != nil {
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
	fmt.Println("Choose the password gomuks will log in with (user \"" + ownerMXID.Localpart() + "\").")
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
func serve(ctx context.Context, log zerolog.Logger, a *app.App, addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	server := &http.Server{Handler: a.API, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	log.Info().Msgf("Ready. In gomuks, log in to http://%s as %s. Press Ctrl+C to stop.", listener.Addr(), ownerMXID.Localpart())
	if err = server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func isTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
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

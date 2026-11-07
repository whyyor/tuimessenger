package tuiclient

import (
	"context"
	"errors"
	"fmt"
	stdlog "log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/whyyor/tuimessenger/internal/server"
	"github.com/whyyor/tuimessenger/internal/service"
	"github.com/whyyor/tuimessenger/internal/tui"
)

// localStopTimeout caps how long quitting waits for the networks to disconnect.
const localStopTimeout = 5 * time.Second

// localBackend runs the backend inside the TUI for Complete setup: it starts on demand and stops when the TUI exits.
type localBackend struct {
	opts Options

	mu     sync.Mutex
	srv    *server.Backend
	cancel context.CancelFunc
}

var _ tui.LocalBackend = (*localBackend)(nil)

func (l *localBackend) Running() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.srv != nil
}

// listenAddr is host:port of the backend address the TUI uses, e.g. 127.0.0.1:8008.
func listenAddr(server string) (string, error) {
	parsed, err := url.Parse(server)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("%q isn't an address like http://127.0.0.1:8008", server)
	}
	if parsed.Port() == "" {
		return net.JoinHostPort(parsed.Hostname(), "80"), nil
	}
	return parsed.Host, nil
}

// Start brings this machine's backend up: the service if one is installed for this data folder (it keeps running
// after the TUI), otherwise the backend inside this process.
func (l *localBackend) Start(ctx context.Context) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.srv == nil {
		if service.InstalledFor(l.opts.DataDir, server.DefaultDataDir()) {
			// A service backend asked for its password when it was installed.
			return true, startService(ctx, l.opts.Server)
		}
		if err := l.start(ctx); err != nil {
			return false, err
		}
	}
	return l.srv.API.HasPassword(ctx)
}

// serviceStartTimeout is how long to wait for a started service's backend to answer.
const serviceStartTimeout = 60 * time.Second

func startService(ctx context.Context, base string) error {
	if err := service.Start(); err != nil {
		return fmt.Errorf("couldn't start the tuimessenger service: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, serviceStartTimeout)
	defer cancel()
	for {
		err := tui.CheckBackend(ctx, base)
		if err == nil {
			return nil
		} else if !tui.IsUnreachable(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return errors.New("the tuimessenger service started but its backend isn't answering; see tuimessenger.log or `tuimessenger service status`")
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (l *localBackend) start(ctx context.Context) error {
	addr, err := listenAddr(l.opts.Server)
	if err != nil {
		return err
	}
	// Its log goes where the service's does; it must never write to the terminal the TUI draws on.
	logFile, err := os.OpenFile(filepath.Join(l.opts.LogDir, "tuimessenger.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	stdlog.SetOutput(logFile)
	log := zerolog.New(zerolog.ConsoleWriter{Out: logFile, TimeFormat: time.TimeOnly, NoColor: true}).
		With().Timestamp().Str("run", "inside tui").Logger()
	srv, err := server.Open(ctx, server.Options{Log: log, DataDir: l.opts.DataDir})
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		srv.Close()
		return fmt.Errorf("can't listen on %s: %w", addr, err)
	}
	runCtx, cancel := context.WithCancel(log.WithContext(context.Background()))
	if err = srv.Start(runCtx); err != nil {
		cancel()
		_ = listener.Close()
		srv.Stop()
		return err
	}
	go func() {
		if err := srv.Serve(runCtx, listener); err != nil {
			log.Err(err).Msg("Client API stopped")
		}
	}()
	log.Info().Str("address", addr).Msg("Backend running inside the TUI")
	l.srv, l.cancel = srv, cancel
	return nil
}

func (l *localBackend) SetPassword(ctx context.Context, password string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.srv == nil {
		return errors.New("the backend isn't running here")
	}
	return l.srv.API.SetPassword(ctx, password)
}

// Stop disconnects the networks and closes the databases, giving up after localStopTimeout.
func (l *localBackend) Stop() {
	l.mu.Lock()
	srv, cancel := l.srv, l.cancel
	l.srv = nil
	l.mu.Unlock()
	if srv == nil {
		return
	}
	cancel()
	done := make(chan struct{})
	go func() {
		srv.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(localStopTimeout):
	}
}

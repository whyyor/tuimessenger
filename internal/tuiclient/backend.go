// Package tuiclient runs the terminal client and its engine (hicli) together in one process.
package tuiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix"

	"github.com/whyyor/tuimessenger/internal/hicli"
	"github.com/whyyor/tuimessenger/internal/hicli/jsoncmd"
	"github.com/whyyor/tuimessenger/internal/rpc"
	"github.com/whyyor/tuimessenger/internal/rpc/client"
)

const (
	// initialBatchSize is how many rooms go into each initial sync message, the same as the gomuks daemon.
	initialBatchSize = 100
	// reconnectTimeout caps how long resync waits for the backend's answer to "reconnect broken networks".
	reconnectTimeout = 10 * time.Second
)

// Backend gives the TUI the same view of hicli that the gomuks daemon's websocket did. Commands and events are
// copied through JSON, so the TUI never shares objects with hicli, which keeps changing them from its own goroutines.
type Backend struct {
	*rpc.GomuksRPC // the typed commands; they reach hicli through submit

	hc     *hicli.HiClient
	log    zerolog.Logger
	events *queue

	mu     sync.Mutex
	live   bool  // false while the initial data is being queued; events from hicli wait in held meanwhile
	held   []any // events from hicli that arrived while live was false
	runCtx context.Context

	initialMu sync.Mutex
	resyncing atomic.Bool
}

var _ client.Backend = (*Backend)(nil)

func newBackend(log zerolog.Logger) *Backend {
	b := &Backend{log: log, events: newQueue()}
	b.GomuksRPC = rpc.NewWithTransport(b.submit)
	return b
}

func (b *Backend) submit(ctx context.Context, cmd *jsoncmd.Container[json.RawMessage]) *jsoncmd.Container[json.RawMessage] {
	return b.hc.SubmitJSONCommand(ctx, cmd)
}

// onEvent is hicli's event handler. It must not block: hicli calls it from its sync goroutine.
func (b *Backend) onEvent(evt any) {
	copied := b.copyEvent(evt)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.live {
		b.events.push(copied)
	} else {
		b.held = append(b.held, copied)
	}
}

// copyEvent encodes and decodes an event the way it travelled over the websocket.
func (b *Backend) copyEvent(evt any) (copied any) {
	defer func() {
		if err := recover(); err != nil {
			b.log.Warn().Any("error", err).Type("event_type", evt).Msg("Passing an event on without copying it")
			copied = evt
		}
	}()
	data, err := json.Marshal(evt)
	if err != nil {
		b.log.Err(err).Type("event_type", evt).Msg("Failed to copy event")
		return evt
	}
	container := &jsoncmd.Container[json.RawMessage]{Command: jsoncmd.EventTypeName(evt), Data: data}
	return rpc.ParseEvent(b.log.WithContext(context.Background()), container)
}

// Run sends the current state, then hicli's events, to handler until ctx is cancelled.
func (b *Backend) Run(ctx context.Context, handler rpc.EventHandler, onState rpc.ConnStateHandler) error {
	if onState == nil {
		onState = func(rpc.ConnState, error) {}
	}
	onState(rpc.ConnStateConnecting, nil)
	b.mu.Lock()
	b.runCtx = ctx
	b.mu.Unlock()
	b.queueInitial(ctx)
	onState(rpc.ConnStateConnected, nil)
	for {
		evt, ok := b.events.pop(ctx)
		if !ok {
			break
		}
		b.deliver(ctx, handler, evt)
	}
	onState(rpc.ConnStateDisconnected, nil)
	return ctx.Err()
}

func (b *Backend) deliver(ctx context.Context, handler rpc.EventHandler, evt any) {
	defer func() {
		if err := recover(); err != nil {
			b.log.Error().Any("error", err).Bytes(zerolog.ErrorStackFieldName, debug.Stack()).Msg("Panic in event handler")
		}
	}()
	handler(ctx, evt)
}

// queueInitial queues what the daemon sends a new connection: client state, sync status, and when logged in,
// every room and InitComplete.
// Events hicli emits meanwhile are held back and queued after it, so they don't land in the middle.
func (b *Backend) queueInitial(ctx context.Context) {
	b.initialMu.Lock()
	defer b.initialMu.Unlock()
	b.mu.Lock()
	b.held = nil // anything from before this point is already part of the initial data
	b.live = false
	b.mu.Unlock()

	b.events.push(b.copyEvent(b.hc.State()))
	if status := b.hc.SyncStatus.Load(); status != nil {
		b.events.push(b.copyEvent(status))
	}
	// Like the daemon: without a verified login there are no rooms yet, and hicli sends them itself after its first sync.
	if b.hc.IsLoggedInAndVerified() {
		for payload := range b.hc.GetInitialSync(ctx, initialBatchSize, 0) {
			b.events.push(b.copyEvent(payload))
		}
		b.events.push(&jsoncmd.InitComplete{})
	}

	b.mu.Lock()
	b.events.push(b.held...)
	b.held = nil
	b.live = true
	b.mu.Unlock()
}

func (b *Backend) runCtxOrNil() context.Context {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.runCtx
}

// Resync is what the resync key does: reconnect broken networks on the backend, restart syncing with the backend,
// and send the whole state to the TUI again. Presses while one is running are ignored.
func (b *Backend) Resync() {
	ctx := b.runCtxOrNil()
	if ctx == nil || !b.resyncing.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer b.resyncing.Store(false)
		if b.hc.IsLoggedInAndVerified() {
			b.reconnectNetworks(ctx)
			go b.hc.Sync() // stops the running sync loop and starts a new one
		}
		b.queueInitial(ctx)
	}()
}

// reconnectNetworks asks the backend to reconnect networks whose connection looks broken, e.g. a WhatsApp socket
// that stopped answering, so messages stuck there arrive now instead of after a timeout.
func (b *Backend) reconnectNetworks(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, reconnectTimeout)
	defer cancel()
	var resp struct {
		Reconnecting []string `json:"reconnecting"`
	}
	url := b.hc.Client.BuildURL(mautrix.ClientURLPath{"unstable", "com.tuimessenger", "reconnect"})
	if _, err := b.hc.Client.MakeRequest(ctx, http.MethodPost, url, struct{}{}, &resp); err != nil {
		b.log.Warn().Err(err).Msg("Backend didn't reconnect networks")
	} else if len(resp.Reconnecting) > 0 {
		b.log.Info().Strs("networks", resp.Reconnecting).Msg("Backend is reconnecting networks")
	}
}

// Disconnect stops syncing; the TUI calls it right before the program exits.
func (b *Backend) Disconnect() {
	b.hc.Stop()
}

// Package tuiclient runs the terminal client and its engine (hicli) together in one process.
package tuiclient

import (
	"context"
	"encoding/json"
	"runtime/debug"
	"sync"

	"github.com/rs/zerolog"

	"github.com/whyyor/tuimessenger/internal/hicli"
	"github.com/whyyor/tuimessenger/internal/hicli/jsoncmd"
	"github.com/whyyor/tuimessenger/internal/rpc"
	"github.com/whyyor/tuimessenger/internal/rpc/client"
)

// initialBatchSize is how many rooms go into each initial sync message, the same as the gomuks daemon.
const initialBatchSize = 100

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

// Resync sends the whole state again, the in-process version of reconnecting to the daemon.
func (b *Backend) Resync() {
	b.mu.Lock()
	ctx := b.runCtx
	b.mu.Unlock()
	if ctx != nil {
		go b.queueInitial(ctx)
	}
}

// Disconnect stops syncing; the TUI calls it right before the program exits.
func (b *Backend) Disconnect() {
	b.hc.Stop()
}

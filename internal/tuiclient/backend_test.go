package tuiclient

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli"
	"github.com/whyyor/tuimessenger/internal/hicli/jsoncmd"
)

// newTestBackend runs a real hicli on an empty database, without logging in anywhere.
func newTestBackend(t *testing.T) *Backend {
	t.Helper()
	db, err := dbutil.NewFromConfig("test", dbutil.Config{
		PoolConfig: dbutil.PoolConfig{
			Type:         "sqlite3-fk-wal",
			URI:          "file:" + filepath.Join(t.TempDir(), "client.db") + "?_txlock=immediate",
			MaxOpenConns: 1,
		},
	}, dbutil.NoopLogger)
	if err != nil {
		t.Fatal(err)
	}
	b := newBackend(zerolog.Nop())
	b.hc = hicli.New(db, nil, zerolog.Nop(), []byte("test"), b.onEvent)
	if err = b.hc.Start(context.Background(), "", nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return b
}

func TestBackendRun(t *testing.T) {
	b := newTestBackend(t)
	// Events from before the TUI connects are covered by the initial data, so they must not be delivered.
	b.onEvent(&jsoncmd.Typing{RoomID: "!old:x"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan any, 10)
	go func() {
		_ = b.Run(ctx, func(_ context.Context, evt any) { got <- evt }, nil)
	}()
	next := func() any {
		select {
		case evt := <-got:
			return evt
		case <-time.After(5 * time.Second):
			t.Fatal("no event from the backend")
			return nil
		}
	}

	// Logged out: client state and sync status, but no rooms (hicli sends those itself after logging in).
	if state, ok := next().(*jsoncmd.ClientState); !ok || state.IsLoggedIn {
		t.Fatalf("first event should be the logged-out client state, got %#v", state)
	}
	if evt := next(); evt == nil {
		t.Fatal("missing sync status")
	} else if _, ok := evt.(*jsoncmd.SyncStatus); !ok {
		t.Fatalf("second event should be the sync status, got %T", evt)
	}

	sent := &jsoncmd.Typing{RoomID: "!new:x", TypingEventContent: event.TypingEventContent{UserIDs: []id.UserID{"@a:x"}}}
	b.onEvent(sent)
	typing, ok := next().(*jsoncmd.Typing)
	if !ok || typing.RoomID != "!new:x" || len(typing.UserIDs) != 1 || typing.UserIDs[0] != "@a:x" {
		t.Fatalf("live event didn't arrive intact: %#v", typing)
	} else if typing == sent {
		t.Error("the TUI got hicli's own object instead of a copy")
	}
}

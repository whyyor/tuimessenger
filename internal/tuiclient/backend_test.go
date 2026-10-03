package tuiclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli"
	"github.com/whyyor/tuimessenger/internal/hicli/database"
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

// fakeBackend answers just enough of the Matrix API for a logged-in hicli, and counts reconnect requests.
func fakeBackend(t *testing.T, reconnects *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/_matrix/client/versions":
			_, _ = w.Write([]byte(`{"versions":["v1.1","v1.11"],"unstable_features":{"com.tuimessenger.no_e2ee":true}}`))
		case r.URL.Path == "/_matrix/client/unstable/com.tuimessenger/reconnect":
			reconnects.Add(1)
			_, _ = w.Write([]byte(`{"reconnecting":["whatsapp"]}`))
		case strings.HasSuffix(r.URL.Path, "/sync"):
			time.Sleep(50 * time.Millisecond)
			_, _ = w.Write([]byte(`{"next_batch":"1"}`))
		case strings.HasSuffix(r.URL.Path, "/filter"):
			_, _ = w.Write([]byte(`{"filter_id":"1"}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestResyncReconnectsOnce(t *testing.T) {
	var reconnects atomic.Int32
	srv := fakeBackend(t, &reconnects)
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
	t.Cleanup(func() { _ = db.Close() })
	b := newBackend(zerolog.Nop())
	b.hc = hicli.New(db, nil, zerolog.Nop(), []byte("test"), b.onEvent)
	account := &database.Account{UserID: "@me:x", DeviceID: "TEST", AccessToken: "token", HomeserverURL: srv.URL}
	if err = b.hc.Start(context.Background(), account.UserID, account); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.hc.Stop)
	if !b.hc.IsLoggedIn() || !b.hc.VerificationState.IsVerified {
		t.Fatal("hicli should be logged in and verified (no E2EE)")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.Run(ctx, func(context.Context, any) {}, nil) }()
	for b.runCtxOrNil() == nil {
		time.Sleep(10 * time.Millisecond)
	}
	for range 10 {
		b.Resync() // a held-down key
	}
	deadline := time.Now().Add(5 * time.Second)
	for !b.resyncing.Load() && reconnects.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	for b.resyncing.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := reconnects.Load(); got != 1 {
		t.Errorf("backend got %d reconnect requests, want 1", got)
	}
}

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
	"go.mau.fi/util/jsontime"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli"
	"github.com/whyyor/tuimessenger/internal/hicli/database"
	"github.com/whyyor/tuimessenger/internal/hicli/jsoncmd"
)

// syncServer is a fake backend whose /sync answers come from syncs, one per request; past the end it waits until
// the test ends. Each func may block (e.g. until the test has prepared the database).
func syncServer(t *testing.T, syncs ...func() string) *httptest.Server {
	t.Helper()
	var n atomic.Int32
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/_matrix/client/versions":
			_, _ = w.Write([]byte(`{"versions":["v1.1","v1.11"],"unstable_features":{"com.tuimessenger.no_e2ee":true}}`))
		case strings.HasSuffix(r.URL.Path, "/sync"):
			i := int(n.Add(1)) - 1
			if i >= len(syncs) {
				select {
				case <-done:
				case <-r.Context().Done():
				}
				return
			}
			_, _ = w.Write([]byte(syncs[i]()))
		case strings.HasSuffix(r.URL.Path, "/filter"):
			_, _ = w.Write([]byte(`{"filter_id":"1"}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(done) }) // runs first: lets waiting /sync requests return so Close doesn't hang
	return srv
}

func body(s string) func() string { return func() string { return s } }

// loggedIn starts a real hicli logged in to srv; every event it emits goes to the returned channel.
func loggedIn(t *testing.T, srv *httptest.Server, onEvent func(any)) *hicli.HiClient {
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
	t.Cleanup(func() { _ = db.Close() })
	hc := hicli.New(db, nil, zerolog.Nop(), []byte("test"), onEvent)
	account := &database.Account{UserID: "@me:x", DeviceID: "TEST", AccessToken: "token", HomeserverURL: srv.URL}
	if err = hc.Start(context.Background(), account.UserID, account); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(hc.Stop)
	return hc
}

// roomSync waits for the sync payload that carries room, and returns that room's part of it.
func roomSync(t *testing.T, events <-chan any, room id.RoomID) *jsoncmd.SyncRoom {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case evt := <-events:
			if sync, ok := evt.(*jsoncmd.SyncComplete); ok && sync.Rooms[room] != nil {
				return sync.Rooms[room]
			}
		case <-deadline:
			t.Fatalf("no sync for %s", room)
			return nil
		}
	}
}

func eventIn(events []*database.Event, evtID id.EventID) *database.Event {
	for _, evt := range events {
		if evt.ID == evtID {
			return evt
		}
	}
	return nil
}

const msgA = `{"type":"m.room.message","event_id":"$a:x","sender":"@peter:x","origin_server_ts":1000,
	"content":{"msgtype":"m.text","body":"hi"}}`
const editA = `{"type":"m.room.message","event_id":"$b:x","sender":"@peter:x","origin_server_ts":2000,
	"content":{"msgtype":"m.text","body":"* hello","m.new_content":{"msgtype":"m.text","body":"hello"},
	"m.relates_to":{"rel_type":"m.replace","event_id":"$a:x"}}}`

func joinSync(batch string, timeline ...string) string {
	return `{"next_batch":"` + batch + `","rooms":{"join":{"!r:x":{"timeline":{"events":[` +
		strings.Join(timeline, ",") + `]}}}}}`
}

// Logged in but before the first sync: the TUI gets no "all chats loaded" yet; hicli sends the rooms itself
// after its first sync (upstream 15ac73be).
func TestNoInitialChatsBeforeFirstSync(t *testing.T) {
	srv := syncServer(t) // never answers /sync
	b := newBackend(zerolog.Nop())
	b.hc = loggedIn(t, srv, b.onEvent)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan any, 50)
	go func() { _ = b.Run(ctx, func(_ context.Context, evt any) { got <- evt }, nil) }()
	deadline := time.After(time.Second)
	for {
		select {
		case evt := <-got:
			if _, ok := evt.(*jsoncmd.InitComplete); ok {
				t.Fatal("the TUI was told all chats are loaded before the first sync")
			}
		case <-deadline:
			return
		}
	}
}

// A message and its edit in the same sync: the TUI gets the message already pointing at its edit. A guard for
// upstream 6c4f7a5a (the TUI gets the newest copy of an event processed twice in one sync); this held before too.
func TestEditInSameSync(t *testing.T) {
	events := make(chan any, 50)
	loggedIn(t, syncServer(t, body(joinSync("2", msgA, editA))), func(evt any) { events <- evt })
	room := roomSync(t, events, "!r:x")
	msg, edit := eventIn(room.Events, "$a:x"), eventIn(room.Events, "$b:x")
	if msg == nil || edit == nil {
		t.Fatalf("both events in the sync: %v %v", msg, edit)
	}
	if msg.LastEditRowID == nil || *msg.LastEditRowID != edit.RowID {
		t.Errorf("the message as sent to the TUI should point at its edit %d, got %v", edit.RowID, msg.LastEditRowID)
	}
}

// A message that arrives with its latest edit bundled in it (as servers do in backfill and some syncs) gets that
// edit stored and linked (upstream 07b3e23e).
func TestBundledEditIsStored(t *testing.T) {
	bundled := `{"type":"m.room.message","event_id":"$a:x","sender":"@peter:x","origin_server_ts":1000,
		"content":{"msgtype":"m.text","body":"hi"},"unsigned":{"m.relations":{"m.replace":` + editA + `}}}`
	events := make(chan any, 50)
	hc := loggedIn(t, syncServer(t, body(joinSync("2", bundled))), func(evt any) { events <- evt })
	roomSync(t, events, "!r:x")
	ctx := context.Background()
	edit, err := hc.DB.Event.GetByID(ctx, "!r:x", "$b:x")
	if err != nil || edit == nil {
		t.Fatalf("the bundled edit should be stored: %v %v", edit, err)
	}
	msg, err := hc.DB.Event.GetByID(ctx, "!r:x", "$a:x")
	if err != nil || msg == nil || msg.LastEditRowID == nil || *msg.LastEditRowID != edit.RowID {
		t.Errorf("the message should point at its bundled edit %d: %+v %v", edit.RowID, msg, err)
	}
}

// Your own message coming back from the server (it has your transaction ID) is processed again instead of being
// skipped: the stored copy takes the server's timestamp and metadata, the content stays (upstream c1e6ecc6).
func TestSentMessageTakesServerVersion(t *testing.T) {
	ready := make(chan struct{})
	echo := `{"type":"m.room.message","event_id":"$s:x","sender":"@me:x","origin_server_ts":3000,
		"content":{"msgtype":"m.text","body":"as the server has it"},"unsigned":{"transaction_id":"txn1"}}`
	events := make(chan any, 50)
	hc := loggedIn(t, syncServer(t,
		body(joinSync("2", msgA)),
		func() string {
			// Gives up if the test failed before preparing the database, so the server can shut down.
			select {
			case <-ready:
			case <-time.After(5 * time.Second):
			}
			return joinSync("3", echo)
		},
	), func(evt any) { events <- evt })
	roomSync(t, events, "!r:x")
	ctx := context.Background()
	// What sending left in the database: the message as we sent it.
	sent := &database.Event{RoomID: "!r:x", ID: "$s:x", Sender: "@me:x", Type: "m.room.message",
		Content: []byte(`{"msgtype":"m.text","body":"as we sent it"}`), Unsigned: []byte(`{}`), TransactionID: "txn1",
		Timestamp: jsontime.UM(time.UnixMilli(2500))}
	if _, err := hc.DB.Event.Upsert(ctx, sent); err != nil {
		t.Fatal(err)
	}
	close(ready)
	roomSync(t, events, "!r:x")
	got, err := hc.DB.Event.GetByID(ctx, "!r:x", "$s:x")
	if err != nil || got == nil {
		t.Fatalf("the sent message should still be there: %v", err)
	}
	if got.Timestamp.UnixMilli() != 3000 || !strings.Contains(string(got.Unsigned), "txn1") {
		t.Errorf("the server's timestamp and metadata should replace ours: %d %s", got.Timestamp.UnixMilli(), got.Unsigned)
	}
}

// A message and a reaction to it in the same sync: the TUI gets the message with the reaction counted. Also a guard
// for upstream 6c4f7a5a; this held before too.
func TestReactionInSameSync(t *testing.T) {
	reaction := `{"type":"m.reaction","event_id":"$r:x","sender":"@lena:x","origin_server_ts":2000,
		"content":{"m.relates_to":{"rel_type":"m.annotation","event_id":"$a:x","key":"👍"}}}`
	events := make(chan any, 50)
	loggedIn(t, syncServer(t, body(joinSync("2", msgA, reaction))), func(evt any) { events <- evt })
	room := roomSync(t, events, "!r:x")
	msg := eventIn(room.Events, "$a:x")
	if msg == nil || msg.Reactions["👍"] != 1 {
		t.Errorf("the message as sent to the TUI should count the 👍: %+v", msg)
	}
}

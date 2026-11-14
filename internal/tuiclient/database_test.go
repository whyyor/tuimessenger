package tuiclient

import (
	"context"
	"testing"
	"time"

	"go.mau.fi/util/jsontime"
	"maunium.net/go/mautrix/event"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
)

// dbWithMessage is a fresh client.db (real hicli, upgraded) with one chat and one message in it.
func dbWithMessage(t *testing.T) (*database.Database, database.EventRowID) {
	t.Helper()
	ctx := context.Background()
	db := newTestBackend(t).hc.DB
	if err := db.Room.CreateRow(ctx, "!r:x"); err != nil {
		t.Fatal(err)
	}
	rowID, err := db.Event.Upsert(ctx, &database.Event{RoomID: "!r:x", ID: "$a:x", Sender: "@peter:x",
		Type: "m.room.message", Content: []byte(`{"msgtype":"m.text","body":"hi"}`), Unsigned: []byte(`{}`),
		Timestamp: jsontime.UM(time.UnixMilli(1000))})
	if err != nil {
		t.Fatal(err)
	}
	return db, rowID
}

// Reaction counts are filled for every message loaded, also one that already carries a (stale, here empty)
// count (upstream 97014b2b).
func TestReactionCountsRefreshed(t *testing.T) {
	db, _ := dbWithMessage(t)
	ctx := context.Background()
	_, err := db.Event.Upsert(ctx, &database.Event{RoomID: "!r:x", ID: "$r:x", Sender: "@lena:x", Type: "m.reaction",
		RelatesTo: "$a:x", RelationType: event.RelAnnotation, Unsigned: []byte(`{}`),
		Content:   []byte(`{"m.relates_to":{"rel_type":"m.annotation","event_id":"$a:x","key":"👍"}}`),
		Timestamp: jsontime.UM(time.UnixMilli(2000))})
	if err != nil {
		t.Fatal(err)
	}
	loaded := &database.Event{RoomID: "!r:x", ID: "$a:x", Reactions: map[string]int{}}
	if err = db.Event.FillReactionCounts(ctx, "!r:x", []*database.Event{loaded}); err != nil {
		t.Fatal(err)
	}
	if loaded.Reactions["👍"] != 1 {
		t.Errorf("the message should count the 👍, got %v", loaded.Reactions)
	}
}

// Loading older messages into a chat works when the timeline table is still empty (upstream 2a3c9c27).
func TestPrependOnEmptyTimeline(t *testing.T) {
	db, rowID := dbWithMessage(t)
	if _, err := db.Timeline.Prepend(context.Background(), "!r:x", []database.EventRowID{rowID}); err != nil {
		t.Errorf("prepending to an empty timeline failed: %v", err)
	}
}

package tui

import (
	"testing"
	"time"

	"go.mau.fi/util/jsontime"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
	"go.mau.fi/gomuks/pkg/rpc/client"
	"go.mau.fi/gomuks/pkg/rpc/store"
)

func TestSeenMarkerCovers(t *testing.T) {
	at := time.Date(2026, 10, 22, 15, 24, 23, 0, time.Local)
	preview := &database.Event{ID: "$new", Timestamp: jsontime.UM(at)}
	for _, c := range []struct {
		m    seenMarker
		want bool
	}{
		{seenMarker{}, false},
		{seenMarker{EventID: "$new"}, true},
		{seenMarker{EventID: "$old", Timestamp: at.Add(-time.Minute).UnixMilli()}, false},
		{seenMarker{EventID: "$later", Timestamp: at.Add(time.Second).UnixMilli()}, true},
	} {
		if got := c.m.coveredBy(preview); got != c.want {
			t.Errorf("%+v: got %v", c.m, got)
		}
	}
	if (seenMarker{EventID: "$new"}).coveredBy(nil) {
		t.Error("no preview: nothing to cover")
	}
}

func TestApplyLocalSeen(t *testing.T) {
	gs := store.NewStore()
	gmx := &client.GomuksClient{GomuksStore: gs}
	const roomID = "!r:beeper.local"
	sync := func(ad map[event.Type]*database.AccountData) {
		gs.ApplySync(&jsoncmd.SyncComplete{Rooms: map[id.RoomID]*jsoncmd.SyncRoom{
			roomID: {Meta: &database.Room{ID: roomID}, AccountData: ad},
		}})
	}
	sync(nil)
	entry := &store.RoomListEntry{
		RoomID:       roomID,
		PreviewEvent: &database.Event{ID: "$new", Timestamp: jsontime.UM(time.Now())},
		UnreadCounts: database.UnreadCounts{UnreadMessages: 3, UnreadNotifications: 3},
	}
	rooms := []*store.RoomListEntry{entry}
	if got := applyLocalSeen(gmx, rooms)[0]; got != entry {
		t.Fatal("no marker: entry passes through untouched")
	}
	if got := applyLocalSeen(nil, rooms)[0]; got != entry {
		t.Fatal("no client: entry passes through untouched")
	}
	// Read up to the newest message in gomuks: counts cleared, on a copy.
	sync(map[event.Type]*database.AccountData{
		AccountDataSeen: {Type: AccountDataSeen.Type, Content: []byte(`{"event_id":"$new","timestamp":1}`)},
	})
	got := applyLocalSeen(gmx, rooms)[0]
	if got == entry || !got.UnreadCounts.IsZero() {
		t.Errorf("read in gomuks: %+v, want a zeroed copy", got.UnreadCounts)
	}
	if entry.UnreadMessages != 3 || rooms[0] != entry {
		t.Error("the shared entry and slice must not be modified")
	}
	if len(unreadRooms([]*store.RoomListEntry{got}, nil)) != 0 {
		t.Error("the hint screen should no longer list it")
	}
	// Marked unread always wins.
	entry.MarkedUnread = true
	if c := unreadCounts(gmx, entry); c.UnreadMessages != 3 {
		t.Errorf("marked unread keeps its counts: %+v", c)
	}
}

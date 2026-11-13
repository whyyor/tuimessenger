package client

import (
	"context"
	"testing"

	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
	"github.com/whyyor/tuimessenger/internal/hicli/jsoncmd"
	"github.com/whyyor/tuimessenger/internal/rpc/store"
)

// A sync without room metadata (nothing about the room changed but a message came in) still notifies, and a
// notification for a room we don't know is skipped instead of crashing (upstream ea47adb9).
func TestNotificationWithoutRoomMeta(t *testing.T) {
	gc := New(nil)
	roomID := id.RoomID("!r:x")
	gc.Store.ApplySync(&jsoncmd.SyncComplete{Rooms: map[id.RoomID]*jsoncmd.SyncRoom{
		roomID: {Meta: &database.Room{ID: roomID}},
	}})
	var got []database.EventRowID
	gc.SendNotification = func(_ *store.RoomStore, n jsoncmd.SyncNotification) { got = append(got, n.RowID) }
	msg := &database.Event{RowID: 7, ID: "$m:x", RoomID: roomID, Type: "m.room.message"}
	gc.handleEvent(context.Background(), &jsoncmd.SyncComplete{Rooms: map[id.RoomID]*jsoncmd.SyncRoom{
		roomID:       {Events: []*database.Event{msg}, Notifications: []jsoncmd.SyncNotification{{RowID: 7}}},
		"!unknown:x": {Notifications: []jsoncmd.SyncNotification{{RowID: 8}}},
	}})
	if len(got) != 1 || got[0] != 7 {
		t.Errorf("one notification, for the known room: %v", got)
	}
}

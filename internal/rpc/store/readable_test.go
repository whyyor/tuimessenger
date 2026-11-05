package store

import (
	"testing"

	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
)

func TestIsReadableEvent(t *testing.T) {
	for typ, want := range map[string]bool{
		"m.room.message": true, "m.sticker": true, "m.room.encrypted": true,
		"com.beeper.message_send_status": false, "m.reaction": false,
	} {
		if got := isReadableEvent(&database.Event{Type: typ}); got != want {
			t.Errorf("%s: got %v", typ, got)
		}
	}
}

func TestForgetMarkedReadResends(t *testing.T) {
	gs := &GomuksStore{}
	room := NewRoomStore(gs, &database.Room{ID: "!r:x"})
	room.ApplySync(&jsoncmd.SyncRoom{
		Events:   []*database.Event{{RowID: 1, ID: "$m:x", RoomID: "!r:x", Sender: "@a:x", Type: "m.room.message"}},
		Timeline: []database.TimelineRowTuple{{Timeline: 1, Event: 1}},
	})
	if room.GetMarkAsReadParams() == nil {
		t.Fatal("first read should be sent")
	}
	if room.GetMarkAsReadParams() != nil {
		t.Fatal("same message again: skipped")
	}
	room.ForgetMarkedRead()
	if req := room.GetMarkAsReadParams(); req == nil || req.EventID != "$m:x" {
		t.Errorf("after ForgetMarkedRead the read is sent again, got %+v", req)
	}
}

// A receipt synced before its event is in memory is dropped; fetching
// receipts when the chat opens brings it back.
func TestFetchedReceiptsFillTheGap(t *testing.T) {
	room := NewRoomStore(&GomuksStore{}, &database.Room{ID: "!r:x"})
	her := id.UserID("@whatsapp_lid-1:beeper.local")
	receipt := map[id.EventID][]*database.Receipt{"$krlo:x": {{UserID: her, EventID: "$krlo:x"}}}
	room.ApplySync(&jsoncmd.SyncRoom{Receipts: receipt})
	room.ApplySync(&jsoncmd.SyncRoom{
		Events: []*database.Event{
			{RowID: 1, TimelineRowID: 1, ID: "$understood:x", Type: "m.room.message"},
			{RowID: 2, TimelineRowID: 2, ID: "$krlo:x", Type: "m.room.message"},
		},
		Timeline: []database.TimelineRowTuple{{Timeline: 1, Event: 1}, {Timeline: 2, Event: 2}},
	})
	if users := room.ReceiptUsersAt("$krlo:x"); len(users) != 0 {
		t.Fatalf("precondition: the early receipt was dropped, got %v", users)
	}
	if ids := room.LoadedEventIDs(10); len(ids) != 2 || ids[0] != "$understood:x" || ids[1] != "$krlo:x" {
		t.Fatalf("loaded IDs oldest first: %v", ids)
	}
	if ids := room.LoadedEventIDs(1); len(ids) != 1 || ids[0] != "$krlo:x" {
		t.Fatalf("limit keeps the newest: %v", ids)
	}
	room.ApplyFetchedReceipts(receipt)
	if users := room.ReceiptUsersAt("$krlo:x"); len(users) != 1 || users[0] != her {
		t.Errorf("after fetching: %v, want her receipt on Krlo", users)
	}
}

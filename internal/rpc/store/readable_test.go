package store

import (
	"testing"

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

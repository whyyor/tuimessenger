package store

import (
	"testing"

	"go.mau.fi/gomuks/pkg/hicli/database"
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

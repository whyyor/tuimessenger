package store

import (
	"testing"

	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/database"
)

func newReceiptTestRoom() *RoomStore {
	rs := NewRoomStore(NewStore(), &database.Room{ID: "!room"})
	rs.eventsByID["$1"] = &database.Event{ID: "$1", TimelineRowID: 1}
	rs.eventsByID["$2"] = &database.Event{ID: "$2", TimelineRowID: 2}
	return rs
}

func receiptFor(user id.UserID) []*database.Receipt {
	return []*database.Receipt{{UserID: user}}
}

func TestReceiptsFollowLatestPosition(t *testing.T) {
	rs := newReceiptTestRoom()

	if !rs.applyReceiptsLocked(map[id.EventID][]*database.Receipt{"$1": receiptFor("@alice:x")}) {
		t.Fatal("first receipt should report a change")
	}
	if got := rs.ReceiptUsersAt("$1"); len(got) != 1 || got[0] != "@alice:x" {
		t.Fatalf("expected alice at $1, got %v", got)
	}

	// Reading further moves the user off the old cluster.
	rs.applyReceiptsLocked(map[id.EventID][]*database.Receipt{"$2": receiptFor("@alice:x")})
	if got := rs.ReceiptUsersAt("$1"); len(got) != 0 {
		t.Fatalf("alice should have left $1, got %v", got)
	}
	if got := rs.ReceiptUsersAt("$2"); len(got) != 1 || got[0] != "@alice:x" {
		t.Fatalf("expected alice at $2, got %v", got)
	}

	// A stale receipt pointing backwards must not move the position.
	if rs.applyReceiptsLocked(map[id.EventID][]*database.Receipt{"$1": receiptFor("@alice:x")}) {
		t.Fatal("stale receipt should not report a change")
	}
	if got := rs.ReceiptUsersAt("$2"); len(got) != 1 {
		t.Fatalf("alice should still be at $2, got %v", got)
	}
}

func TestReceiptsForUnknownEventsAreDropped(t *testing.T) {
	rs := newReceiptTestRoom()
	if rs.applyReceiptsLocked(map[id.EventID][]*database.Receipt{"$unknown": receiptFor("@bob:x")}) {
		t.Fatal("receipt for an unloaded event should be a no-op")
	}
	if got := rs.ReceiptUsersAt("$unknown"); len(got) != 0 {
		t.Fatalf("expected no cluster, got %v", got)
	}
}

func TestReceiptClustersAccumulateUsers(t *testing.T) {
	rs := newReceiptTestRoom()
	rs.applyReceiptsLocked(map[id.EventID][]*database.Receipt{"$2": {{UserID: "@a:x"}, {UserID: "@b:x"}}})
	if got := rs.ReceiptUsersAt("$2"); len(got) != 2 {
		t.Fatalf("expected two readers at $2, got %v", got)
	}
}

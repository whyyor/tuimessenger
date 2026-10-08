package tui

import (
	"testing"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/rpc/store"
)

func names(rows []*store.RoomListEntry) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		if r == nil {
			out[i] = "<archive>"
		} else {
			out[i] = string(r.RoomID)
		}
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBuildRoomRows(t *testing.T) {
	rooms := []*store.RoomListEntry{
		{RoomID: "a"}, {RoomID: "junk1", LowPriority: true}, {RoomID: "b"}, {RoomID: "junk2", LowPriority: true},
	}
	cases := []struct {
		name     string
		expanded bool
		selected id.RoomID
		want     []string
	}{
		{"collapsed", false, "a", []string{"a", "b", "<archive>"}},
		{"expanded keeps order", true, "a", []string{"a", "b", "<archive>", "junk1", "junk2"}},
		{"collapsed still shows the open chat", false, "junk2", []string{"a", "b", "<archive>", "junk2"}},
	}
	for _, c := range cases {
		rows, archived := buildRoomRows(rooms, c.expanded, c.selected)
		if got := names(rows); !eq(got, c.want) || archived != 2 {
			t.Errorf("%s: got %v (archived=%d), want %v", c.name, got, archived, c.want)
		}
	}
	// No archived chats: no header row at all.
	if rows, n := buildRoomRows(rooms[:1], false, ""); len(rows) != 1 || n != 0 {
		t.Errorf("without archived chats: got %v, %d", names(rows), n)
	}
}

func TestWithLowPriorityKeepsOtherTags(t *testing.T) {
	tags := event.Tags{event.RoomTagFavourite: {Order: "0.1"}}
	got, changed := withLowPriority(tags, true)
	if !changed || len(got) != 2 {
		t.Fatalf("archive: got %v changed=%v", got, changed)
	}
	if _, ok := tags[event.RoomTagLowPriority]; ok {
		t.Error("mutated the input map")
	}
	got, changed = withLowPriority(got, false)
	if _, fav := got[event.RoomTagFavourite]; !changed || len(got) != 1 || !fav {
		t.Errorf("unarchive: got %v changed=%v", got, changed)
	}
	if _, changed = withLowPriority(nil, false); changed {
		t.Error("unarchiving an untagged chat should be a no-op")
	}
}

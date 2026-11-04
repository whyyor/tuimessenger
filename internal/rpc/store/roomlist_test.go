package store

import (
	"testing"
	"time"

	"maunium.net/go/mautrix/id"
)

func TestInsertSorted(t *testing.T) {
	at := func(id id.RoomID, sec int64) *RoomListEntry {
		return &RoomListEntry{RoomID: id, SortingTimestamp: time.Unix(sec, 0)}
	}
	order := func(list []*RoomListEntry) (ids string) {
		for _, e := range list {
			ids += string(e.RoomID)
		}
		return
	}
	cases := []struct {
		name string
		list []*RoomListEntry
		add  *RoomListEntry
		want string
	}{
		{"empty", nil, at("x", 5), "x"},
		{"newest", []*RoomListEntry{at("a", 1), at("b", 2)}, at("x", 3), "abx"},
		{"oldest", []*RoomListEntry{at("a", 1), at("b", 2)}, at("x", 0), "xab"},
		{"middle", []*RoomListEntry{at("a", 1), at("b", 3)}, at("x", 2), "axb"},
		// This used to panic: same time as the first entry, and everything else is newer.
		{"same time as the first", []*RoomListEntry{at("a", 1), at("b", 2)}, at("x", 1), "axb"},
		{"same time as the last", []*RoomListEntry{at("a", 1), at("b", 2)}, at("x", 2), "abx"},
		{"all the same time", []*RoomListEntry{at("a", 1), at("b", 1)}, at("x", 1), "abx"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := order(insertSorted(c.list, c.add)); got != c.want {
				t.Errorf("got %s, want %s", got, c.want)
			}
		})
	}
}

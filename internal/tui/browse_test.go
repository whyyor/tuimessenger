package tui

import (
	"testing"

	"go.mau.fi/gomuks/pkg/rpc/store"
)

func TestBrowseStep(t *testing.T) {
	// rooms is newest first, as drawn.
	list := &RoomList{rooms: []*store.RoomListEntry{{RoomID: "newest"}, {RoomID: "mid"}, {RoomID: "oldest"}}}
	steps := []struct {
		down bool
		want string
	}{
		{true, "newest"}, // first ↓ with nothing highlighted: top of the list
		{true, "mid"},
		{true, "oldest"},
		{true, "oldest"}, // stays at the bottom
		{false, "mid"},
		{false, "newest"},
		{false, "newest"}, // stays at the top
	}
	for i, s := range steps {
		list.selected = list.BrowseStep(s.down)
		if string(list.selected) != s.want {
			t.Fatalf("step %d (down=%v): got %q, want %q", i, s.down, list.selected, s.want)
		}
	}
	list.selected = ""
	if got := list.BrowseStep(false); got != "newest" {
		t.Errorf("first ↑ with nothing highlighted: got %q, want newest", got)
	}
}

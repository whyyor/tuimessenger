package tui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/database"
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

func TestHintRoomForKey(t *testing.T) {
	rows := []hintRow{{roomID: "a"}, {roomID: "b"}, {roomID: "c"}}
	cases := []struct {
		ev   *tcell.EventKey
		want id.RoomID
	}{
		{tcell.NewEventKey(tcell.KeyRune, '1', tcell.ModNone), "a"},
		{tcell.NewEventKey(tcell.KeyRune, '3', tcell.ModNone), "c"},
		{tcell.NewEventKey(tcell.KeyRune, '4', tcell.ModNone), ""}, // past the list
		{tcell.NewEventKey(tcell.KeyRune, '0', tcell.ModNone), ""},
		{tcell.NewEventKey(tcell.KeyRune, 'x', tcell.ModNone), ""},
		{tcell.NewEventKey(tcell.KeyRune, '1', tcell.ModAlt), ""},
	}
	for _, c := range cases {
		if got := hintRoomForKey(rows, c.ev); got != c.want {
			t.Errorf("rune %q mod %v: got %q, want %q", c.ev.Rune(), c.ev.Modifiers(), got, c.want)
		}
	}
}

func TestUnreadRooms(t *testing.T) {
	rooms := []*store.RoomListEntry{
		{RoomID: "read"},
		{RoomID: "unread", UnreadCounts: database.UnreadCounts{UnreadMessages: 3}},
		{RoomID: "muted", UnreadCounts: database.UnreadCounts{UnreadMessages: 5}},
		{RoomID: "marked", MarkedUnread: true},
		{RoomID: "mention", UnreadCounts: database.UnreadCounts{UnreadHighlights: 1}},
	}
	got := unreadRooms(rooms, map[id.RoomID]bool{"muted": true})
	var ids []string
	for _, r := range got {
		ids = append(ids, string(r.RoomID))
	}
	if strings.Join(ids, ",") != "unread,marked,mention" {
		t.Errorf("got %v, want unread, marked, mention in order", ids)
	}
}

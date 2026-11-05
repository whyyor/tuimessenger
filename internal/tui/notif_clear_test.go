package tui

import (
	"slices"
	"testing"

	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/pkg/rpc/store"
)

func TestNotificationTracking(t *testing.T) {
	trackNotification("!a", "whatsapp", notificationTag("!a", "$1"))
	trackNotification("!a", "whatsapp", notificationTag("!a", "$2"))
	trackNotification("!b", "telegram", notificationTag("!b", "$3"))
	trackNotification("!c", "slack", notificationTag("!c", "$4"))
	defer takeNotifications("!a", "!b", "!c")

	rooms := []*store.RoomListEntry{
		{RoomID: "!a"}, // read on another device
		{RoomID: "!b", UnreadCounts: database.UnreadCounts{UnreadMessages: 2}},
		{RoomID: "!c", MarkedUnread: true},
		{RoomID: "!untracked"},
	}
	if got := readTrackedRooms(rooms); !slices.Equal(got, []id.RoomID{"!a"}) {
		t.Fatalf("only the read, tracked chat should clear: %v", got)
	}
	got := takeNotifications("!a")
	if len(got) != 2 || got[0].tag != "gomuks:!a:$1" || got[0].source != "whatsapp" {
		t.Errorf("took %+v", got)
	}
	if again := takeNotifications("!a"); len(again) != 0 {
		t.Error("taking twice must not remove twice")
	}
	if left := takeNotifications("!b"); len(left) != 1 {
		t.Errorf("unread chat keeps its banner: %+v", left)
	}
}

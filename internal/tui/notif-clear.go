package tui

import (
	"sync"

	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/rpc/store"
	"go.mau.fi/gomuks/tui/debug"
	"go.mau.fi/gomuks/tui/lib/notification"
)

type sentNotification struct {
	source, tag string
}

// shownNotifications are this session's banners per chat, cleared once read.
var shownNotifications = struct {
	sync.Mutex
	byRoom map[id.RoomID][]sentNotification
}{byRoom: make(map[id.RoomID][]sentNotification)}

func notificationTag(roomID id.RoomID, eventID id.EventID) string {
	return "gomuks:" + string(roomID) + ":" + string(eventID)
}

func trackNotification(roomID id.RoomID, source, tag string) {
	shownNotifications.Lock()
	defer shownNotifications.Unlock()
	shownNotifications.byRoom[roomID] = append(shownNotifications.byRoom[roomID], sentNotification{source, tag})
}

// takeNotifications removes and returns the tracked banners of the rooms.
func takeNotifications(roomIDs ...id.RoomID) []sentNotification {
	shownNotifications.Lock()
	defer shownNotifications.Unlock()
	var out []sentNotification
	for _, roomID := range roomIDs {
		out = append(out, shownNotifications.byRoom[roomID]...)
		delete(shownNotifications.byRoom, roomID)
	}
	return out
}

func removeNotifications(list []sentNotification) {
	if len(list) == 0 {
		return
	}
	go func() {
		defer debug.Recover()
		for _, n := range list {
			if err := notification.Remove(n.source, n.tag); err != nil {
				debug.Print("Failed to remove notification", n.tag, err)
			}
		}
	}()
}

// ClearNotifications takes back the chat's banners from Notification Center.
func ClearNotifications(roomID id.RoomID) {
	removeNotifications(takeNotifications(roomID))
}

func isRead(room *store.RoomListEntry) bool {
	return room.UnreadMessages == 0 && room.UnreadNotifications == 0 &&
		room.UnreadHighlights == 0 && !room.MarkedUnread
}

// readTrackedRooms returns tracked rooms that are now fully read.
func readTrackedRooms(rooms []*store.RoomListEntry) []id.RoomID {
	shownNotifications.Lock()
	defer shownNotifications.Unlock()
	if len(shownNotifications.byRoom) == 0 {
		return nil
	}
	var out []id.RoomID
	for _, room := range rooms {
		if _, tracked := shownNotifications.byRoom[room.RoomID]; tracked && isRead(room) {
			out = append(out, room.RoomID)
		}
	}
	return out
}

// clearReadNotifications runs on room list updates: a chat read on any
// device (phone, Beeper Desktop) loses its banners here too.
func clearReadNotifications(rooms []*store.RoomListEntry) {
	removeNotifications(takeNotifications(readTrackedRooms(rooms)...))
}

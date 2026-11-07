package tui

import (
	"context"
	"encoding/json"
	"slices"
	"sync"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
	"github.com/whyyor/tuimessenger/internal/hicli/jsoncmd"
	"github.com/whyyor/tuimessenger/internal/rpc/client"
	"github.com/whyyor/tuimessenger/internal/rpc/store"
	"github.com/whyyor/tuimessenger/internal/tui/debug"
)

// AccountDataSeen is how far you've read a /seen-off chat in tuimessenger. Those
// chats send no receipt at all until Ctrl+m: Beeper only bridges a read that
// moves your read position, so a private receipt first would swallow it.
var AccountDataSeen = event.Type{Type: "com.whyyor.tuimessenger.seen", Class: event.AccountDataEventType}

type seenMarker struct {
	EventID   id.EventID `json:"event_id"`
	Timestamp int64      `json:"timestamp"`
}

// seenCache parses each room's marker once per account data update.
var seenCache = struct {
	sync.Mutex
	byAD map[*database.AccountData]seenMarker
}{byAD: make(map[*database.AccountData]seenMarker)}

func seenMarkerOf(room *store.RoomStore) seenMarker {
	if room == nil {
		return seenMarker{}
	}
	ad := room.GetAccountData(AccountDataSeen)
	if ad == nil {
		return seenMarker{}
	}
	seenCache.Lock()
	defer seenCache.Unlock()
	if m, ok := seenCache.byAD[ad]; ok {
		return m
	}
	var m seenMarker
	_ = json.Unmarshal(ad.Content, &m)
	seenCache.byAD[ad] = m
	return m
}

// coveredBy reports whether the marker has reached the chat's newest event.
func (m seenMarker) coveredBy(preview *database.Event) bool {
	if preview == nil || m.EventID == "" {
		return false
	}
	return m.EventID == preview.ID || m.Timestamp >= preview.Timestamp.UnixMilli()
}

// unreadCounts is what tuimessenger shows: the server's counts, zeroed for a chat
// you've read here without sending a receipt (unless marked unread).
func unreadCounts(gmx *client.Client, entry *store.RoomListEntry) database.UnreadCounts {
	counts := entry.UnreadCounts
	if entry.MarkedUnread || counts.IsZero() || gmx == nil {
		return counts
	}
	if seenMarkerOf(gmx.GetRoom(entry.RoomID)).coveredBy(entry.PreviewEvent) {
		return database.UnreadCounts{}
	}
	return counts
}

// applyLocalSeen returns rooms with unread counts cleared for chats read in
// tuimessenger; changed entries are copies, the rest are shared.
func applyLocalSeen(gmx *client.Client, rooms []*store.RoomListEntry) []*store.RoomListEntry {
	out := rooms
	copied := false
	for i, entry := range rooms {
		counts := unreadCounts(gmx, entry)
		if counts == entry.UnreadCounts {
			continue
		}
		if !copied {
			out = slices.Clone(rooms)
			copied = true
		}
		adjusted := *entry
		adjusted.UnreadCounts = counts
		out[i] = &adjusted
	}
	return out
}

// markSeenLocally records the read in tuimessenger only.
func (view *MainView) markSeenLocally(room *store.RoomStore, req *jsoncmd.MarkReadParams) {
	defer debug.Recover()
	if saveLocalSeen(view.matrix, room, req) {
		view.parent.Render()
	}
}

// accountDataSetter is the one command saveLocalSeen needs.
type accountDataSetter interface {
	SetAccountData(ctx context.Context, params *jsoncmd.SetAccountDataParams) error
}

// saveLocalSeen writes the seen marker, which stays in tuimessenger. It must never clear the chat's marked-unread flag:
// bridges pass that to the network, and WhatsApp treats "no longer unread" as reading the chat.
func saveLocalSeen(api accountDataSetter, room *store.RoomStore, req *jsoncmd.MarkReadParams) (changed bool) {
	marker := seenMarker{EventID: req.EventID}
	if evt := room.GetEventByID(req.EventID); evt != nil {
		marker.Timestamp = evt.Timestamp.UnixMilli()
	}
	if seenMarkerOf(room) == marker {
		return false
	}
	raw, _ := json.Marshal(&marker)
	err := api.SetAccountData(context.TODO(), &jsoncmd.SetAccountDataParams{
		RoomID: room.ID, Type: AccountDataSeen.Type, Content: raw,
	})
	if err != nil {
		debug.Print("Failed to save local read for", room.ID, err)
	}
	return true
}

// onRoomListChange runs inside the store's ApplySync, which holds its lock:
// anything that looks rooms up (GetRoom) must happen after it returns.
func onRoomListChange(gmx *client.Client, rooms []*store.RoomListEntry) {
	// A closure: `go f(g())` would still evaluate g() right here, under the lock.
	go func() { clearReadNotifications(applyLocalSeen(gmx, rooms)) }()
}

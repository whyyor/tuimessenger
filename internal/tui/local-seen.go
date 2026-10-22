package tui

import (
	"context"
	"encoding/json"
	"slices"
	"sync"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
	"go.mau.fi/gomuks/pkg/rpc/client"
	"go.mau.fi/gomuks/pkg/rpc/store"
	"go.mau.fi/gomuks/tui/debug"
)

// AccountDataSeen is how far you've read a /seen-off chat in gomuks. Those
// chats send no receipt at all until Ctrl+m: Beeper only bridges a read that
// moves your read position, so a private receipt first would swallow it.
var AccountDataSeen = event.Type{Type: "com.whyyor.gomuks.seen", Class: event.AccountDataEventType}

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

// unreadCounts is what gomuks shows: the server's counts, zeroed for a chat
// you've read here without sending a receipt (unless marked unread).
func unreadCounts(gmx *client.GomuksClient, entry *store.RoomListEntry) database.UnreadCounts {
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
// gomuks; changed entries are copies, the rest are shared.
func applyLocalSeen(gmx *client.GomuksClient, rooms []*store.RoomListEntry) []*store.RoomListEntry {
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

// markSeenLocally records the read in gomuks only, and clears a marked-unread
// flag as opening the chat would.
func (view *MainView) markSeenLocally(room *store.RoomStore, req *jsoncmd.MarkReadParams) {
	defer debug.Recover()
	marker := seenMarker{EventID: req.EventID}
	if evt := room.GetEventByID(req.EventID); evt != nil {
		marker.Timestamp = evt.Timestamp.UnixMilli()
	}
	if seenMarkerOf(room) == marker {
		return
	}
	raw, _ := json.Marshal(&marker)
	err := view.matrix.SetAccountData(context.TODO(), &jsoncmd.SetAccountDataParams{
		RoomID: room.ID, Type: AccountDataSeen.Type, Content: raw,
	})
	if err != nil {
		debug.Print("Failed to save local read for", room.ID, err)
	}
	if meta := room.Meta.Current(); meta != nil && meta.MarkedUnread != nil && *meta.MarkedUnread {
		raw, _ = json.Marshal(&event.MarkedUnreadEventContent{Unread: false})
		_ = view.matrix.SetAccountData(context.TODO(), &jsoncmd.SetAccountDataParams{
			RoomID: room.ID, Type: event.AccountDataMarkedUnread.Type, Content: raw,
		})
	}
	view.parent.Render()
}

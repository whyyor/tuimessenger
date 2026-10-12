package tui

import (
	"context"
	"encoding/json"
	"time"

	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
	"go.mau.fi/gomuks/pkg/rpc/store"
	"go.mau.fi/gomuks/tui/debug"
)

// clearedAt is when history was last cleared in this room, or zero.
func clearedAt(room *store.RoomStore) time.Time {
	return room.ClearState().At()
}

// hiddenByClear reports whether an event predates the room's clear point.
func hiddenByClear(evt *database.Event, cleared time.Time) bool {
	return !cleared.IsZero() && evt.Timestamp.Time.Before(cleared)
}

// historyExhausted reports that the oldest loaded event is already hidden,
// so paginating further would only fetch more hidden history.
func historyExhausted(timeline []*database.Event, cleared time.Time) bool {
	return len(timeline) > 0 && hiddenByClear(timeline[0], cleared)
}

// listedRooms drops chats with nothing visible since their /clear point; a
// new message moves SortingTimestamp past it and brings the chat back. The
// open chat stays listed so it doesn't vanish while you're in it.
func listedRooms(rooms []*store.RoomListEntry, open id.RoomID) []*store.RoomListEntry {
	out := rooms[:0:0]
	for _, room := range rooms {
		if room.ClearedAt.IsZero() || room.SortingTimestamp.After(room.ClearedAt) || room.RoomID == open {
			out = append(out, room)
		}
	}
	return out
}

func (view *RoomView) writeClearState(state store.ClearState) error {
	raw, _ := json.Marshal(state)
	return view.parent.matrix.SetAccountData(context.TODO(), &jsoncmd.SetAccountDataParams{
		RoomID:  view.Room.ID,
		Type:    store.AccountDataCleared.Type,
		Content: raw,
	})
}

// SetCleared hides this chat's history in gomuks only, keeping individually
// hidden messages; unclearing resets both. Nothing is deleted anywhere.
func (view *RoomView) SetCleared(clear bool) {
	defer debug.Recover()
	state := store.ClearState{}
	if clear {
		state = view.Room.ClearState()
		state.TS = time.Now().UnixMilli()
	}
	if err := view.writeClearState(state); err != nil {
		view.AddServiceMessage("Failed to update cleared history: %v", err)
	} else if !clear {
		go view.parent.LoadHistory(view.Room.ID)
	}
	view.parent.parent.Render()
}

// HideMessage hides one message in gomuks until /unclear.
func (view *RoomView) HideMessage(evtID id.EventID) {
	defer debug.Recover()
	state, changed := view.Room.ClearState().WithHidden(evtID)
	if !changed {
		return
	}
	if err := view.writeClearState(state); err != nil {
		view.AddServiceMessage("Failed to hide message: %v", err)
	}
	view.parent.parent.Render()
}

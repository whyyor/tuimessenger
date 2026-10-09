package tui

import (
	"context"
	"encoding/json"
	"time"

	"maunium.net/go/mautrix/event"

	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
	"go.mau.fi/gomuks/pkg/rpc/store"
	"go.mau.fi/gomuks/tui/debug"
)

// accountDataCleared is private room account data: only this client reads
// it, so Beeper and the bridges never act on it and nothing is deleted.
var accountDataCleared = event.Type{Type: "com.whyyor.gomuks.cleared", Class: event.AccountDataEventType}

type clearedContent struct {
	TS int64 `json:"ts,omitempty"`
}

// clearedAt is when history was last cleared in this room, or zero.
func clearedAt(room *store.RoomStore) time.Time {
	ad := room.GetAccountData(accountDataCleared)
	if ad == nil {
		return time.Time{}
	}
	var content clearedContent
	if json.Unmarshal(ad.Content, &content) != nil || content.TS == 0 {
		return time.Time{}
	}
	return time.UnixMilli(content.TS)
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

// SetCleared hides (or shows again) this chat's history in gomuks only.
// Account data can't be deleted, so unclearing writes an empty object.
func (view *RoomView) SetCleared(clear bool) {
	defer debug.Recover()
	content := clearedContent{}
	if clear {
		content.TS = time.Now().UnixMilli()
	}
	raw, _ := json.Marshal(content)
	err := view.parent.matrix.SetAccountData(context.TODO(), &jsoncmd.SetAccountDataParams{
		RoomID:  view.Room.ID,
		Type:    accountDataCleared.Type,
		Content: raw,
	})
	switch {
	case err != nil:
		view.AddServiceMessage("Failed to update cleared history: %v", err)
	case clear:
		view.AddServiceMessage("History hidden in gomuks only; nothing was deleted on WhatsApp or Beeper. /unclear shows it again.")
	default:
		view.AddServiceMessage("History shown again.")
		go view.parent.LoadHistory(view.Room.ID)
	}
	view.parent.parent.Render()
}

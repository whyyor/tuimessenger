package store

import (
	"encoding/json"
	"slices"
	"time"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// AccountDataCleared is private room account data written by the terminal
// client's /clear and hide: only gomuks reads it, so Beeper and the bridges
// never act on it and nothing is deleted anywhere.
var AccountDataCleared = event.Type{Type: "com.whyyor.gomuks.cleared", Class: event.AccountDataEventType}

// ClearState hides history older than TS and the listed events.
type ClearState struct {
	TS     int64        `json:"ts,omitempty"`
	Hidden []id.EventID `json:"hidden,omitempty"`
}

// At is the clear point, or zero if the room was never cleared.
func (cs ClearState) At() time.Time {
	if cs.TS == 0 {
		return time.Time{}
	}
	return time.UnixMilli(cs.TS)
}

// WithHidden returns a copy with evtID hidden, and whether that changed anything.
func (cs ClearState) WithHidden(evtID id.EventID) (ClearState, bool) {
	if evtID == "" || slices.Contains(cs.Hidden, evtID) {
		return cs, false
	}
	cs.Hidden = append(slices.Clone(cs.Hidden), evtID)
	return cs, true
}

func (cs ClearState) IsHidden(evtID id.EventID) bool {
	return evtID != "" && slices.Contains(cs.Hidden, evtID)
}

func (rs *RoomStore) ClearState() (cs ClearState) {
	if ad := rs.GetAccountData(AccountDataCleared); ad != nil {
		_ = json.Unmarshal(ad.Content, &cs)
	}
	return
}

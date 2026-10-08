package tui

import (
	"context"
	"encoding/json"

	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
	"go.mau.fi/gomuks/tui/debug"
)

// SetMuted mutes or unmutes the chat with a server-side push rule, the same
// one Beeper and gomuks web write, so every device follows it.
func (view *RoomView) SetMuted(muted bool) {
	defer debug.Recover()
	name := view.Room.ID.String()
	if n := view.Room.Meta.Current().Name; n != nil && *n != "" {
		name = *n
	}
	_, err := view.parent.matrix.MuteRoom(context.TODO(), &jsoncmd.MuteRoomParams{RoomID: view.Room.ID, Muted: muted})
	switch {
	case err != nil:
		view.AddServiceMessage("Failed to change mute for %s: %v", name, err)
	case muted:
		view.AddServiceMessage("Muted: %s", name)
	default:
		view.AddServiceMessage("Unmuted: %s", name)
	}
	view.parent.parent.Render()
}

// mutedRooms reads m.push_rules content into the set of muted chats: an
// enabled room rule (rule_id is the room ID) with no "notify" action. That's
// what MuteRoom writes, and how gomuks web decides it too.
func mutedRooms(content json.RawMessage) map[id.RoomID]bool {
	var rules struct {
		Global struct {
			Room []struct {
				RuleID  id.RoomID         `json:"rule_id"`
				Enabled bool              `json:"enabled"`
				Actions []json.RawMessage `json:"actions"`
			} `json:"room"`
		} `json:"global"`
	}
	if json.Unmarshal(content, &rules) != nil {
		return nil
	}
	muted := make(map[id.RoomID]bool)
	for _, rule := range rules.Global.Room {
		if !rule.Enabled {
			continue
		}
		notifies := false
		for _, action := range rule.Actions {
			if string(action) == `"notify"` {
				notifies = true
			}
		}
		if !notifies {
			muted[rule.RuleID] = true
		}
	}
	return muted
}

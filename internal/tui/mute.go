package tui

import (
	"context"
	"encoding/json"
	"maps"
	"time"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
	"go.mau.fi/gomuks/pkg/rpc/store"
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
	if err == nil {
		err = view.setBridgeMute(muted)
	}
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

// setBridgeMute writes com.beeper.mute, which bridges relay to the network's own mute.
func (view *RoomView) setBridgeMute(muted bool) error {
	content := event.BeeperMuteEventContent{}
	if muted {
		content.MutedUntil = -1
	}
	raw, _ := json.Marshal(&content)
	return view.parent.matrix.SetAccountData(context.TODO(), &jsoncmd.SetAccountDataParams{
		RoomID:  view.Room.ID,
		Type:    event.AccountDataBeeperMute.Type,
		Content: raw,
	})
}

// SyncMutes copies every push-rule mute into com.beeper.mute so bridges mute them on the network.
func (view *RoomView) SyncMutes() {
	defer debug.Recover()
	list := view.parent.roomList
	list.lock.Lock()
	muted := maps.Clone(list.mutedLocked())
	list.lock.Unlock()
	view.AddServiceMessage("Syncing %d muted chats to their networks…", len(muted))
	view.parent.parent.Render()
	var synced, skipped, failed int
	raw := json.RawMessage(`{"muted_until":-1}`)
	for roomID := range muted {
		if bridgeMuted(view.parent.matrix.GetRoom(roomID)) {
			skipped++
			continue
		}
		err := view.parent.matrix.SetAccountData(context.TODO(), &jsoncmd.SetAccountDataParams{
			RoomID:  roomID,
			Type:    event.AccountDataBeeperMute.Type,
			Content: raw,
		})
		if err != nil {
			debug.Print("Failed to sync mute for", roomID, err)
			failed++
		} else {
			synced++
		}
		time.Sleep(200 * time.Millisecond)
	}
	view.AddServiceMessage("Mute sync done: %d synced, %d already synced, %d failed", synced, skipped, failed)
	view.parent.parent.Render()
}

func bridgeMuted(room *store.RoomStore) bool {
	if room == nil {
		return false
	}
	ad := room.GetAccountData(event.AccountDataBeeperMute)
	if ad == nil {
		return false
	}
	var content event.BeeperMuteEventContent
	return json.Unmarshal(ad.Content, &content) == nil && content.IsMuted()
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

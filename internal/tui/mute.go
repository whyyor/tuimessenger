package tui

import (
	"context"
	"encoding/json"
	"fmt"
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

// SyncMutes (any chat or none open) copies every push-rule mute into com.beeper.mute so bridges mute them on the network.
func (view *MainView) SyncMutes() {
	defer debug.Recover()
	list := view.roomList
	muted := maps.Clone(list.mutedSet())
	// Push rules outlive left/deleted chats; the server 404s on those.
	joined := joinedMuted(view.matrix.ReversedRoomList.Current(), muted)
	view.notify("Syncing %d muted chats to their networks…", len(joined))
	view.parent.Render()
	var synced, skipped, failed int
	raw := json.RawMessage(`{"muted_until":-1}`)
	for _, roomID := range joined {
		if bridgeMuted(view.matrix.GetRoom(roomID)) {
			skipped++
			continue
		}
		err := view.matrix.SetAccountData(context.TODO(), &jsoncmd.SetAccountDataParams{
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
	view.notify("Mute sync done: %d synced, %d already synced, %d failed (%d rules for chats you left ignored)",
		synced, skipped, failed, len(muted)-len(joined))
	view.parent.Render()
}

// PruneMutes deletes mute rules left behind by chats you've left or deleted.
func (view *MainView) PruneMutes() {
	defer debug.Recover()
	list := view.roomList
	muted := maps.Clone(list.mutedSet())
	rooms := view.matrix.ReversedRoomList.Current()
	if len(rooms) == 0 {
		view.notify("Room list not loaded yet; try again in a moment")
		view.parent.Render()
		return
	}
	for _, roomID := range joinedMuted(rooms, muted) {
		delete(muted, roomID)
	}
	var removed, failed, kept int
	for roomID := range muted {
		if view.matrix.GetRoom(roomID) != nil {
			kept++
			continue
		}
		_, err := view.matrix.MuteRoom(context.TODO(), &jsoncmd.MuteRoomParams{RoomID: roomID, Muted: false})
		if err != nil {
			debug.Print("Failed to prune mute rule for", roomID, err)
			failed++
		} else {
			removed++
		}
		time.Sleep(200 * time.Millisecond)
	}
	view.notify("Mute prune done: %d stale rules removed, %d failed, %d kept (chat still known locally)", removed, failed, kept)
	view.parent.Render()
}

func joinedMuted(rooms []*store.RoomListEntry, muted map[id.RoomID]bool) []id.RoomID {
	var out []id.RoomID
	for _, room := range rooms {
		if muted[room.RoomID] {
			out = append(out, room.RoomID)
		}
	}
	return out
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

// notify shows a gomuks notice as a toast; for commands that need no chat.
func (view *MainView) notify(text string, args ...any) {
	if len(args) > 0 {
		text = fmt.Sprintf(text, args...)
	}
	toasts.Show(classifyNotice(text), text)
}

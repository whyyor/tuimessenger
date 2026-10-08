package tui

import (
	"context"
	"encoding/json"

	"maunium.net/go/mautrix/event"

	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
	"go.mau.fi/gomuks/tui/debug"
)

// SetArchived files the chat under Archive via the standard m.lowpriority tag
// (Beeper and gomuks web show it as Low priority) and mutes it server-side,
// so no device notifies about it. Unarchiving reverses both.
func (view *RoomView) SetArchived(archived bool) {
	defer debug.Recover()
	ctx := context.TODO()
	var tags event.TagEventContent
	if ad := view.Room.GetAccountData(event.AccountDataRoomTags); ad != nil {
		_ = json.Unmarshal(ad.Content, &tags)
	}
	newTags, changed := withLowPriority(tags.Tags, archived)
	if changed {
		content, _ := json.Marshal(event.TagEventContent{Tags: newTags})
		err := view.parent.matrix.SetAccountData(ctx, &jsoncmd.SetAccountDataParams{
			RoomID:  view.Room.ID,
			Type:    event.AccountDataRoomTags.Type,
			Content: content,
		})
		if err != nil {
			view.AddServiceMessage("Failed to update the chat's tags: %v", err)
			view.parent.parent.Render()
			return
		}
	}
	_, err := view.parent.matrix.MuteRoom(ctx, &jsoncmd.MuteRoomParams{RoomID: view.Room.ID, Muted: archived})
	switch {
	case err != nil && archived:
		view.AddServiceMessage("Archived, but muting failed (it may still notify): %v", err)
	case err != nil:
		view.AddServiceMessage("Unarchived, but unmuting failed: %v", err)
	case archived:
		view.AddServiceMessage("Archived and muted. Alt+z shows the Archive section.")
	default:
		view.AddServiceMessage("Unarchived and unmuted.")
	}
	view.parent.parent.Render()
}

// withLowPriority returns a copy of tags with m.lowpriority added or removed,
// keeping every other tag (e.g. favourites), and whether anything changed.
func withLowPriority(tags event.Tags, set bool) (event.Tags, bool) {
	_, has := tags[event.RoomTagLowPriority]
	if has == set {
		return tags, false
	}
	out := make(event.Tags, len(tags)+1)
	for tag, meta := range tags {
		out[tag] = meta
	}
	if set {
		out[event.RoomTagLowPriority] = event.TagMetadata{Order: "0.5"}
	} else {
		delete(out, event.RoomTagLowPriority)
	}
	return out, true
}

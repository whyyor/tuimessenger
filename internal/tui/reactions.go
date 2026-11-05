package tui

import (
	"context"

	"go.mau.fi/util/variationselector"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
	"go.mau.fi/gomuks/tui/debug"
)

// sameReaction compares keys ignoring variation selectors (❤ vs ❤️).
func sameReaction(a, b string) bool {
	return variationselector.Remove(a) == variationselector.Remove(b)
}

// OwnReactions maps each of our live reaction keys on eventID to its event.
func (view *RoomView) OwnReactions(eventID id.EventID) map[string]id.EventID {
	evts, err := view.parent.matrix.GetRelatedEvents(context.TODO(), &jsoncmd.GetRelatedEventsParams{
		RoomID:       view.Room.ID,
		EventID:      eventID,
		RelationType: event.RelAnnotation,
		EventType:    event.EventReaction.Type,
	})
	if err != nil {
		debug.Print("Failed to look up own reactions:", err)
		return nil
	}
	return ownReactionKeys(evts, view.Room.OwnUserID())
}

func ownReactionKeys(evts []*database.Event, me id.UserID) map[string]id.EventID {
	out := make(map[string]id.EventID)
	for _, evt := range evts {
		if evt.Sender != me || evt.RedactedBy != "" {
			continue
		}
		content, ok := evt.GetMautrixContent().Parsed.(*event.ReactionEventContent)
		if !ok || content.RelatesTo.Key == "" {
			continue
		}
		out[content.RelatesTo.Key] = evt.ID
	}
	return out
}

// ToggleReaction removes our reaction if it's the same emoji, otherwise
// replaces whatever we had: one reaction per message, like the networks.
func (view *RoomView) ToggleReaction(eventID id.EventID, key string) {
	defer debug.Recover()
	own := view.OwnReactions(eventID)
	removedSame := false
	for ownKey, reactionID := range own {
		if sameReaction(ownKey, key) {
			removedSame = true
		}
		view.Redact(reactionID, "")
	}
	if !removedSame {
		view.SendReaction(eventID, key)
	}
	view.parent.parent.Render()
}

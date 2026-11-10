package messages

import (
	"context"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
	"github.com/whyyor/tuimessenger/internal/hicli/jsoncmd"
	"github.com/whyyor/tuimessenger/internal/rpc/client"
	"github.com/whyyor/tuimessenger/internal/rpc/store"
	"github.com/whyyor/tuimessenger/internal/tui/config"
	"github.com/whyyor/tuimessenger/internal/tui/debug"
	"github.com/whyyor/tuimessenger/internal/tui/messages/tstring"
)

const replyFetchTimeout = 30 * time.Second

// replyFetches holds quoted events asked of the backend, so each is fetched once unless it fails.
var replyFetches sync.Map

// attachReply sets msg.ReplyTo; a quote that isn't loaded gets a placeholder and a background fetch.
func attachReply(matrix *client.Client, prefs *config.UserPreferences, room *store.RoomStore, evt *database.Event, msg *UIMessage) {
	replyTo := evt.GetReplyTo()
	if replyTo == "" {
		return
	}
	replyRoom := room
	if roomID := evt.GetReplyToRoom(); roomID != "" && roomID != room.ID {
		replyRoom = nil
		if matrix != nil && matrix.Store != nil {
			replyRoom = matrix.GetRoom(roomID)
		}
		msg.ReplyRoomName = roomName(replyRoom)
	}
	var target *database.Event
	if replyRoom != nil {
		target = replyRoom.GetEventByID(replyTo)
	}
	if target != nil {
		// A loaded target that renders as nothing (e.g. hidden bridge noise) gets no quote.
		if msg.ReplyTo = renderReply(matrix, prefs, replyRoom, target); msg.ReplyTo == nil {
			return
		}
	} else {
		msg.ReplyTo = replyPlaceholder(room, replyTo)
		msg.missingReply, msg.replyRoom = replyTo, replyRoom
		fetchReply(matrix, replyRoom, replyTo)
	}
	msg.ReplyTo.IsReplyBubble = true
}

func renderReply(matrix *client.Client, prefs *config.UserPreferences, room *store.RoomStore, target *database.Event) *UIMessage {
	if cached, ok := target.RenderMeta.(*UIMessage); ok && cached != nil {
		return cached.Clone()
	}
	return directParseEvent(matrix, prefs, room, target)
}

// ReplyArrived reports that a quote shown as a placeholder has since loaded, so the message should be parsed again.
func (msg *UIMessage) ReplyArrived() bool {
	return msg != nil && msg.missingReply != "" && msg.replyRoom != nil && msg.replyRoom.GetEventByID(msg.missingReply) != nil
}

func replyPlaceholder(room *store.RoomStore, eventID id.EventID) *UIMessage {
	text := tstring.NewStyleTString("Original message isn't loaded", tcell.StyleDefault.Italic(true).Foreground(tcell.ColorGray))
	placeholder := NewExpandedTextMessage(&database.Event{ID: eventID}, room, text)
	placeholder.OverrideSenderName = "…"
	placeholder.DefaultSenderColor = tcell.ColorGray
	return placeholder
}

func fetchReply(matrix *client.Client, room *store.RoomStore, eventID id.EventID) {
	if matrix == nil || matrix.API == nil || room == nil {
		return
	}
	if _, asked := replyFetches.LoadOrStore(eventID, struct{}{}); asked {
		return
	}
	go func() {
		defer debug.Recover()
		ctx, cancel := context.WithTimeout(context.Background(), replyFetchTimeout)
		defer cancel()
		evt, err := matrix.GetEvent(ctx, &jsoncmd.GetEventParams{RoomID: room.ID, EventID: eventID})
		if err != nil || evt == nil {
			debug.Printf("Couldn't fetch replied-to event %s in %s: %v", eventID, room.ID, err)
			replyFetches.Delete(eventID) // a later re-parse may try again
			return
		}
		room.ApplyFetched(evt)
		RequestRender()
	}()
}

func roomName(room *store.RoomStore) string {
	if room == nil {
		return "another chat"
	}
	if name := ptr.Val(room.Meta.Current().Name); name != "" {
		return name
	}
	return "another chat"
}

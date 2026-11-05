package tui

import (
	"context"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli/jsoncmd"
	"github.com/whyyor/tuimessenger/internal/tui/debug"
	"github.com/whyyor/tuimessenger/internal/tui/messages"
)

type retryKind int

const (
	retryNone     retryKind = iota
	retryNotOwn             // only your own messages can be sent again
	retryResend             // the backend never got it: send the same event again
	retrySendCopy           // the network refused it: send the content as a new message
)

// retryKindFor decides how R retries a message. networkFailed is the bridge's FAIL_PERMANENT status.
func retryKindFor(msg *messages.UIMessage, own id.UserID, networkFailed bool) retryKind {
	switch {
	case msg == nil || msg.Event == nil:
		return retryNone
	case msg.Sender != own:
		return retryNotOwn
	case msg.SendError != "" && msg.SendError != "not sent" && msg.TransactionID != "":
		return retryResend
	case networkFailed:
		return retrySendCopy
	}
	return retryNone
}

// networkRefused reports whether the bridge said the network refused this message (the red ! tick).
func networkRefused(evtID id.EventID) bool {
	deliveries.lock.Lock()
	defer deliveries.lock.Unlock()
	entry := deliveries.entries[evtID]
	return entry != nil && entry.state == deliveryFailed
}

// RetryMessage sends one of your failed messages again (R in visual mode). If the network had refused it, the
// content goes out as a new message and the failed copy is hidden (back with /unclear).
func (view *RoomView) RetryMessage(msg *messages.UIMessage) {
	defer debug.Recover()
	defer view.parent.parent.Render()
	var networkFailed bool
	if msg != nil {
		networkFailed = networkRefused(msg.ID)
	}
	switch retryKindFor(msg, view.Room.OwnUserID(), networkFailed) {
	case retryNotOwn:
		view.AddServiceMessage("Only your own messages can be sent again")
	case retryResend:
		_, err := view.parent.matrix.ResendEvent(context.TODO(), &jsoncmd.ResendEventParams{TransactionID: msg.TransactionID})
		if err != nil {
			view.AddServiceMessage("Failed to send it again: %v", err)
		}
	case retrySendCopy:
		_, err := view.parent.matrix.SendEvent(context.TODO(), &jsoncmd.SendEventParams{
			RoomID:    view.Room.ID,
			EventType: event.Type{Type: msg.Type, Class: event.MessageEventType},
			Content:   msg.Content,
		})
		if err != nil {
			view.AddServiceMessage("Failed to send it again: %v", err)
			return
		}
		view.HideMessage(msg.ID)
	default:
		view.AddServiceMessage("That message didn't fail, so there's nothing to retry")
	}
}

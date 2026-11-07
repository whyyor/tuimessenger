package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
)

// The reply line names the person the way the chat does, not by their bridge ID.
func TestReplyStatusShowsName(t *testing.T) {
	mv, room := bubbleView(t)
	view := mv.parent
	msg := textMsg(room, "@whatsapp_lid-10000000000004:tuimessenger.localhost", time.Now(), "hi")
	msg.OverrideSenderName = "Mariana"
	view.startReply(msg)
	if got := view.GetStatus(); !strings.Contains(got, "Replying to Mariana") || strings.Contains(got, "whatsapp_lid") {
		t.Errorf("status: %q", got)
	}
	// No name known when replying: the room's name for the sender.
	view.replying, view.replyingName = &database.Event{Sender: "@peter:x"}, ""
	if got := view.GetStatus(); !strings.Contains(got, "Replying to ") {
		t.Errorf("status without a name: %q", got)
	}
}

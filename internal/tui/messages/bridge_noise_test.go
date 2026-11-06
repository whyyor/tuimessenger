package messages

import (
	"encoding/json"
	"testing"

	"maunium.net/go/mautrix/event"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
)

func TestBridgeNoise(t *testing.T) {
	evt := func(msgtype event.MessageType, body string) *database.Event {
		content, _ := json.Marshal(map[string]any{"msgtype": msgtype, "body": body})
		return &database.Event{Type: event.EventMessage.Type, Content: content}
	}
	cases := []struct {
		evt  *database.Event
		want bool
	}{
		{evt(event.MsgNotice, "Kept a message"), true},
		{evt(event.MsgNotice, "Unkept a message"), true},
		{evt(event.MsgText, "Kept a message"), false}, // someone typing those words is a real message
		{evt(event.MsgNotice, "Pinned a message"), false},
		{evt(event.MsgText, "Game?"), false},
	}
	for _, c := range cases {
		if got := isBridgeNoise(c.evt); got != c.want {
			t.Errorf("%s: got %v, want %v", c.evt.Content, got, c.want)
		}
	}
}

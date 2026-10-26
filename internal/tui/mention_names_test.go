package tui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"go.mau.fi/util/jsontime"

	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/tui/config"
	"go.mau.fi/gomuks/tui/messages"
)

// WhatsApp's "mention all": the HTML carries each name, members aren't loaded.
func TestMentionsShowNamesNotIDs(t *testing.T) {
	_, room := bubbleView(t)
	content, _ := json.Marshal(map[string]any{
		"msgtype": "m.text",
		"body":    "Alex Rivera Mariana",
		"format":  "org.matrix.custom.html",
		"formatted_body": `<a href="https://matrix.to/#/@alex:beeper.com">Alex Rivera</a> ` +
			`<a href="https://matrix.to/#/@whatsapp_lid-10000000000004:beeper.local">Mariana</a>`,
	})
	evt := &database.Event{ID: "$m:x", RoomID: room.ID, Sender: "@whatsapp_lid-1:beeper.local",
		Type: "m.room.message", Content: content, Timestamp: jsontime.UM(time.Now())}
	prefs := config.UserPreferences{}
	msg := messages.ParseEvent(nil, &prefs, room, evt)
	text := msg.PlainText()
	if strings.Contains(text, "whatsapp_lid") || !strings.Contains(text, "Mariana") || !strings.Contains(text, "Alex Rivera") {
		t.Errorf("mentions should read as names: %q", text)
	}
}

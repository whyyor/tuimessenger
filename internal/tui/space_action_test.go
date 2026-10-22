package tui

import (
	"testing"
	"time"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/tui/messages"
)

func fileMsg(msgType event.MessageType, mime string) *messages.UIMessage {
	return &messages.UIMessage{Event: &database.Event{}, Renderer: &messages.FileMessage{
		Type: msgType, MimeType: mime, URL: id.ContentURI{Homeserver: "beeper.local", FileID: "x"},
	}}
}

func TestSpaceActionFor(t *testing.T) {
	_, room := bubbleView(t)
	for _, c := range []struct {
		name string
		msg  *messages.UIMessage
		want spaceAction
		url  string
	}{
		{"image", fileMsg(event.MsgImage, "image/jpeg"), spaceViewImage, ""},
		{"voice note", fileMsg(event.MsgAudio, "audio/ogg; codecs=opus"), spacePlayAudio, ""},
		{"audio sent as file", fileMsg(event.MsgFile, "audio/mpeg"), spacePlayAudio, ""},
		{"video", fileMsg(event.MsgVideo, "video/mp4"), spaceOpenFile, ""},
		{"pdf", fileMsg(event.MsgFile, "application/pdf"), spaceOpenFile, ""},
		{"text with link", textMsg(room, "@a:x", time.Now(), "see https://example.com/x ok"), spaceOpenLink, "https://example.com/x"},
		{"plain text", textMsg(room, "@a:x", time.Now(), "hello"), spaceNothing, ""},
		{"nothing selected", nil, spaceNothing, ""},
	} {
		got, url := spaceActionFor(c.msg)
		if got != c.want || url != c.url {
			t.Errorf("%s: got %v %q, want %v %q", c.name, got, url, c.want, c.url)
		}
	}
}

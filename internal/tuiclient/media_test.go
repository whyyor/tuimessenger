package tuiclient

import (
	"testing"

	"maunium.net/go/mautrix/event"
)

func TestMsgTypeFor(t *testing.T) {
	cases := map[string]event.MessageType{
		"image/jpeg":         event.MsgImage,
		"image/png":          event.MsgImage,
		"image/gif":          event.MsgImage,
		"image/webp":         event.MsgImage,
		"image/svg+xml":      event.MsgFile,
		"image/tiff":         event.MsgFile,
		"image/heic":         event.MsgFile,
		"video/mp4":          event.MsgVideo,
		"audio/ogg":          event.MsgAudio,
		"application/pdf":    event.MsgFile,
		"application/x-what": event.MsgFile,
	}
	for mime, want := range cases {
		if got := msgTypeFor(mime); got != want {
			t.Errorf("%s: got %s, want %s", mime, got, want)
		}
	}
}

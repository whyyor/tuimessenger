package messages

import (
	"bytes"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
	"github.com/whyyor/tuimessenger/internal/rpc/store"
)

func parseFileMessage(t *testing.T, raw string) *FileMessage {
	t.Helper()
	gs := &store.Store{}
	gs.UserID = "@me:x"
	room := store.NewRoomStore(gs, &database.Room{ID: "!room:x"})
	content := &event.Content{VeryRaw: []byte(raw)}
	if err := content.ParseRaw(event.EventMessage); err != nil {
		t.Fatal(err)
	}
	evt := &database.Event{ID: "$e:x", RoomID: room.ID, Sender: "@a:x", Type: "m.room.message", Content: []byte(raw)}
	return NewFileMessage(room, nil, evt, content.AsMessage()).Renderer.(*FileMessage)
}

func TestGIFsAndVideosGetPreviews(t *testing.T) {
	video := id.ContentURI{Homeserver: "x", FileID: "video"}
	thumb := id.ContentURI{Homeserver: "x", FileID: "thumb"}
	for _, c := range []struct {
		name      string
		raw       string
		src       id.ContentURI
		encrypted bool
		badge     string
		gif       bool
		notif     string
	}{
		{"WhatsApp GIF (a short silent mp4, no thumbnail)",
			`{"msgtype":"m.video","body":"","url":"mxc://x/video","info":{"mimetype":"video/mp4","size":300000,"duration":3000,"fi.mau.gif":true}}`,
			video, false, "GIF", true, "Sent a GIF"},
		{"video with the bridge's thumbnail",
			`{"msgtype":"m.video","body":"clip.mp4","url":"mxc://x/video","info":{"mimetype":"video/mp4","size":90000000,"duration":75000,"thumbnail_url":"mxc://x/thumb"}}`,
			thumb, false, "▶ 1:15", false, "Sent a video"},
		{"small video without a thumbnail: first frame of the video",
			`{"msgtype":"m.video","body":"clip.mp4","url":"mxc://x/video","info":{"mimetype":"video/mp4","size":2000000}}`,
			video, false, "▶ Video", false, "Sent a video"},
		{"big video without a thumbnail: not downloaded for a picture",
			`{"msgtype":"m.video","body":"film.mp4","url":"mxc://x/video","info":{"mimetype":"video/mp4","size":50000000,"duration":3725000}}`,
			id.ContentURI{}, false, "▶ 1:02:05", false, "Sent a video"},
		{"video of unknown size without a thumbnail",
			`{"msgtype":"m.video","body":"clip.mp4","url":"mxc://x/video"}`,
			id.ContentURI{}, false, "▶ Video", false, "Sent a video"},
		{"encrypted thumbnail",
			`{"msgtype":"m.video","body":"clip.mp4","url":"mxc://x/video","info":{"thumbnail_file":{"url":"mxc://x/thumb","v":"v2","key":{"kty":"oct","key_ops":["encrypt","decrypt"],"alg":"A256CTR","k":"a","ext":true},"iv":"b","hashes":{"sha256":"c"}}}}`,
			thumb, true, "▶ Video", false, "Sent a video"},
		{"Slack GIF (a real image/gif)",
			`{"msgtype":"m.image","body":"party.gif","url":"mxc://x/video","info":{"mimetype":"image/gif","size":800000}}`,
			video, false, "GIF", true, "Sent a GIF"},
		{"photo",
			`{"msgtype":"m.image","body":"photo.jpg","url":"mxc://x/video","info":{"mimetype":"image/jpeg"}}`,
			video, false, "", false, "Sent an image"},
	} {
		fm := parseFileMessage(t, c.raw)
		src, encrypted := fm.previewSource()
		if src != c.src || encrypted != c.encrypted {
			t.Errorf("%s: preview from %v (encrypted %v), want %v (%v)", c.name, src, encrypted, c.src, c.encrypted)
		}
		if got := fm.MediaBadge(); got != c.badge {
			t.Errorf("%s: badge %q, want %q", c.name, got, c.badge)
		}
		if fm.IsGIF != c.gif {
			t.Errorf("%s: IsGIF %v, want %v", c.name, fm.IsGIF, c.gif)
		}
		if got := fm.NotificationContent(); got != c.notif {
			t.Errorf("%s: notification %q, want %q", c.name, got, c.notif)
		}
	}
}

func TestVideoDurationParsed(t *testing.T) {
	fm := parseFileMessage(t, `{"msgtype":"m.video","body":"a.mp4","url":"mxc://x/v","info":{"duration":12400}}`)
	if fm.Duration != 12400*time.Millisecond || fm.MediaBadge() != "▶ 0:12" {
		t.Errorf("duration %v badge %q", fm.Duration, fm.MediaBadge())
	}
}

// The picture of a GIF without a thumbnail is the video's first frame, taken by ffmpeg.
func TestFirstFrameOfAVideo(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg isn't installed")
	}
	mp4 := filepath.Join(t.TempDir(), "gif.mp4")
	if out, err := exec.Command(ffmpeg, "-loglevel", "error", "-f", "lavfi", "-i", "color=red:s=64x48:d=1",
		"-pix_fmt", "yuv420p", mp4).CombinedOutput(); err != nil {
		t.Fatalf("making a test video: %v %s", err, out)
	}
	data, err := os.ReadFile(mp4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = encodeToPNG(data); err == nil {
		t.Fatal("an mp4 isn't an image; the ffmpeg fallback must be what decodes it")
	}
	png, err := ffmpegFirstFramePNG(data)
	if err != nil {
		t.Fatal(err)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(png))
	if err != nil || format != "png" || cfg.Width != 64 || cfg.Height != 48 {
		t.Errorf("first frame: %v %s %dx%d", err, format, cfg.Width, cfg.Height)
	}
}

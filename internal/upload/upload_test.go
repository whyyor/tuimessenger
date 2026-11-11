package upload

import (
	"archive/zip"
	"bytes"
	"image"
	"image/png"
	"io"
	"strings"
	"testing"

	"maunium.net/go/mautrix/event"
)

func TestMsgType(t *testing.T) {
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
		if got := MsgType(mime); got != want {
			t.Errorf("%s: got %s, want %s", mime, got, want)
		}
	}
}

func TestContentImage(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	c := Content("dot.png", buf.Bytes())
	if c.MsgType != event.MsgImage || c.Body != "dot.png" || c.FileName != "dot.png" {
		t.Errorf("content: %+v", c)
	}
	if c.Info.MimeType != "image/png" || c.Info.Size != buf.Len() || c.Info.Width != 3 || c.Info.Height != 2 {
		t.Errorf("info: %+v", c.Info)
	}
}

// An Android app (a zip underneath) goes as a plain file.
func TestContentAPKIsAFile(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("AndroidManifest.xml")
	_, _ = w.Write([]byte("<manifest/>"))
	_ = zw.Close()
	if c := Content("app.apk", buf.Bytes()); c.MsgType != event.MsgFile || c.Info.Width != 0 {
		t.Errorf("an apk should be a file: %+v %+v", c, c.Info)
	}
}

func TestReadLimit(t *testing.T) {
	if raw, err := Read(strings.NewReader("hello")); err != nil || string(raw) != "hello" {
		t.Errorf("small file: %q %v", raw, err)
	}
	if _, err := Read(io.LimitReader(zeros{}, MaxSize+1)); err == nil {
		t.Error("a file over MaxSize must be refused")
	}
	if raw, err := Read(io.LimitReader(zeros{}, MaxSize)); err != nil || len(raw) != MaxSize {
		t.Errorf("exactly MaxSize is fine: %d %v", len(raw), err)
	}
}

type zeros struct{}

func (zeros) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

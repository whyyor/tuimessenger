package messages

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/tui/config"
)

func TestCaption(t *testing.T) {
	for _, c := range []struct {
		msg  FileMessage
		want string
	}{
		{FileMessage{FileName: "image.jpg", Body: "are we required to set this?"}, "are we required to set this?"},
		{FileMessage{FileName: "", Body: "image.jpg"}, ""},
		{FileMessage{FileName: "image.jpg", Body: "image.jpg"}, ""},
		{FileMessage{FileName: "r.jpg", LinkURL: "https://x.com/reel/1", Body: "[https://x.com/reel/1](https://x.com/reel/1)"}, ""},
		{FileMessage{FileName: "r.jpg", LinkURL: "https://x.com/reel/1", Body: "look [https://x.com/reel/1](https://x.com/reel/1)"}, "look"},
	} {
		if got := c.msg.Caption(); got != c.want {
			t.Errorf("%+v: got %q, want %q", c.msg, got, c.want)
		}
	}
}

func TestImageShowsCaptionBelow(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := range 8 {
		for y := range 8 {
			img.Set(x, y, color.White)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	fm := &FileMessage{FileName: "image.jpg", Body: "hello caption", imageData: buf.Bytes()}
	ui := &UIMessage{Event: &database.Event{}, Renderer: fm}
	prefs := config.UserPreferences{ImageProtocol: "ansi"}
	ui.CalculateBuffer(prefs, 40)
	imgRows := len(fm.buffer)
	if imgRows == 0 || fm.Height() != imgRows+1 {
		t.Fatalf("image rows %d, height %d: want caption as one extra row", imgRows, fm.Height())
	}
	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(40, fm.Height())
	fm.Draw(sim, ui)
	var row strings.Builder
	for x := range 13 {
		r, _, _, _ := sim.GetContent(x, imgRows)
		row.WriteRune(r)
	}
	if row.String() != "hello caption" {
		t.Errorf("caption row: %q", row.String())
	}
}

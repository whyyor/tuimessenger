package messages

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"maunium.net/go/mautrix/event"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
	"github.com/whyyor/tuimessenger/internal/tui/config"
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
	fm := &FileMessage{FileName: "image.jpg", Body: "hello caption"}
	if err := fm.setPreview(buf.Bytes()); err != nil {
		t.Fatal(err)
	}
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

func TestStickerDrawnSmall(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 512, 512))
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	size := func(sticker bool) (int, int) {
		fm := &FileMessage{IsSticker: sticker}
		if err := fm.setPreview(buf.Bytes()); err != nil {
			t.Fatal(err)
		}
		ui := &UIMessage{Event: &database.Event{}, Renderer: fm}
		ui.CalculateBuffer(config.UserPreferences{ImageProtocol: "ansi"}, 100)
		w := 0
		for _, line := range fm.buffer {
			w = max(w, len(line))
		}
		return w, len(fm.buffer)
	}
	pw, ph := size(false)
	sw, sh := size(true)
	if sw > stickerMaxCols || sh > stickerMaxRows || sw >= pw || sh >= ph {
		t.Errorf("sticker %dx%d should fit %dx%d and be smaller than a photo %dx%d",
			sw, sh, stickerMaxCols, stickerMaxRows, pw, ph)
	}
}

func TestImagePreviewMemory(t *testing.T) {
	var buf bytes.Buffer
	_ = png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 64, 32)))
	fm := &FileMessage{Type: event.MsgImage}
	if err := fm.setPreview(buf.Bytes()); err != nil {
		t.Fatal(err)
	}
	if clone := fm.Clone().(*FileMessage); clone.imgW != 64 || clone.imgH != 32 || len(clone.kittyPNG) == 0 {
		t.Errorf("clone lost the preview: %dx%d, %d bytes", clone.imgW, clone.imgH, len(clone.kittyPNG))
	}

	// In kitty mode the PNG is dropped once the terminal has it; half-blocks then have to fetch it again.
	fm.kittyPNG = nil
	fm.downloaded = true
	ui := &UIMessage{Event: &database.Event{}, Renderer: fm}
	ui.CalculateBuffer(config.UserPreferences{ImageProtocol: "ansi"}, 40)
	if fm.downloaded || len(fm.buffer) != 1 || !strings.HasPrefix(fm.buffer[0].String(), "Loading image") {
		t.Errorf("want a refetch and a loading line, got downloaded=%v buffer=%v", fm.downloaded, fm.buffer)
	}
}

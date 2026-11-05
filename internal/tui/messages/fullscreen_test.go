package messages

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"
)

func TestFitCells(t *testing.T) {
	// 200x50 terminal of 10x20px cells = 2000x1000px.
	cases := []struct {
		name               string
		imgW, imgH         int
		wantCols, wantRows int
	}{
		{"wide screenshot fills the width", 2000, 500, 200, 25},
		{"tall phone screenshot fills the height", 1000, 2000, 50, 50},
		{"small image is scaled up to fit", 100, 50, 200, 50},
	}
	for _, c := range cases {
		cols, rows := FitCells(c.imgW, c.imgH, 200, 50, 10, 20)
		if cols != c.wantCols || rows != c.wantRows {
			t.Errorf("%s: got %dx%d, want %dx%d", c.name, cols, rows, c.wantCols, c.wantRows)
		}
	}
}

func TestFullscreenPNGShrinksAndConverts(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 3000, 1500))
	src.Set(0, 0, color.White)
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, src, nil); err != nil {
		t.Fatal(err)
	}
	pngData, w, h, err := FullscreenPNG(jpg.Bytes(), 1200, 800)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pngData, []byte("\x89PNG")) {
		t.Error("output is not PNG")
	}
	if w != 1200 || h != 600 {
		t.Errorf("got %dx%d, want 1200x600 (fit, aspect kept)", w, h)
	}
}

func TestWriteFullscreenChunksAndCentres(t *testing.T) {
	var out bytes.Buffer
	data := bytes.Repeat([]byte{1}, 5000) // > one 4096-byte base64 chunk
	if err := WriteFullscreen(&out, data, 100, 40, 200, 50); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "\x1b[6;51H") {
		t.Error("image not centred at row 6, column 51")
	}
	if !strings.Contains(s, "c=100,r=40,m=1;") || !strings.Contains(s, "\x1b_Gm=0;") {
		t.Error("expected a first chunk with more=1 and a final chunk with more=0")
	}
}

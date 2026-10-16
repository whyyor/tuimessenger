package messages

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"io"

	"github.com/disintegration/imaging"
)

// fullscreenImageID is outside the range inline images use (they count up
// from 1), so deleting the viewer's image never touches the chat's.
const fullscreenImageID = 0xfffffe

// FullscreenPNG prepares image data for the viewer: decoded (first frame for
// animated stickers), shrunk to at most maxW x maxH pixels so a huge photo
// doesn't flood the tty, and PNG-encoded as the kitty protocol requires.
func FullscreenPNG(data []byte, maxW, maxH int) (pngData []byte, w, h int, err error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		frame, ffErr := ffmpegFirstFramePNG(data)
		if ffErr != nil {
			return nil, 0, 0, err
		}
		if img, err = png.Decode(bytes.NewReader(frame)); err != nil {
			return nil, 0, 0, err
		}
	}
	if b := img.Bounds(); maxW > 0 && maxH > 0 && (b.Dx() > maxW || b.Dy() > maxH) {
		img = imaging.Fit(img, maxW, maxH, imaging.Lanczos)
	}
	var buf bytes.Buffer
	if err = png.Encode(&buf, img); err != nil {
		return nil, 0, 0, err
	}
	b := img.Bounds()
	return buf.Bytes(), b.Dx(), b.Dy(), nil
}

// FitCells sizes an imgW x imgH image to fill a termCols x termRows grid of
// cellW x cellH pixel cells as far as possible without distortion; kitty
// stretches the image to exactly the cell box it's given.
func FitCells(imgW, imgH, termCols, termRows, cellW, cellH int) (cols, rows int) {
	if imgW < 1 || imgH < 1 || cellW < 1 || cellH < 1 {
		return termCols, termRows
	}
	scale := min(float64(termCols*cellW)/float64(imgW), float64(termRows*cellH)/float64(imgH))
	cols = int(float64(imgW)*scale/float64(cellW) + 0.5)
	rows = int(float64(imgH)*scale/float64(cellH) + 0.5)
	return max(1, min(cols, termCols)), max(1, min(rows, termRows))
}

// WriteFullscreen draws the image centred in a cleared alternate screen.
func WriteFullscreen(w io.Writer, pngData []byte, cols, rows, termCols, termRows int) error {
	var buf bytes.Buffer
	// Alternate screen keeps the image out of scrollback; hide the cursor.
	buf.WriteString("\x1b[?1049h\x1b[?25l\x1b[2J")
	fmt.Fprintf(&buf, "\x1b[%d;%dH", (termRows-rows)/2+1, (termCols-cols)/2+1)
	b64 := base64.StdEncoding.EncodeToString(pngData)
	const chunkSize = 4096
	for first := true; len(b64) > 0; first = false {
		chunk := b64[:min(chunkSize, len(b64))]
		b64 = b64[len(chunk):]
		more := 0
		if len(b64) > 0 {
			more = 1
		}
		if first {
			// C=1: don't move the cursor, so the terminal can't scroll.
			fmt.Fprintf(&buf, "\x1b_Ga=T,f=100,q=2,C=1,i=%d,c=%d,r=%d,m=%d;%s\x1b\\",
				fullscreenImageID, cols, rows, more, chunk)
		} else {
			fmt.Fprintf(&buf, "\x1b_Gm=%d;%s\x1b\\", more, chunk)
		}
	}
	_, err := w.Write(buf.Bytes())
	return err
}

// ClearFullscreen frees the viewer's image and leaves the alternate screen.
func ClearFullscreen(w io.Writer) {
	_, _ = fmt.Fprintf(w, "\x1b_Ga=d,d=I,q=2,i=%d\x1b\\\x1b[2J\x1b[?25h\x1b[?1049l", fullscreenImageID)
}

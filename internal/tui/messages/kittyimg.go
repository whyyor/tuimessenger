package messages

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"os"
	"strings"
	"sync"

	"go.mau.fi/gomuks/tui/debug"
)

// Kitty graphics protocol, unicode placeholder flavor (U=1). The image is
// transmitted once over the raw tty; the timeline then draws ordinary cells
// of U+10EEEE whose combining diacritics encode the row/column and whose
// foreground color encodes the image ID. The terminal composites the pixels
// wherever those cells end up, so scrolling and clipping behave like text.

// kittyPlaceholder is the codepoint reserved for image placeholder cells.
const kittyPlaceholder rune = 0x10EEEE

// kittyRowColDiacritics is the start of the canonical diacritic table from
// kitty's rowcolumn-diacritics.txt; index n encodes row or column n. 48
// entries covers the maximum grid this renderer produces.
var kittyRowColDiacritics = [48]rune{
	0x0305, 0x030D, 0x030E, 0x0310, 0x0312, 0x033D, 0x033E, 0x033F,
	0x0346, 0x034A, 0x034B, 0x034C, 0x0350, 0x0351, 0x0352, 0x0357,
	0x035B, 0x0363, 0x0364, 0x0365, 0x0366, 0x0367, 0x0368, 0x0369,
	0x036A, 0x036B, 0x036C, 0x036D, 0x036E, 0x036F, 0x0483, 0x0484,
	0x0485, 0x0486, 0x0487, 0x0592, 0x0593, 0x0594, 0x0595, 0x0597,
	0x0598, 0x0599, 0x059C, 0x059D, 0x059E, 0x059F, 0x05A0, 0x05A1,
}

// kittyTerminalSupported sniffs whether the terminal speaks the protocol.
// Ghostty and kitty both implement unicode placeholders.
var kittyTerminalSupported = func() bool {
	term := os.Getenv("TERM")
	prog := os.Getenv("TERM_PROGRAM")
	return prog == "ghostty" || prog == "kitty" ||
		strings.Contains(term, "ghostty") || strings.Contains(term, "kitty")
}()

var (
	kittyTTY     *os.File
	kittyTTYOnce sync.Once
	kittyLock    sync.Mutex

	kittyNextID uint32 = 1
	// kittyImages tracks transmitted images by media key so the same sticker
	// is uploaded to the terminal once per session, shared across clones.
	kittyImages = map[string]*kittyImage{}
)

type kittyImage struct {
	id          uint32
	transmitted bool
	placedCols  int
	placedRows  int
}

func kittyTTYFile() *os.File {
	kittyTTYOnce.Do(func() {
		f, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
		if err != nil {
			debug.Print("Kitty graphics disabled, cannot open tty:", err)
			return
		}
		kittyTTY = f
	})
	return kittyTTY
}

// kittyUsable reports whether placeholder rendering should be used at all.
func kittyUsable(protocolPref string) bool {
	switch protocolPref {
	case "kitty":
		return kittyTTYFile() != nil
	case "ansi", "off":
		return false
	default:
		return kittyTerminalSupported && kittyTTYFile() != nil
	}
}

// kittyImageFor returns the session-wide state for a media key.
func kittyImageFor(key string) *kittyImage {
	kittyLock.Lock()
	defer kittyLock.Unlock()
	img, ok := kittyImages[key]
	if !ok {
		img = &kittyImage{id: kittyNextID}
		kittyNextID++
		kittyImages[key] = img
	}
	return img
}

// kittyEnsurePlaced transmits the PNG (once) and (re)creates the virtual
// placement whenever the requested cell box changes. Called from the render
// thread, so writes are sequenced with tcell's own output. q=2 suppresses
// terminal responses, which would otherwise land in tcell's input stream.
func kittyEnsurePlaced(img *kittyImage, pngData []byte, cols, rows int) bool {
	tty := kittyTTYFile()
	if tty == nil {
		return false
	}
	kittyLock.Lock()
	defer kittyLock.Unlock()
	var buf bytes.Buffer
	if !img.transmitted {
		b64 := base64.StdEncoding.EncodeToString(pngData)
		const chunkSize = 4096
		first := true
		for len(b64) > 0 {
			chunk := b64
			if len(chunk) > chunkSize {
				chunk = b64[:chunkSize]
			}
			b64 = b64[len(chunk):]
			more := 0
			if len(b64) > 0 {
				more = 1
			}
			if first {
				fmt.Fprintf(&buf, "\x1b_Ga=t,f=100,t=d,q=2,i=%d,m=%d;%s\x1b\\", img.id, more, chunk)
				first = false
			} else {
				fmt.Fprintf(&buf, "\x1b_Gm=%d;%s\x1b\\", more, chunk)
			}
		}
	}
	if !img.transmitted || img.placedCols != cols || img.placedRows != rows {
		// p=1: reuse the placement ID so a resize replaces instead of stacking.
		fmt.Fprintf(&buf, "\x1b_Ga=p,U=1,q=2,i=%d,p=1,c=%d,r=%d\x1b\\", img.id, cols, rows)
	}
	if buf.Len() > 0 {
		if _, err := tty.Write(buf.Bytes()); err != nil {
			debug.Print("Failed to write kitty graphics:", err)
			return false
		}
	}
	img.transmitted = true
	img.placedCols = cols
	img.placedRows = rows
	return true
}

// encodeToPNG converts arbitrary decoded-supported image data (webp stickers,
// jpeg photos) to PNG, which is the only compressed format in the protocol.
func encodeToPNG(data []byte) ([]byte, error) {
	if bytes.HasPrefix(data, []byte("\x89PNG")) {
		return data, nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

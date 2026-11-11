// Package upload turns a file into the message the bridges send for it, for the TUI and the agent alike.
package upload

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	_ "golang.org/x/image/webp"
	"maunium.net/go/mautrix/event"
)

// MaxSize is the biggest file tuimessenger sends.
const MaxSize = 100 << 20

// Read reads a whole file, refusing ones over MaxSize.
func Read(r io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, MaxSize+1))
	if err != nil {
		return nil, err
	} else if len(raw) > MaxSize {
		return nil, fmt.Errorf("file is bigger than %d MB", MaxSize>>20)
	}
	return raw, nil
}

// Content is the message for a file: its kind from its bytes, its size, and an image's dimensions. The caller sets
// URL after uploading it (with ContentType as the upload's content type).
func Content(filename string, raw []byte) *event.MessageEventContent {
	mime := mimetype.Detect(raw).String()
	content := &event.MessageEventContent{
		MsgType:  MsgType(mime),
		Body:     filename,
		FileName: filename,
		Info:     &event.FileInfo{MimeType: mime, Size: len(raw)},
	}
	if content.MsgType == event.MsgImage {
		if cfg, _, err := image.DecodeConfig(bytes.NewReader(raw)); err == nil {
			content.Info.Width, content.Info.Height = cfg.Width, cfg.Height
		}
	}
	return content
}

// photoTypes are the image formats every network accepts as a photo; other images (SVG, TIFF, HEIC…) go as files.
var photoTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/gif": true, "image/webp": true}

// MsgType is how a file of this type is sent: photo, video, audio, or a plain file.
func MsgType(mime string) event.MessageType {
	switch strings.Split(mime, "/")[0] {
	case "image":
		if !photoTypes[mime] {
			return event.MsgFile
		}
		return event.MsgImage
	case "video":
		return event.MsgVideo
	case "audio":
		return event.MsgAudio
	default:
		return event.MsgFile
	}
}

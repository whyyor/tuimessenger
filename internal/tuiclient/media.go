package tuiclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	_ "golang.org/x/image/webp"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/rpc"
)

const maxUploadSize = 100 << 20

// UploadMedia uploads a file to the backend and returns the message content for it, like the daemon's upload did
// (without its thumbnails, blurhashes and video probing, which the bridges don't need).
func (b *Backend) UploadMedia(ctx context.Context, filename string, encrypt bool, data io.Reader) (*event.MessageEventContent, error) {
	if encrypt {
		return nil, errors.New("sending files to encrypted rooms isn't supported")
	}
	raw, err := io.ReadAll(io.LimitReader(data, maxUploadSize+1))
	if err != nil {
		return nil, err
	} else if len(raw) > maxUploadSize {
		return nil, fmt.Errorf("file is bigger than %d MB", maxUploadSize>>20)
	}
	mime := mimetype.Detect(raw)
	content := &event.MessageEventContent{
		MsgType:  msgTypeFor(mime.String()),
		Body:     filename,
		FileName: filename,
		Info:     &event.FileInfo{MimeType: mime.String(), Size: len(raw)},
	}
	if content.MsgType == event.MsgImage {
		if cfg, _, err := image.DecodeConfig(bytes.NewReader(raw)); err == nil {
			content.Info.Width, content.Info.Height = cfg.Width, cfg.Height
		}
	}
	resp, err := b.hc.Client.UploadMedia(ctx, mautrix.ReqUploadMedia{
		ContentBytes: raw,
		ContentType:  mime.String(),
		FileName:     filename,
	})
	if err != nil {
		return nil, err
	}
	content.URL = resp.ContentURI.CUString()
	return content, nil
}

// photoTypes are the image formats every network accepts as a photo; other images (SVG, TIFF, HEIC…) go as files.
var photoTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/gif": true, "image/webp": true}

func msgTypeFor(mime string) event.MessageType {
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

// DownloadMedia fetches a file from the backend.
func (b *Backend) DownloadMedia(ctx context.Context, params rpc.DownloadMediaParams) (*http.Response, error) {
	return b.hc.Client.Download(ctx, params.MXC)
}

// MediaURL is the backend's address for a file. It needs the access token, so a browser can't open it yet.
func (b *Backend) MediaURL(mxc id.ContentURI, _ url.Values) string {
	return b.hc.Client.BuildURL(mautrix.ClientURLPath{"v1", "media", "download", mxc.Homeserver, mxc.FileID})
}

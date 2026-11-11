package tuiclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/rpc"
	"github.com/whyyor/tuimessenger/internal/upload"
)

// UploadMedia uploads a file to the backend and returns the message content for it, like the daemon's upload did
// (without its thumbnails, blurhashes and video probing, which the bridges don't need).
func (b *Backend) UploadMedia(ctx context.Context, filename string, encrypt bool, data io.Reader) (*event.MessageEventContent, error) {
	if encrypt {
		return nil, errors.New("sending files to encrypted rooms isn't supported")
	}
	raw, err := upload.Read(data)
	if err != nil {
		return nil, err
	}
	content := upload.Content(filename, raw)
	resp, err := b.hc.Client.UploadMedia(ctx, mautrix.ReqUploadMedia{
		ContentBytes: raw,
		ContentType:  content.Info.MimeType,
		FileName:     filename,
	})
	if err != nil {
		return nil, err
	}
	content.URL = resp.ContentURI.CUString()
	return content, nil
}

// DownloadMedia fetches a file from the backend.
func (b *Backend) DownloadMedia(ctx context.Context, params rpc.DownloadMediaParams) (*http.Response, error) {
	return b.hc.Client.Download(ctx, params.MXC)
}

// MediaURL is the backend's address for a file. It needs the access token, so a browser can't open it yet.
func (b *Backend) MediaURL(mxc id.ContentURI, _ url.Values) string {
	return b.hc.Client.BuildURL(mautrix.ClientURLPath{"v1", "media", "download", mxc.Homeserver, mxc.FileID})
}

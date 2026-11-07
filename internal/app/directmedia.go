package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/mediaproxy"

	"github.com/whyyor/tuimessenger/internal/media"
)

// mediaCacheSize is how much recently opened direct media is kept on disk.
const mediaCacheSize = 512 << 20

// FetchMedia returns a direct media file, downloading it from its network the first time it's opened.
func (a *App) FetchMedia(ctx context.Context, fileID string, params map[string]string) (*os.File, string, error) {
	if file, contentType, ok := a.mediaCache.Get(fileID); ok {
		return file, contentType, nil
	}
	if _, err, _ := a.downloads.Do(fileID, func() (any, error) {
		return nil, a.download(ctx, fileID, params)
	}); err != nil {
		return nil, "", err
	}
	file, contentType, ok := a.mediaCache.Get(fileID)
	if !ok {
		return nil, "", errors.New("downloaded media went missing from the cache")
	}
	return file, contentType, nil
}

func (a *App) download(ctx context.Context, fileID string, params map[string]string) error {
	network, mediaID, ok := media.ParseDirectID(fileID)
	br := a.Bridge(network)
	if !ok || br == nil {
		return mautrix.MNotFound.WithMessage("Unknown media")
	}
	dmn, ok := br.Network.(bridgev2.DirectMediableNetwork)
	if !ok {
		return mautrix.MNotFound.WithMessage("%s doesn't support opening media later", network)
	}
	resp, err := dmn.Download(ctx, networkid.MediaID(mediaID), params)
	if err != nil {
		return fmt.Errorf("%s couldn't download the media: %w", network, err)
	}
	tmp, err := a.mediaCache.TempFile()
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // a no-op once Put has moved it into the cache
	contentType, err := writeMediaResponse(ctx, resp, tmp)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return a.mediaCache.Put(fileID, tmp.Name(), contentType)
}

// writeMediaResponse saves any of the four kinds of answer a network can give into file, returning the content
// type if the network knows it.
func writeMediaResponse(ctx context.Context, resp mediaproxy.GetMediaResponse, file *os.File) (string, error) {
	switch r := resp.(type) {
	case *mediaproxy.GetMediaResponseURL:
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.URL, nil)
		if err != nil {
			return "", err
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", err
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return "", fmt.Errorf("media download returned HTTP %d", res.StatusCode)
		}
		_, err = io.Copy(file, res.Body)
		return res.Header.Get("Content-Type"), err
	case *mediaproxy.GetMediaResponseFile:
		meta, err := r.Callback(file)
		if err != nil {
			return "", err
		}
		if meta.ReplacementFile != "" {
			// The network wrote the result to another file (e.g. after converting it); copy that in instead.
			if err = copyInto(file, meta.ReplacementFile); err != nil {
				return "", err
			}
		}
		return meta.ContentType, nil
	case mediaproxy.GetMediaResponseWriter:
		if data, ok := r.(*mediaproxy.GetMediaResponseData); ok {
			defer data.Reader.Close()
		}
		_, err := r.WriteTo(file)
		return r.GetContentType(), err
	default:
		return "", fmt.Errorf("unsupported media response %T", resp)
	}
}

func copyInto(file *os.File, path string) error {
	defer os.Remove(path)
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()
	if err = file.Truncate(0); err != nil {
		return err
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err = io.Copy(file, src)
	return err
}

package csapi

import (
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/media"
)

const maxUploadSize = 100 << 20

func (s *Server) mediaConfig(r *http.Request) (any, error) {
	return map[string]int64{"m.upload.size": maxUploadSize}, nil
}

// upload streams a file from the client into the media store and returns its mxc:// URI.
func (s *Server) upload(r *http.Request) (any, error) {
	tmp, err := s.Media.TempFile()
	if err != nil {
		return nil, err
	}
	_, err = io.Copy(tmp, io.LimitReader(r.Body, maxUploadSize+1))
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		if info, statErr := os.Stat(tmp.Name()); statErr == nil && info.Size() > maxUploadSize {
			err = mautrix.MTooLarge.WithMessage("File is too large")
		}
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return nil, err
	}
	mediaID, err := s.Media.SaveFile(tmp.Name())
	if err != nil {
		return nil, err
	}
	return &mautrix.RespMediaUpload{ContentURI: id.ContentURI{Homeserver: s.ServerName, FileID: mediaID}}, nil
}

// download serves a stored file. Thumbnails get the original file too; clients scale it themselves.
func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("server") != s.ServerName {
		writeError(w, mautrix.MNotFound.WithMessage("Media from other servers isn't available"))
		return
	}
	file, err := s.Media.Open(r.PathValue("mediaID"))
	if errors.Is(err, media.ErrInvalidID) || errors.Is(err, os.ErrNotExist) {
		writeError(w, mautrix.MNotFound.WithMessage("Media not found"))
		return
	} else if err != nil {
		s.writeErr(w, r, err)
		return
	}
	defer file.Close()
	name := r.PathValue("fileName")
	if name == "" {
		name = r.PathValue("mediaID")
	}
	// A file's ID is its hash, so its content never changes and clients may cache it forever.
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	http.ServeContent(w, r, name, time.Time{}, file)
}

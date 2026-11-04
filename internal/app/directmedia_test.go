package app

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"maunium.net/go/mautrix/mediaproxy"
)

func TestWriteMediaResponse(t *testing.T) {
	ctx := context.Background()
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = io.WriteString(w, "from-url")
	}))
	defer cdn.Close()
	replacement := filepath.Join(t.TempDir(), "converted")

	cases := map[string]struct {
		resp     mediaproxy.GetMediaResponse
		wantData string
		wantType string
	}{
		"url":  {&mediaproxy.GetMediaResponseURL{URL: cdn.URL}, "from-url", "video/mp4"},
		"data": {&mediaproxy.GetMediaResponseData{Reader: io.NopCloser(bytes.NewReader([]byte("from-data"))), ContentType: "image/png"}, "from-data", "image/png"},
		"callback": {&mediaproxy.GetMediaResponseCallback{ContentType: "audio/ogg", Callback: func(w io.Writer) (int64, error) {
			n, err := io.WriteString(w, "from-callback")
			return int64(n), err
		}}, "from-callback", "audio/ogg"},
		"file": {&mediaproxy.GetMediaResponseFile{Callback: func(f *os.File) (*mediaproxy.FileMeta, error) {
			_, err := f.WriteString("from-file")
			return &mediaproxy.FileMeta{ContentType: "image/webp"}, err
		}}, "from-file", "image/webp"},
		"replacement file": {&mediaproxy.GetMediaResponseFile{Callback: func(f *os.File) (*mediaproxy.FileMeta, error) {
			_, _ = f.WriteString("original, too long")
			return &mediaproxy.FileMeta{ReplacementFile: replacement}, os.WriteFile(replacement, []byte("converted"), 0o600)
		}}, "converted", ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			file, err := os.CreateTemp(t.TempDir(), "media-*")
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			contentType, err := writeMediaResponse(ctx, tc.resp, file)
			if err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(file.Name())
			if string(data) != tc.wantData || contentType != tc.wantType {
				t.Errorf("got %q as %q, want %q as %q", data, contentType, tc.wantData, tc.wantType)
			}
		})
	}
}

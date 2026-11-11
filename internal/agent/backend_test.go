package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// fakeHomeserver answers the three calls a send makes: media upload, send, and the messages that hold the delivery report.
type fakeHomeserver struct {
	mu          sync.Mutex
	uploadType  string
	uploadName  string
	uploadBody  []byte
	sentContent map[string]any
}

func (f *fakeHomeserver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/media/v3/upload"):
		f.uploadType, f.uploadName = r.Header.Get("Content-Type"), r.URL.Query().Get("filename")
		f.uploadBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"content_uri":"mxc://tuimessenger.localhost/abc"}`))
	case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/send/m.room.message/"):
		_ = json.NewDecoder(r.Body).Decode(&f.sentContent)
		_, _ = w.Write([]byte(`{"event_id":"$sent"}`))
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/messages"):
		_, _ = w.Write([]byte(`{"chunk":[{"type":"com.beeper.message_send_status","event_id":"$status","sender":"@slackbot:x",
			"origin_server_ts":1,"content":{"m.relates_to":{"rel_type":"m.reference","event_id":"$sent"},"status":"SUCCESS"}}]}`))
	default:
		http.NotFound(w, r)
	}
}

func fakeBackend(t *testing.T) (*Backend, *fakeHomeserver) {
	t.Helper()
	hs := &fakeHomeserver{}
	srv := httptest.NewServer(hs)
	t.Cleanup(srv.Close)
	cli, err := mautrix.NewClient(srv.URL, "@me:tuimessenger.localhost", "token")
	if err != nil {
		t.Fatal(err)
	}
	cli.DefaultHTTPRetries = 0
	return &Backend{cli: cli}, hs
}

// A file goes to the backend's media store, then as a message pointing at it, with the caption as its body.
func TestSendFile(t *testing.T) {
	b, hs := fakeBackend(t)
	raw := []byte("%PDF-1.4\nnot really a pdf\n")
	res, err := b.SendFile(context.Background(), id.RoomID("!dm:x"), "notes.pdf", raw, "here you go", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "delivered" || res.ID != "$sent" {
		t.Errorf("result: %+v", res)
	}
	hs.mu.Lock()
	defer hs.mu.Unlock()
	if hs.uploadType != "application/pdf" || hs.uploadName != "notes.pdf" || string(hs.uploadBody) != string(raw) {
		t.Errorf("upload: type=%q name=%q body=%q", hs.uploadType, hs.uploadName, hs.uploadBody)
	}
	c := hs.sentContent
	if c["msgtype"] != "m.file" || c["url"] != "mxc://tuimessenger.localhost/abc" || c["filename"] != "notes.pdf" || c["body"] != "here you go" {
		t.Errorf("sent content: %v", c)
	}
	if info, _ := c["info"].(map[string]any); info["mimetype"] != "application/pdf" || info["size"] != float64(len(raw)) {
		t.Errorf("info: %v", c["info"])
	}
}

// Without a caption the body is the file name, which the bridges read as "no caption".
func TestSendFileWithoutCaption(t *testing.T) {
	b, hs := fakeBackend(t)
	if _, err := b.SendFile(context.Background(), "!dm:x", "app.apk", []byte("PK\x03\x04"), "", time.Second); err != nil {
		t.Fatal(err)
	}
	hs.mu.Lock()
	defer hs.mu.Unlock()
	if hs.sentContent["body"] != "app.apk" || hs.sentContent["filename"] != "app.apk" {
		t.Errorf("sent content: %v", hs.sentContent)
	}
}

func TestSendFileNeedsLogin(t *testing.T) {
	b, _ := fakeBackend(t)
	b.cli.AccessToken = ""
	if _, err := b.SendFile(context.Background(), "!dm:x", "a.txt", []byte("a"), "", time.Second); err != ErrNotLoggedIn {
		t.Errorf("got %v, want ErrNotLoggedIn", err)
	}
}

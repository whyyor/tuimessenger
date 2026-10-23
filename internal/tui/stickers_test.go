package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"go.mau.fi/mauview"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/tui/config"
	"go.mau.fi/gomuks/tui/messages"
)

const receivedSticker = `{"body":"","file":{"url":"mxc://local.beeper.com/abc","v":"v2","key":{"k":"secret","alg":"A256CTR","kty":"oct","ext":true,"key_ops":["encrypt","decrypt"]},"iv":"x","hashes":{"sha256":"y"}},` +
	`"info":{"mimetype":"image/webp","w":190,"h":190,"size":39806},"m.relates_to":{"m.in_reply_to":{"event_id":"$old"}},"m.mentions":{}}`

func TestStickerContent(t *testing.T) {
	c, err := stickerContent(json.RawMessage(receivedSticker))
	if err != nil {
		t.Fatal(err)
	}
	if c.MsgType != "m.sticker" || c.File == nil || c.File.URL != "mxc://local.beeper.com/abc" || c.File.Key.Key != "secret" {
		t.Errorf("same image and key, sent as a sticker: %+v", c)
	}
	if c.RelatesTo != nil || c.Mentions != nil || c.Body == "" || c.Info.MimeType != "image/webp" {
		t.Errorf("no old reply/mentions, a body, info kept: %+v", c)
	}
}

func TestCopyPasteSendSticker(t *testing.T) {
	var clip string
	origClip := writeClipboard
	writeClipboard = func(text, _ string) error { clip = text; return nil }
	defer func() { writeClipboard = origClip }()

	var mu sync.Mutex
	var stickers, files, texts []string
	done := make(chan struct{}, 8)
	origSticker, origFile, origText := sendSticker, sendFile, sendText
	sendSticker = func(_ *RoomView, raw json.RawMessage, _ *event.RelatesTo) {
		mu.Lock()
		stickers = append(stickers, string(raw))
		mu.Unlock()
		done <- struct{}{}
	}
	sendFile = func(_ *RoomView, path, caption string, _ *event.RelatesTo) {
		mu.Lock()
		files = append(files, filepath.Base(path)+"|"+caption)
		mu.Unlock()
		done <- struct{}{}
	}
	sendText = func(_ *RoomView, text string) {
		mu.Lock()
		texts = append(texts, text)
		mu.Unlock()
		done <- struct{}{}
	}
	defer func() { sendSticker, sendFile, sendText = origSticker, origFile, origText }()

	_, room := bubbleView(t)
	cfg := &config.Config{}
	cfg.LoadKeybindings()
	view := &RoomView{Room: room, config: cfg, input: mauview.NewInputArea()}

	// y on a sticker: token on the clipboard.
	sticker := &messages.UIMessage{Event: &database.Event{ID: "$stk:x", Type: event.EventSticker.Type, Content: json.RawMessage(receivedSticker)}}
	view.CopySticker(sticker)
	if clip != stickerTokenPrefix+"$stk:x" {
		t.Fatalf("clipboard %q", clip)
	}
	// Pasting it in a chat makes a sticker chip, not text.
	view.OnPasteEvent(fakePaste{text: clip})
	if items := view.Attachments(); len(items) != 1 || items[0].sticker == nil || view.input.GetText() != "" {
		t.Fatalf("chips %+v, text %q", items, view.input.GetText())
	}
	// Only stickers: the typed text follows as its own message.
	view.InputSubmit("lol")
	wait := func(n int) {
		for range n {
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("not sent")
			}
		}
	}
	wait(2)
	mu.Lock()
	if len(stickers) != 1 || len(texts) != 1 || texts[0] != "lol" {
		t.Errorf("stickers %d, texts %q", len(stickers), texts)
	}
	mu.Unlock()

	// Sticker plus a file: the caption goes on the file.
	path := filepath.Join(t.TempDir(), "a.pdf")
	_ = os.WriteFile(path, []byte("x"), 0600)
	view.AttachSticker(json.RawMessage(receivedSticker))
	view.AttachFiles(path)
	view.InputSubmit("docs")
	wait(2)
	mu.Lock()
	defer mu.Unlock()
	if len(files) != 1 || files[0] != "a.pdf|docs" || len(texts) != 1 {
		t.Errorf("files %q texts %q", files, texts)
	}
	// Unknown tokens are just text.
	if _, ok := stickerFromToken(stickerTokenPrefix + "$nope"); ok {
		t.Error("an unknown token isn't a sticker")
	}
}

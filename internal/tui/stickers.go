package tui

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"maunium.net/go/mautrix/event"

	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
	"go.mau.fi/gomuks/tui/debug"
	"go.mau.fi/gomuks/tui/messages"
)

// stickerTokenPrefix marks a copied sticker on the clipboard; pasting the
// token into any chat re-sends the sticker, like forwarding it in Beeper.
const stickerTokenPrefix = "gomuks-sticker:"

var copiedStickers = struct {
	sync.Mutex
	byToken map[string]json.RawMessage
}{byToken: make(map[string]json.RawMessage)}

func isSticker(msg *messages.UIMessage) bool {
	return msg != nil && msg.Event != nil && msg.Event.GetType() == event.EventSticker
}

// CopySticker remembers the sticker and puts its token on the clipboard.
func (view *RoomView) CopySticker(msg *messages.UIMessage) {
	token := stickerTokenPrefix + string(msg.ID)
	copiedStickers.Lock()
	copiedStickers.byToken[token] = json.RawMessage(msg.Event.GetContent())
	copiedStickers.Unlock()
	if err := writeClipboard(token, "clipboard"); err != nil {
		debug.Print("Failed to copy sticker token:", err)
	}
	view.AddServiceMessage("Sticker copied: paste it in any chat to send it")
}

// stickerFromToken returns the copied sticker a pasted text stands for.
func stickerFromToken(text string) (json.RawMessage, bool) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, stickerTokenPrefix) {
		return nil, false
	}
	copiedStickers.Lock()
	defer copiedStickers.Unlock()
	raw, ok := copiedStickers.byToken[text]
	return raw, ok
}

// stickerContent turns a received sticker's content into one to send: same
// image (and decryption key), no relations, mentions or edit leftovers.
func stickerContent(raw json.RawMessage) (*event.MessageEventContent, error) {
	var content event.MessageEventContent
	if err := json.Unmarshal(raw, &content); err != nil {
		return nil, err
	}
	content.MsgType = "m.sticker" // the backend sends this as an m.sticker event
	content.RelatesTo = nil
	content.Mentions = nil
	content.NewContent = nil
	if content.Body == "" {
		content.Body = "Sticker"
	}
	return &content, nil
}

// sendSticker is swapped out in tests.
var sendSticker = func(view *RoomView, raw json.RawMessage, relatesTo *event.RelatesTo) {
	defer debug.Recover()
	content, err := stickerContent(raw)
	if err != nil {
		view.AddServiceMessage("Failed to read the copied sticker: %v", err)
		view.requestRender()
		return
	}
	err = view.parent.matrix.SendMessage(context.TODO(), &jsoncmd.SendMessageParams{
		RoomID:      view.Room.ID,
		BaseContent: content,
		RelatesTo:   relatesTo,
	})
	if err != nil {
		view.AddServiceMessage("Failed to send sticker: %v", err)
	}
	view.requestRender()
}

// AttachSticker adds a copied sticker as a chip; Enter sends it.
func (view *RoomView) AttachSticker(raw json.RawMessage) {
	view.attached.Lock()
	view.attached.items = append(view.attached.items, pendingFile{sticker: raw})
	view.attached.Unlock()
	view.requestRender()
}

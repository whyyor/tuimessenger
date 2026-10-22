package tui

import (
	"context"
	"encoding/json"
	"strings"

	"maunium.net/go/mautrix/event"

	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
	"go.mau.fi/gomuks/pkg/rpc/store"
	"go.mau.fi/gomuks/tui/debug"
)

// AccountDataPrivacy holds per-chat opt-ins to share typing and "seen";
// private room account data, so it follows the account to every device.
var AccountDataPrivacy = event.Type{Type: "com.whyyor.gomuks.privacy", Class: event.AccountDataEventType}

// ChatPrivacy is what we share in one chat. Both default to off.
type ChatPrivacy struct {
	Typing       bool `json:"typing,omitempty"`
	ReadReceipts bool `json:"read_receipts,omitempty"`
}

func chatPrivacy(room *store.RoomStore) ChatPrivacy {
	var p ChatPrivacy
	if room == nil {
		return p
	}
	if ad := room.GetAccountData(AccountDataPrivacy); ad != nil {
		_ = json.Unmarshal(ad.Content, &p)
	}
	return p
}

// receiptTypeFor is public m.read only in chats that opted into "seen".
func receiptTypeFor(room *store.RoomStore) event.ReceiptType {
	if chatPrivacy(room).ReadReceipts {
		return event.ReceiptTypeRead
	}
	return event.ReceiptTypeReadPrivate
}

// parseToggle reads on/off; anything else (or nothing) flips current.
func parseToggle(arg string, current bool) bool {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "on", "yes", "true", "1":
		return true
	case "off", "no", "false", "0":
		return false
	}
	return !current
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// SetPrivacy changes what this chat shares and saves it to the account.
func (view *RoomView) SetPrivacy(update func(*ChatPrivacy)) {
	defer debug.Recover()
	p := chatPrivacy(view.Room)
	before := p
	update(&p)
	raw, _ := json.Marshal(&p)
	err := view.parent.matrix.SetAccountData(context.TODO(), &jsoncmd.SetAccountDataParams{
		RoomID:  view.Room.ID,
		Type:    AccountDataPrivacy.Type,
		Content: raw,
	})
	switch {
	case err != nil:
		view.AddServiceMessage("Failed to save chat privacy: %v", err)
	case p.Typing != before.Typing:
		view.AddServiceMessage("Typing in this chat: %s", onOff(p.Typing))
		if !p.Typing {
			view.SendTyping(false)
		}
	case p.ReadReceipts != before.ReadReceipts:
		view.AddServiceMessage("Seen in this chat: %s", onOff(p.ReadReceipts))
		// Re-send the read position so the new receipt type takes effect.
		view.Room.ForgetMarkedRead()
	default:
		view.AddServiceMessage("Typing in this chat: %s · seen: %s", onOff(p.Typing), onOff(p.ReadReceipts))
	}
	view.requestRender()
}

// typingNames is who's typing in the room, by display name, minus us.
func (view *RoomView) typingNames() []string {
	var names []string
	own := view.Room.OwnUserID()
	for _, userID := range view.Room.Typing.Current() {
		if userID != own {
			names = append(names, view.Room.GetDisplayname(userID))
		}
	}
	return names
}

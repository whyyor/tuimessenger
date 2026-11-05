package tui

import (
	"context"
	"encoding/json"

	"maunium.net/go/mautrix/event"

	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
	"go.mau.fi/gomuks/pkg/rpc/store"
	"go.mau.fi/gomuks/tui/debug"
)

// AccountDataChatView holds per-chat display choices that only affect gomuks;
// private room account data, so it syncs and the network never sees it.
var AccountDataChatView = event.Type{Type: "com.whyyor.gomuks.view", Class: event.AccountDataEventType}

// ChatView is how gomuks shows one chat. HideDeleted leaves deleted messages
// out of the timeline (nothing is removed anywhere).
type ChatView struct {
	HideDeleted bool `json:"hide_deleted,omitempty"`
}

func chatView(room *store.RoomStore) ChatView {
	var v ChatView
	if room == nil {
		return v
	}
	if ad := room.GetAccountData(AccountDataChatView); ad != nil {
		_ = json.Unmarshal(ad.Content, &v)
	}
	return v
}

// hiddenAsDeleted reports whether the chat's view hides this event.
func hiddenAsDeleted(evt *database.Event, v ChatView) bool {
	return v.HideDeleted && evt.RedactedBy != ""
}

// SetHideDeleted turns hiding deleted messages in this chat on or off.
func (view *RoomView) SetHideDeleted(hide bool) {
	defer debug.Recover()
	v := chatView(view.Room)
	v.HideDeleted = hide
	raw, _ := json.Marshal(&v)
	err := view.parent.matrix.SetAccountData(context.TODO(), &jsoncmd.SetAccountDataParams{
		RoomID: view.Room.ID, Type: AccountDataChatView.Type, Content: raw,
	})
	switch {
	case err != nil:
		view.AddServiceMessage("Failed to save: %v", err)
	case hide:
		view.AddServiceMessage("Deleted messages in this chat: hidden")
	default:
		view.AddServiceMessage("Deleted messages in this chat: shown")
	}
	RequestRepaint()
}

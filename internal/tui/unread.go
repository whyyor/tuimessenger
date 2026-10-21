package tui

import (
	"context"
	"encoding/json"

	"maunium.net/go/mautrix/event"

	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
	"go.mau.fi/gomuks/pkg/rpc/store"
	"go.mau.fi/gomuks/tui/debug"
)

// MarkUnread flags the open chat unread (synced to the network by bridges)
// and closes it, since any key in an open chat would read it again.
func (view *MainView) MarkUnread() {
	room := view.currentRoom
	if room == nil {
		return
	}
	view.browsing = false
	room.Room.ForgetMarkedRead()
	view.CloseCurrentRoom()
	go setMarkedUnread(view, room.Room)
}

func setMarkedUnread(view *MainView, room *store.RoomStore) {
	defer debug.Recover()
	raw, _ := json.Marshal(&event.MarkedUnreadEventContent{Unread: true})
	err := view.matrix.SetAccountData(context.TODO(), &jsoncmd.SetAccountDataParams{
		RoomID:  room.ID,
		Type:    event.AccountDataMarkedUnread.Type,
		Content: raw,
	})
	if err != nil {
		debug.Print("Failed to mark", room.ID, "unread:", err)
	}
	view.parent.Render()
}

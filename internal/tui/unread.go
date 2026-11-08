package tui

import (
	"context"
	"encoding/json"

	"maunium.net/go/mautrix/event"

	"github.com/whyyor/tuimessenger/internal/hicli/jsoncmd"
	"github.com/whyyor/tuimessenger/internal/rpc/store"
	"github.com/whyyor/tuimessenger/internal/tui/debug"
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
	if err := writeMarkedUnread(view.matrix, room); err != nil {
		debug.Print("Failed to mark", room.ID, "unread:", err)
		toasts.Show(toastError, "Couldn't mark it unread: "+err.Error())
	}
	view.parent.Render()
}

// writeMarkedUnread forgets the chat's local read first (else it would hide the flag again), then sets the flag.
func writeMarkedUnread(api accountDataSetter, room *store.RoomStore) error {
	if seenMarkerOf(room) != (seenMarker{}) {
		err := api.SetAccountData(context.TODO(), &jsoncmd.SetAccountDataParams{
			RoomID: room.ID, Type: AccountDataSeen.Type, Content: json.RawMessage(`{}`),
		})
		if err != nil {
			return err
		}
	}
	raw, _ := json.Marshal(&event.MarkedUnreadEventContent{Unread: true})
	return api.SetAccountData(context.TODO(), &jsoncmd.SetAccountDataParams{
		RoomID: room.ID, Type: event.AccountDataMarkedUnread.Type, Content: raw,
	})
}

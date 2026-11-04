package tui

import (
	"context"
	"sync"
	"time"

	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
	"go.mau.fi/gomuks/tui/debug"
)

// viewingRefresh is how often the open chat is re-sent while you're looking at it; the server forgets after 2 min.
const viewingRefresh = 30 * time.Second

// viewingReporter tells the server which chat you're looking at (tuimessenger only; other servers ignore it), so
// WhatsApp shows you online, and sends typing notices, only while a chat is really open on screen.
type viewingReporter struct {
	lock     sync.Mutex
	last     id.RoomID
	lastSent time.Time
}

var viewing = &viewingReporter{}

// reportViewing sends the open chat if it changed, or refreshes it when the last report is getting old.
func (view *MainView) reportViewing() {
	var roomID id.RoomID
	if room := view.currentRoom; room != nil && userAttention.looking(time.Now()) {
		roomID = room.Room.ID
	}
	viewing.lock.Lock()
	unchanged := roomID == viewing.last
	if unchanged && (roomID == "" || time.Since(viewing.lastSent) < viewingRefresh) {
		viewing.lock.Unlock()
		return
	}
	viewing.last, viewing.lastSent = roomID, time.Now()
	viewing.lock.Unlock()
	go func() {
		defer debug.Recover()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := view.matrix.SetViewingRoom(ctx, &jsoncmd.SetViewingRoomParams{RoomID: roomID}); err != nil {
			debug.Print("Failed to report viewed room:", err)
		}
	}()
}

func (view *MainView) tickViewing() {
	for range time.Tick(viewingRefresh / 2) {
		view.reportViewing()
	}
}

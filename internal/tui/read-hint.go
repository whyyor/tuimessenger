package tui

import (
	"strings"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"go.mau.fi/mauview"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/pkg/rpc/store"
	"go.mau.fi/gomuks/tui/widget"
)

const readHint = "ctrl+m mark as read"

// publicReads is, per chat, the timestamp (ms) of the newest message we've
// publicly read (m.read): what the other side and WhatsApp see.
var publicReads = struct {
	sync.Mutex
	at map[id.RoomID]int64
}{at: make(map[id.RoomID]int64)}

func notePublicRead(roomID id.RoomID, ts int64) {
	publicReads.Lock()
	defer publicReads.Unlock()
	if ts > publicReads.at[roomID] {
		publicReads.at[roomID] = ts
	}
}

func publicReadAt(roomID id.RoomID) int64 {
	publicReads.Lock()
	defer publicReads.Unlock()
	return publicReads.at[roomID]
}

// notePublicReadOf records a public read we just sent for evtID.
func notePublicReadOf(room *store.RoomStore, evtID id.EventID) {
	if evt := room.GetEventByID(evtID); evt != nil {
		notePublicRead(room.ID, evt.Timestamp.UnixMilli())
	}
}

// notePublicReceipts records our own m.read receipts among receipts.
func notePublicReceipts(room *store.RoomStore, receipts map[id.EventID][]*database.Receipt) {
	own := room.OwnUserID()
	for evtID, list := range receipts {
		for _, r := range list {
			if r.UserID != own || r.ReceiptType != event.ReceiptTypeRead {
				continue
			}
			if evt := room.GetEventByID(evtID); evt != nil {
				notePublicRead(room.ID, evt.Timestamp.UnixMilli())
			}
		}
	}
}

// newestPeerMessage is the timestamp (ms) of the other side's latest message,
// or 0 if the newest message is ours: replying shows we've seen it.
func newestPeerMessage(room *store.RoomStore) int64 {
	tl := room.TimelineCache.Current()
	if tl == nil {
		return 0
	}
	own := room.OwnUserID()
	for i := len(*tl) - 1; i >= 0; i-- {
		evt := (*tl)[i]
		if evt == nil || !strings.HasPrefix(string(evt.ID), "$") {
			continue
		}
		switch evt.GetType().Type {
		case event.EventMessage.Type, event.EventSticker.Type:
		default:
			continue
		}
		switch {
		case evt.Sender == own:
			return 0
		case isPeer(evt.Sender, own):
			return evt.Timestamp.UnixMilli()
		}
	}
	return 0
}

// needsReadHint: /seen-off chat whose newest message from them hasn't had a
// public read yet. /seen-on chats read publicly by themselves.
func (view *RoomView) needsReadHint() bool {
	if receiptTypeFor(view.Room) == event.ReceiptTypeRead {
		return false
	}
	newest := newestPeerMessage(view.Room)
	return newest > 0 && newest > publicReadAt(view.Room.ID)
}

// drawReadHint centres the hint in the status row.
func (view *RoomView) drawReadHint(screen mauview.Screen, y, width int) {
	if !view.needsReadHint() {
		return
	}
	w := runewidth.StringWidth(readHint)
	x := (width - w) / 2
	if x < runewidth.StringWidth(view.GetStatus())+3 {
		return // the status line (typing, replying…) needs the room
	}
	widget.WriteLine(screen, mauview.AlignLeft, readHint, x, y, w,
		tcell.StyleDefault.Foreground(ColorBorder))
}

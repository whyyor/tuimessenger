package tui

import (
	"strings"
	"sync"

	"github.com/mattn/go-runewidth"
	"go.mau.fi/mauview"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/pkg/rpc/store"
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

// newestPeerMessage is the other side's latest message, or nil if the newest
// message is ours: replying shows we've seen it.
func newestPeerMessage(room *store.RoomStore) *database.Event {
	tl := room.TimelineCache.Current()
	if tl == nil {
		return nil
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
			return nil
		case isPeer(evt.Sender, own):
			return evt
		}
	}
	return nil
}

// reactedTo caches whether we reacted to a message, keyed by its reaction
// total so a new reaction (ours from any device) triggers a fresh check.
var reactedTo = struct {
	sync.Mutex
	byEvent map[id.EventID]reactionCheck
}{byEvent: make(map[id.EventID]reactionCheck)}

type reactionCheck struct {
	total    int
	mine     bool
	inFlight bool
}

func reactionTotal(evt *database.Event) int {
	total := 0
	for _, n := range evt.Reactions {
		total += n
	}
	return total
}

// lookupOwnReaction is swapped out in tests.
var lookupOwnReaction = func(view *RoomView, evtID id.EventID) bool {
	return len(view.OwnReactions(evtID)) > 0
}

// weReactedTo answers from the cache, starting a background lookup when the
// message's reactions changed; until then it says no (hint stays).
func (view *RoomView) weReactedTo(evt *database.Event) bool {
	total := reactionTotal(evt)
	if total == 0 {
		return false
	}
	reactedTo.Lock()
	check, ok := reactedTo.byEvent[evt.ID]
	stale := !ok || check.total != total
	if stale && !check.inFlight {
		reactedTo.byEvent[evt.ID] = reactionCheck{total: total, mine: check.mine, inFlight: true}
		go func() {
			mine := lookupOwnReaction(view, evt.ID)
			reactedTo.Lock()
			reactedTo.byEvent[evt.ID] = reactionCheck{total: total, mine: mine}
			reactedTo.Unlock()
			view.requestRender()
		}()
	}
	reactedTo.Unlock()
	return ok && check.mine && !stale
}

// needsReadHint: /seen-off chat whose newest message is theirs, without a
// public read or our reaction. /seen-on chats read publicly by themselves.
func (view *RoomView) needsReadHint() bool {
	if receiptTypeFor(view.Room) == event.ReceiptTypeRead && !view.peek {
		return false
	}
	newest := newestPeerMessage(view.Room)
	if newest == nil || newest.Timestamp.UnixMilli() <= publicReadAt(view.Room.ID) {
		return false
	}
	// Reacting to it shows we've seen it, just like replying.
	return !view.weReactedTo(newest)
}

// drawReadHint centres the hint in the status row.
func (view *RoomView) drawReadHint(screen mauview.Screen, y, width int) {
	var hint []pill
	if view.peek {
		hint = append(hint, pill{icon: "\U000f0208", label: "peek · not sending seen", color: ColorPillViolet, dim: true})
	}
	if view.needsReadHint() {
		hint = append(hint, pill{icon: "\uf00c", label: readHint, color: ColorPillYellow, dim: true})
	}
	if len(hint) == 0 {
		return
	}
	w := pillsWidth(hint)
	x := (width - w) / 2
	if x < runewidth.StringWidth(view.GetStatus())+3 {
		return // the status line (typing, replying…) needs the room
	}
	drawPills(screen, x, y, hint)
}

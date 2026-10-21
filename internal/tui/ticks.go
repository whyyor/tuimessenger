package tui

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/tidwall/gjson"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
	"go.mau.fi/gomuks/tui/debug"
	"go.mau.fi/gomuks/tui/messages"
)

// ColorReadTick is WhatsApp's read-receipt blue.
var ColorReadTick = tcell.NewHexColor(0x53bdeb)

const (
	tickWidth = 2
	// Undelivered messages younger than this are rechecked every tickRecheck.
	tickRecheckWindow = 5 * time.Minute
	tickRecheck       = 3 * time.Second
)

type deliveryState int

const (
	deliveryUnknown deliveryState = iota
	deliverySent
	deliveryDelivered
	deliveryFailed
)

type deliveryEntry struct {
	state    deliveryState
	checked  time.Time
	inFlight bool
}

// deliveryCache remembers bridge send statuses (com.beeper.message_send_status).
type deliveryCache struct {
	lock    sync.Mutex
	entries map[id.EventID]*deliveryEntry
}

var deliveries = &deliveryCache{entries: make(map[id.EventID]*deliveryEntry)}

// parseDelivery reads the bridge's status events for one message.
func parseDelivery(evts []*database.Event) deliveryState {
	state := deliverySent
	var latest int64
	for _, evt := range evts {
		switch gjson.GetBytes(evt.GetContent(), "status").Str {
		case "SUCCESS":
			return deliveryDelivered
		case "FAIL_PERMANENT":
			if ts := evt.Timestamp.UnixMilli(); ts >= latest {
				latest, state = ts, deliveryFailed
			}
		}
	}
	return state
}

// state returns the cached status, fetching it in the background when
// unknown or when a fresh message may still get delivered.
func (view *MessageView) deliveryState(msg *messages.UIMessage) deliveryState {
	if msg.ID == "" || strings.HasPrefix(string(msg.ID), "~") {
		return deliveryUnknown
	}
	deliveries.lock.Lock()
	entry, ok := deliveries.entries[msg.ID]
	if !ok {
		entry = &deliveryEntry{}
		deliveries.entries[msg.ID] = entry
	}
	recheck := entry.state != deliveryDelivered && entry.state != deliveryFailed &&
		time.Since(msg.Time()) < tickRecheckWindow && time.Since(entry.checked) > tickRecheck
	fetch := !entry.inFlight && (entry.state == deliveryUnknown || recheck)
	if fetch {
		entry.inFlight = true
	}
	state := entry.state
	deliveries.lock.Unlock()
	if fetch {
		go view.fetchDelivery(msg.ID, entry)
	}
	if state == deliveryUnknown {
		return deliverySent
	}
	return state
}

func (view *MessageView) fetchDelivery(eventID id.EventID, entry *deliveryEntry) {
	defer debug.Recover()
	evts, err := view.matrix.GetRelatedEvents(context.TODO(), &jsoncmd.GetRelatedEventsParams{
		RoomID:       view.parent.Room.ID,
		EventID:      eventID,
		RelationType: event.RelReference,
		EventType:    "com.beeper.message_send_status",
	})
	deliveries.lock.Lock()
	entry.inFlight = false
	entry.checked = time.Now()
	changed := false
	if err != nil {
		debug.Print("Failed to fetch send status for", eventID, err)
	} else if state := parseDelivery(evts); state != entry.state {
		entry.state, changed = state, true
	}
	deliveries.lock.Unlock()
	if changed {
		messages.RequestRender()
	}
}

// markRead sets lay.read on own messages someone else has seen: a receipt
// at or after it, or a later message from them. msgs is oldest first.
func (view *MessageView) markRead(msgs []*messages.UIMessage, layouts map[*messages.UIMessage]*bubbleLayout) {
	own := view.parent.Room.OwnUserID()
	read := false
	for i := len(msgs) - 1; i >= 0; i-- {
		msg := msgs[i]
		lay := layouts[msg]
		if lay == nil || lay.service {
			continue
		}
		if !lay.own {
			read = true
		} else if !read {
			for _, user := range view.parent.Room.ReceiptUsersAt(msg.ID) {
				if user != own && !strings.HasSuffix(user.Localpart(), "bot") {
					read = true
					break
				}
			}
		}
		if lay.own {
			lay.read = read
		}
	}
}

// tickFor picks the glyphs and colour for one of our messages.
func (view *MessageView) tickFor(msg *messages.UIMessage, lay *bubbleLayout) (string, tcell.Color) {
	switch {
	case msg.SendError != "" && msg.SendError != "not sent":
		return "!", tcell.ColorRed
	case msg.Pending:
		return "◌", ColorStatusText
	case lay.read:
		return "✓✓", ColorReadTick
	}
	switch view.deliveryState(msg) {
	case deliveryDelivered:
		return "✓✓", ColorStatusText
	case deliveryFailed:
		return "!", tcell.ColorRed
	}
	return "✓", ColorStatusText
}

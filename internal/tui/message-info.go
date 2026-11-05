package tui

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/tidwall/gjson"
	"go.mau.fi/mauview"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
	"github.com/whyyor/tuimessenger/internal/hicli/jsoncmd"
	"github.com/whyyor/tuimessenger/internal/tui/debug"
	"github.com/whyyor/tuimessenger/internal/tui/messages"
	"github.com/whyyor/tuimessenger/internal/tui/widget"
)

// infoLine is one row of the message info popup: a label (only on the first
// row of a group) and a value, optionally in a person's colour.
type infoLine struct {
	label string
	value string
	right string // dim, right-aligned (times)
	color tcell.Color
}

// MessageInfo is the i popup: when a message was sent, delivered, seen
// and who reacted.
type MessageInfo struct {
	mu      sync.Mutex
	lines   []infoLine
	loading bool
}

var networkNames = map[string]string{
	"whatsapp": "WhatsApp", "telegram": "Telegram", "instagram": "Instagram",
	"instagramgo": "Instagram", "slack": "Slack", "slackgo": "Slack", "signal": "Signal",
	"discord": "Discord", "discordgo": "Discord", "facebook": "Messenger", "metago": "Messenger",
	"linkedin": "LinkedIn", "gmessages": "Google Messages", "imessage": "iMessage", "imessagego": "iMessage",
}

func infoTime(t, now time.Time) string {
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	switch {
	case y1 == y2 && m1 == m2 && d1 == d2:
		return t.Format("15:04:05")
	case y1 == y2:
		return t.Format("Mon 2 Jan, 15:04")
	}
	return t.Format("2 Jan 2006, 15:04")
}

// seenEntry is one person who has seen the message, and when.
type seenEntry struct {
	user    id.UserID
	at      time.Time
	replied bool
}

// whoSaw works out who has seen msg: a public read receipt on it or a later
// event, or a later message from them. Only real people, earliest first.
func whoSaw(msg *database.Event, own id.UserID, later []*database.Event, receipts map[id.EventID][]*database.Receipt) []seenEntry {
	laterIDs := make(map[id.EventID]bool, len(later)+1)
	laterIDs[msg.ID] = true
	for _, evt := range later {
		laterIDs[evt.ID] = true
	}
	seen := make(map[id.UserID]seenEntry)
	note := func(e seenEntry) {
		if old, ok := seen[e.user]; !ok || e.at.Before(old.at) {
			seen[e.user] = e
		}
	}
	for evtID, list := range receipts {
		if !laterIDs[evtID] {
			continue
		}
		for _, r := range list {
			if r.ReceiptType == event.ReceiptTypeRead && isPeer(r.UserID, own) && r.UserID != msg.Sender {
				note(seenEntry{user: r.UserID, at: r.Timestamp.Time})
			}
		}
	}
	for _, evt := range later {
		if evt.Sender != msg.Sender && isPeer(evt.Sender, own) && strings.HasPrefix(string(evt.ID), "$") {
			switch evt.GetType().Type {
			case event.EventMessage.Type, event.EventSticker.Type:
				note(seenEntry{user: evt.Sender, at: evt.Timestamp.Time, replied: true})
			}
		}
	}
	out := make([]seenEntry, 0, len(seen))
	for _, e := range seen {
		out = append(out, e)
	}
	slices.SortFunc(out, func(a, b seenEntry) int { return a.at.Compare(b.at) })
	return out
}

// deliveryInfo reads the bridge's status events: when it reached the network.
func deliveryInfo(evts []*database.Event) (at time.Time, failed string) {
	for _, evt := range evts {
		content := evt.GetContent()
		switch gjson.GetBytes(content, "status").Str {
		case "SUCCESS":
			if at.IsZero() || evt.Timestamp.Time.Before(at) {
				at = evt.Timestamp.Time
			}
		case "FAIL_PERMANENT", "FAIL_RETRIABLE":
			if failed == "" {
				failed = gjson.GetBytes(content, "message").Str
				if failed == "" {
					failed = gjson.GetBytes(content, "reason").Str
				}
			}
		}
	}
	return at, failed
}

// reactionLines groups reactions by emoji: "👍 Peter · You".
func reactionLines(evts []*database.Event, own id.UserID, name func(id.UserID) string) []string {
	var order []string
	byKey := make(map[string][]string)
	for _, evt := range evts {
		if evt.RedactedBy != "" {
			continue
		}
		content, ok := evt.GetMautrixContent().Parsed.(*event.ReactionEventContent)
		if !ok || content.RelatesTo.Key == "" {
			continue
		}
		key := content.RelatesTo.Key
		who := name(evt.Sender)
		if evt.Sender == own {
			who = "You"
		}
		if _, ok := byKey[key]; !ok {
			order = append(order, key)
		}
		byKey[key] = append(byKey[key], who)
	}
	out := make([]string, 0, len(order))
	for _, key := range order {
		out = append(out, key+" "+strings.Join(byKey[key], ", "))
	}
	return out
}

// buildInfoLines assembles the popup rows from what was fetched.
func buildInfoLines(view *RoomView, msg *database.Event, now time.Time, delivered time.Time, failed string,
	seen []seenEntry, reactions []string) []infoLine {
	own := view.Room.OwnUserID()
	lines := []infoLine{{label: "Sent", value: infoTime(msg.Timestamp.Time, now)}}
	if msg.Sender != own {
		lines[0].value += " · " + view.Room.GetDisplayname(msg.Sender)
	}
	if msg.Sender == own {
		network := networkNames[view.Room.BridgeID()]
		switch {
		case failed != "":
			lines = append(lines, infoLine{label: "Delivered", value: "failed: " + failed, color: tcell.ColorRed})
		case !delivered.IsZero() && network != "":
			lines = append(lines, infoLine{label: "Delivered", value: infoTime(delivered, now) + " · " + network})
		case !delivered.IsZero():
			lines = append(lines, infoLine{label: "Delivered", value: infoTime(delivered, now)})
		case network != "":
			lines = append(lines, infoLine{label: "Delivered", value: "not yet confirmed by " + network})
		}
	}
	if len(seen) == 0 {
		lines = append(lines, infoLine{label: "Seen", value: "not yet"})
	}
	for i, s := range seen {
		label := ""
		if i == 0 {
			label = "Seen"
		}
		right := infoTime(s.at, now)
		if s.replied {
			right += " (replied)"
		}
		lines = append(lines, infoLine{label: label, value: view.Room.GetDisplayname(s.user), right: right,
			color: widget.GetHashColor(s.user)})
	}
	for i, r := range reactions {
		label := ""
		if i == 0 {
			label = "Reactions"
		}
		lines = append(lines, infoLine{label: label, value: r})
	}
	return lines
}

// fetchInfo is swapped out in tests: everything the popup needs, from the
// local database (no homeserver calls).
var fetchInfo = func(view *RoomView, msg *database.Event, later []*database.Event) (
	mss, reactions []*database.Event, receipts map[id.EventID][]*database.Receipt) {
	ctx := context.TODO()
	gmx := view.parent.matrix
	mss, _ = gmx.GetRelatedEvents(ctx, &jsoncmd.GetRelatedEventsParams{
		RoomID: view.Room.ID, EventID: msg.ID, RelationType: event.RelReference,
		EventType: "com.beeper.message_send_status",
	})
	reactions, _ = gmx.GetRelatedEvents(ctx, &jsoncmd.GetRelatedEventsParams{
		// No type filter: encrypted reactions are stored as m.room.encrypted.
		RoomID: view.Room.ID, EventID: msg.ID, RelationType: event.RelAnnotation,
	})
	ids := []id.EventID{msg.ID}
	for _, evt := range later {
		if strings.HasPrefix(string(evt.ID), "$") {
			ids = append(ids, evt.ID)
		}
	}
	receipts, err := gmx.GetReceipts(ctx, &jsoncmd.GetReceiptsParams{RoomID: view.Room.ID, EventIDs: ids})
	if err != nil {
		debug.Print("Failed to load receipts for message info:", err)
	}
	return
}

// eventsAfter returns the loaded timeline events newer than evtID.
func eventsAfter(tl []*database.Event, evtID id.EventID) []*database.Event {
	for i, evt := range tl {
		if evt != nil && evt.ID == evtID {
			return tl[i+1:]
		}
	}
	return nil
}

// OpenMessageInfo shows the popup for msg and fills it in the background.
func (view *RoomView) OpenMessageInfo(msg *messages.UIMessage) {
	if msg == nil || msg.Event == nil || msg.IsService || !strings.HasPrefix(string(msg.ID), "$") {
		return
	}
	info := &MessageInfo{loading: true}
	view.info = info
	evt := msg.Event
	var later []*database.Event
	if tl := view.Room.TimelineCache.Current(); tl != nil {
		later = slices.Clone(eventsAfter(*tl, evt.ID))
	}
	fetch := fetchInfo
	go func() {
		defer debug.Recover()
		mss, reactionEvts, receipts := fetch(view, evt, later)
		delivered, failed := deliveryInfo(mss)
		own := view.Room.OwnUserID()
		lines := buildInfoLines(view, evt, time.Now(), delivered, failed,
			whoSaw(evt, own, later, receipts),
			reactionLines(reactionEvts, own, view.Room.GetDisplayname))
		info.mu.Lock()
		info.lines, info.loading = lines, false
		info.mu.Unlock()
		view.requestRender()
	}()
}

func (view *RoomView) closeInfo() {
	view.info = nil
	RequestRepaint()
}

// onInfoKey: any of esc, q, i or enter closes the popup; other keys do nothing.
func (view *RoomView) onInfoKey(ev mauview.KeyEvent) bool {
	switch {
	case ev.Key() == tcell.KeyEscape, ev.Key() == tcell.KeyEnter,
		ev.Key() == tcell.KeyRune && (ev.Rune() == 'q' || ev.Rune() == 'i'):
		view.closeInfo()
	}
	return true
}

// Draw centres the popup in the message area.
func (info *MessageInfo) Draw(screen mauview.Screen) {
	info.mu.Lock()
	lines, loading := info.lines, info.loading
	info.mu.Unlock()
	if loading {
		lines = []infoLine{{label: "", value: "loading…"}}
	}
	const labelW = 11
	inner := 30
	for _, l := range lines {
		w := labelW + runewidth.StringWidth(l.value)
		if l.right != "" {
			w += 2 + runewidth.StringWidth(l.right)
		}
		inner = max(inner, w)
	}
	width, height := screen.Size()
	w := min(inner+4, width)
	h := min(len(lines)+3, height)
	x, y := max((width-w)/2, 0), max((height-h)/2, 0)
	drawThinBox(screen, x, y, w, h, " \uf05a Message info ")
	dim := tcell.StyleDefault.Foreground(ColorStatusText)
	for i, l := range lines {
		ly := y + 1 + i
		if ly >= y+h-2 {
			break
		}
		widget.WriteLine(screen, mauview.AlignLeft, l.label, x+2, ly, labelW, dim)
		style := tcell.StyleDefault
		if l.color != tcell.ColorDefault && l.color != 0 {
			style = style.Foreground(l.color)
		}
		valueMax := w - 4 - labelW
		if l.right != "" {
			rw := runewidth.StringWidth(l.right)
			widget.WriteLine(screen, mauview.AlignLeft, l.right, x+w-2-rw, ly, rw, dim)
			valueMax -= rw + 2
		}
		widget.WriteLine(screen, mauview.AlignLeft, runewidth.Truncate(l.value, max(valueMax, 0), "…"), x+2+labelW, ly, max(valueMax, 0), style)
	}
	widget.WriteLine(screen, mauview.AlignRight, "esc close", x+2, y+h-2, w-4, dim)
}

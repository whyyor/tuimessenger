package tui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/gjson"
	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli/jsoncmd"
)

// failureMaxAge skips failures older than this, e.g. ones replayed after a long disconnect.
const failureMaxAge = 10 * time.Minute

// sendFailure is a bridge's report that something we sent (a message, reaction, edit, delete…) didn't go through.
type sendFailure struct {
	roomID    id.RoomID
	target    id.EventID
	reason    string
	permanent bool
}

// reportedFailures remembers what was already shown: a bridge may report "retriable" and then "permanent".
var reportedFailures = struct {
	sync.Mutex
	seen map[id.EventID]bool
}{seen: make(map[id.EventID]bool)}

// sendFailures finds new failure statuses (com.beeper.message_send_status) in a live sync. Startup data and
// loaded history have no since token: those failures are old and already show as a red ! tick.
func sendFailures(evt *jsoncmd.SyncComplete, now time.Time) []sendFailure {
	if ptr.Val(evt.Since) == "" {
		return nil
	}
	var out []sendFailure
	for roomID, room := range evt.Rooms {
		for _, e := range room.Events {
			if e == nil || e.Type != "com.beeper.message_send_status" || now.Sub(e.Timestamp.Time) > failureMaxAge {
				continue
			}
			content := e.GetContent()
			status := gjson.GetBytes(content, "status").Str
			target := id.EventID(gjson.GetBytes(content, `m\.relates_to.event_id`).Str)
			if !strings.HasPrefix(status, "FAIL") || target == "" {
				continue
			}
			reason := gjson.GetBytes(content, "message").Str
			if reason == "" {
				reason = gjson.GetBytes(content, "reason").Str
			}
			out = append(out, sendFailure{roomID: roomID, target: target, reason: reason, permanent: status == "FAIL_PERMANENT"})
		}
	}
	return out
}

// firstReport is true the first time a failure for this event is seen.
func firstReport(target id.EventID) bool {
	reportedFailures.Lock()
	defer reportedFailures.Unlock()
	if reportedFailures.seen[target] {
		return false
	}
	reportedFailures.seen[target] = true
	return true
}

// reportSendFailures shows a toast for each new failure and turns the message's tick into ! right away.
func (ui *App) reportSendFailures(evt *jsoncmd.SyncComplete) {
	for _, failure := range sendFailures(evt, time.Now()) {
		if failure.permanent {
			markDeliveryFailed(failure.target)
		}
		if !firstReport(failure.target) {
			continue
		}
		toasts.Show(toastError, fmt.Sprintf("Not sent in %s: %s", ui.roomName(failure.roomID), failure.reason))
	}
}

// reportSendError shows why the backend refused one of our sends ("not sent" just means it's still on its way).
func reportSendError(evt *jsoncmd.SendComplete) {
	if evt.Event == nil || evt.Event.SendError == "" || evt.Event.SendError == "not sent" || !firstReport(evt.Event.ID) {
		return
	}
	toasts.Show(toastError, "Not sent: "+evt.Event.SendError)
}

func (ui *App) roomName(roomID id.RoomID) string {
	if room := ui.gmx.GetRoom(roomID); room != nil {
		if name := ptr.Val(room.Meta.Current().Name); name != "" {
			return name
		}
	}
	return "this chat"
}

// markDeliveryFailed records a failure in the tick cache, so the ! (and R to retry) work without a refetch.
func markDeliveryFailed(target id.EventID) {
	deliveries.lock.Lock()
	defer deliveries.lock.Unlock()
	entry := deliveries.entries[target]
	if entry == nil {
		entry = &deliveryEntry{}
		deliveries.entries[target] = entry
	}
	entry.state = deliveryFailed
	entry.checked = time.Now()
}

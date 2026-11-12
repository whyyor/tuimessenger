package tui

import (
	"errors"
	"sync"

	"github.com/whyyor/tuimessenger/internal/hicli/jsoncmd"
	"github.com/whyyor/tuimessenger/internal/rpc/store"
	"github.com/whyyor/tuimessenger/internal/tui/debug"
	"github.com/whyyor/tuimessenger/internal/tui/lib/notification"
)

// notifyQueueSize is how many notifications can wait while one is being sent; a bigger burst (e.g. after a long
// disconnect) drops the rest.
const notifyQueueSize = 64

type pendingNotification struct {
	room  *store.RoomStore
	notif jsoncmd.SyncNotification
}

// notifyQueue sends notifications one at a time, off the event loop: a slow notifier (macOS waiting for an answer
// to its permission prompt) or a slow sender lookup must not hold up incoming messages.
type notifyQueue struct {
	ch chan pendingNotification
}

func newNotifyQueue(send func(*store.RoomStore, jsoncmd.SyncNotification)) *notifyQueue {
	q := &notifyQueue{ch: make(chan pendingNotification, notifyQueueSize)}
	go func() {
		for p := range q.ch {
			func() {
				defer debug.Recover()
				send(p.room, p.notif)
			}()
		}
	}()
	return q
}

// add queues a notification and returns at once.
func (q *notifyQueue) add(room *store.RoomStore, notif jsoncmd.SyncNotification) {
	select {
	case q.ch <- pendingNotification{room, notif}:
	default:
		debug.Print("Too many notifications at once; dropping one")
	}
}

// notificationsOff are the networks whose notifier app may not show notifications, by network; they're skipped for
// the rest of the session after one hint.
var notificationsOff sync.Map

// notificationsAllowed reports whether a network's notifications are still worth sending this session.
func notificationsAllowed(source string) bool {
	_, off := notificationsOff.Load(source)
	return !off
}

// notifyFailed handles a failed notification: "not allowed" turns that network's off for the session with a hint
// saying where to turn them on; anything else is only logged.
func notifyFailed(source string, err error) {
	var na *notification.NotAuthorizedError
	if !errors.As(err, &na) {
		debug.Print("Failed to send notification:", err)
		return
	}
	if _, already := notificationsOff.LoadOrStore(source, true); already {
		return
	}
	toasts.Show(toastWarning, "Notifications are off for "+na.App+": turn them on in System Settings → Notifications")
}

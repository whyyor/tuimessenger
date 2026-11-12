package tui

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
	"github.com/whyyor/tuimessenger/internal/hicli/jsoncmd"
	"github.com/whyyor/tuimessenger/internal/rpc/store"
	"github.com/whyyor/tuimessenger/internal/tui/lib/notification"
)

func notifFor(name string) jsoncmd.SyncNotification {
	return jsoncmd.SyncNotification{Event: &database.Event{ID: id.EventID("$" + name)}}
}

// A notification that hangs (macOS waiting on its permission prompt) doesn't hold up the event loop; the rest go
// out in order once it finishes.
func TestNotifyQueueDoesntBlock(t *testing.T) {
	release := make(chan struct{})
	var mu sync.Mutex
	var sent []string
	done := make(chan struct{}, 10)
	q := newNotifyQueue(func(_ *store.RoomStore, n jsoncmd.SyncNotification) {
		<-release
		mu.Lock()
		sent = append(sent, string(n.Event.ID))
		mu.Unlock()
		done <- struct{}{}
	})
	start := time.Now()
	for _, id := range []string{"a", "b", "c"} {
		q.add(nil, notifFor(id))
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatalf("adding waited for the send: %s", time.Since(start))
	}
	close(release)
	for range 3 {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("notifications never went out")
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(sent, ",") != "$a,$b,$c" {
		t.Errorf("in order: %v", sent)
	}
}

// A burst bigger than the queue drops the extra instead of blocking.
func TestNotifyQueueOverflow(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	q := newNotifyQueue(func(*store.RoomStore, jsoncmd.SyncNotification) { <-release })
	start := time.Now()
	for range notifyQueueSize + 20 {
		q.add(nil, notifFor("x"))
	}
	if time.Since(start) > time.Second {
		t.Errorf("a burst must not block: %s", time.Since(start))
	}
}

// "Not allowed" shows one hint, naming the System Settings entry, and that network is skipped afterwards.
func TestNotifyNotAuthorized(t *testing.T) {
	ts, _, _ := fakeToasts(t)
	notificationsOff.Clear()
	t.Cleanup(notificationsOff.Clear)

	notifyFailed("slack", errors.New("exit status 1"))
	if !notificationsAllowed("slack") || current(ts) != "" {
		t.Error("other failures are only logged")
	}
	for range 3 {
		notifyFailed("whatsapp", &notification.NotAuthorizedError{App: "WhatsApp"})
	}
	if notificationsAllowed("whatsapp") {
		t.Error("whatsapp's notifications are off for the session")
	}
	if !notificationsAllowed("slack") {
		t.Error("other networks keep theirs")
	}
	if got := current(ts); got != "Notifications are off for WhatsApp: turn them on in System Settings → Notifications" {
		t.Errorf("hint: %q", got)
	}
	ts.mu.Lock()
	queued := len(ts.queue)
	ts.mu.Unlock()
	if queued != 0 {
		t.Errorf("the hint shows once: %d more queued", queued)
	}
}

package tui

import (
	"sync"
	"time"

	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
)

// activityWindow is how recent a key or click must be to count as "looking"
// when the terminal never reports focus (e.g. a multiplexer swallows it).
const activityWindow = 60 * time.Second

// attention tracks whether you're looking at gomuks, so messages arriving in
// the open chat are only read (and receipts sent) while you can see them.
type attention struct {
	mu           sync.Mutex
	focusKnown   bool
	focused      bool
	lastActivity time.Time
}

var userAttention = &attention{}

func (a *attention) setFocus(focused bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.focusKnown, a.focused = true, focused
}

func (a *attention) touch() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastActivity = time.Now()
}

// looking is focus when the terminal reports it, else recent activity.
func (a *attention) looking(now time.Time) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.focusKnown {
		return a.focused
	}
	return !a.lastActivity.IsZero() && now.Sub(a.lastActivity) < activityWindow
}

// OnFocusEvent receives terminal focus changes from the patched mauview.
func (br *bufferedRoot) OnFocusEvent(focused bool) bool {
	userAttention.setFocus(focused)
	if focused && onFocusGained != nil {
		// Coming back to the window: what's on screen now counts as seen.
		onFocusGained()
	}
	return false
}

var onFocusGained func()

// autoRead marks the open chat read if new messages arrived in it while
// you're looking; MarkRead still skips arrow browsing and scrolled-up views.
func (view *MainView) autoRead(sync *jsoncmd.SyncComplete) {
	room := view.currentRoom
	if room == nil || view.modal != nil || !userAttention.looking(time.Now()) {
		return
	}
	if sync != nil {
		if update, ok := sync.Rooms[room.Room.ID]; !ok || len(update.Timeline) == 0 {
			return
		}
	}
	view.MarkRead(room)
}

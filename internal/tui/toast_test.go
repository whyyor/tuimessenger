package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

// fakeToasts gives a fresh toaster with a controllable clock and timers.
func fakeToasts(t *testing.T) (*toaster, *time.Time, func()) {
	t.Helper()
	oldT, oldNow, oldAfter := toasts, toastNow, toastAfter
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	var fire func()
	toasts = &toaster{}
	toastNow = func() time.Time { return now }
	toastAfter = func(_ time.Duration, f func()) *time.Timer {
		fire = f
		return time.NewTimer(time.Hour)
	}
	t.Cleanup(func() { toasts, toastNow, toastAfter = oldT, oldNow, oldAfter })
	return toasts, &now, func() { fire() }
}

func current(ts *toaster) string {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.current == nil {
		return ""
	}
	return ts.current.text
}

func TestToastQueueAndReplace(t *testing.T) {
	ts, now, expire := fakeToasts(t)
	ts.Show(toastInfo, "Uploading cat.jpg…")
	ts.Show(toastSuccess, "Copied 3 messages")
	if current(ts) != "Uploading cat.jpg…" || len(ts.queue) != 1 {
		t.Fatalf("within a second the newer one waits: %q queue=%d", current(ts), len(ts.queue))
	}
	expire()
	if current(ts) != "Copied 3 messages" {
		t.Errorf("expiry shows the next: %q", current(ts))
	}
	*now = now.Add(2 * time.Second)
	ts.Show(toastError, "Failed to upload cat.jpg: too big")
	if current(ts) != "Failed to upload cat.jpg: too big" || len(ts.queue) != 0 {
		t.Errorf("after a second a newer toast replaces it: %q", current(ts))
	}
	ts.Show(toastError, "Failed to upload cat.jpg: too big")
	if len(ts.queue) != 0 {
		t.Error("the same toast twice is shown once")
	}
	expire()
	if current(ts) != "" {
		t.Error("nothing left: no toast")
	}
}

func TestToastDrawAndClick(t *testing.T) {
	ts, _, _ := fakeToasts(t)
	ts.Show(toastSuccess, "Alias set to \"sis\": find this chat by it in Ctrl+k and Ctrl+p, and it stays private to you")
	ts.Show(toastInfo, "Fetching video…")
	const w, h = 100, 20
	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(w, h)
	ts.Draw(sim)
	if ts.x+ts.w != w-2 || ts.y != 1 || ts.w > toastMaxWidth {
		t.Errorf("top right, two cells in, at most %d wide: x=%d w=%d y=%d", toastMaxWidth, ts.x, ts.w, ts.y)
	}
	r, _, st, _ := sim.GetContent(ts.x, ts.y+1)
	if fg, _, _ := st.Decompose(); r != '┃' || fg != ColorPillGreen {
		t.Errorf("left bar in the success colour: %q %v", r, fg)
	}
	if r, _, _, _ := sim.GetContent(ts.x+ts.w-1, ts.y+1); r != '┃' {
		t.Error("right bar")
	}
	text := strings.Join(screenText(sim, w, h), "\n")
	for _, want := range []string{"Alias set", "stays private", "+1 more", "x"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q:\n%s", want, text)
		}
	}
	if ts.h < 5 {
		t.Errorf("long text wraps onto several lines: h=%d", ts.h)
	}
	if !ts.Hit(ts.x+3, ts.y+1) || ts.Hit(0, 0) {
		t.Error("hit test")
	}
	ts.Dismiss()
	if current(ts) != "Fetching video…" {
		t.Errorf("a click shows the next one: %q", current(ts))
	}
}

func TestClassifyNotice(t *testing.T) {
	for text, want := range map[string]toastKind{
		"Failed to send reaction: timeout":         toastError,
		"Couldn't open the file picker: x":         toastError,
		"No URL in that message":                   toastWarning,
		"Nothing playable in that message":         toastWarning,
		"Uploading cat.jpg…":                       toastInfo,
		"Playing in QuickTime (install mpv to …)":  toastInfo,
		"Copied 3 messages":                        toastSuccess,
		"Alias removed":                            toastSuccess,
		"Mute sync done: 3 synced, 0 failed":       toastSuccess,
		"Use two slashes to send a non-command...": toastWarning,
	} {
		if got := classifyNotice(text); got != want {
			t.Errorf("%q: got %d want %d", text, got, want)
		}
	}
	if got := noticeText("Failed to parse: <code>a &amp; b</code><br>try again"); got != "Failed to parse: a & b\ntry again" {
		t.Errorf("HTML notice to text: %q", got)
	}
}

func TestServiceMessageIsToastNotTimeline(t *testing.T) {
	ts, _, _ := fakeToasts(t)
	_, room := bubbleView(t)
	view := &RoomView{Room: room}
	before := room.TimelineCache.Current()
	view.AddServiceMessage("Copied %d messages", 2)
	if current(ts) != "Copied 2 messages" {
		t.Errorf("toast: %q", current(ts))
	}
	if room.TimelineCache.Current() != before {
		t.Error("notices must not touch the timeline")
	}
}

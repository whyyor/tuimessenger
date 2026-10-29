package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/mauview"
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

func drawToast(ts *toaster, area toastArea) (tcell.SimulationScreen, string) {
	const w, h = 100, 20
	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(w, h)
	ts.Draw(sim, area)
	return sim, strings.Join(screenText(sim, w, h), "\n")
}

func TestToastDrawAndClick(t *testing.T) {
	ts, _, _ := fakeToasts(t)
	ts.Show(toastSuccess, "Alias set to \"sis\": find this chat by it in Ctrl+k and Ctrl+p, and it stays private to you")
	ts.Show(toastInfo, "Fetching video…")
	area := toastArea{x: 20, w: 80, bottom: 18}
	sim, text := drawToast(ts, area)
	if ts.x != area.x+(area.w-ts.w)/2 || ts.y+ts.h != area.bottom-1 || ts.w > toastMaxWidth {
		t.Errorf("centred, just above the status line: x=%d w=%d y=%d h=%d", ts.x, ts.w, ts.y, ts.h)
	}
	r, _, st, _ := sim.GetContent(ts.x, ts.y)
	if fg, _, _ := st.Decompose(); r != '╭' || fg != ColorHighlight {
		t.Errorf("rounded corner in the highlight colour: %q %v", r, fg)
	}
	if r, _, _, _ := sim.GetContent(ts.x+ts.w-1, ts.y+ts.h-1); r != '╯' {
		t.Error("rounded bottom-right corner")
	}
	r, _, st, _ = sim.GetContent(ts.x+2, ts.y+1)
	if fg, _, _ := st.Decompose(); r != '\uf00c' || fg != ColorPillGreen {
		t.Errorf("a green check for success: %q %v", r, fg)
	}
	for _, want := range []string{"Alias set", "stays private"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "more") || ts.h < 4 {
		t.Errorf("wrapped, no opencode extras:\n%s", text)
	}
	if !ts.Hit(ts.x+3, ts.y+1) || ts.Hit(0, 0) {
		t.Error("hit test")
	}
	ts.Dismiss()
	if current(ts) != "Fetching video…" {
		t.Errorf("a click shows the next one: %q", current(ts))
	}
}

func TestErrorToastHasTitleAndStaysLonger(t *testing.T) {
	ts, _, _ := fakeToasts(t)
	var got time.Duration
	toastAfter = func(d time.Duration, _ func()) *time.Timer { got = d; return time.NewTimer(time.Hour) }
	ts.Show(toastError, "Failed to send message: timeout")
	if got != toastErrorDuration || toastErrorDuration <= toastDuration {
		t.Errorf("errors stay longer: %v", got)
	}
	sim, text := drawToast(ts, toastArea{w: 80, bottom: 18})
	if !strings.Contains(text, "─ Error ─") {
		t.Errorf("a centred Error title:\n%s", text)
	}
	_, _, st, _ := sim.GetContent(ts.x, ts.y)
	if fg, _, _ := st.Decompose(); fg != toastColors[toastError] {
		t.Errorf("a red frame: %v", fg)
	}
}

func TestAbsOffset(t *testing.T) {
	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	outer := &mauview.ProxyScreen{Parent: sim, OffsetX: 30, OffsetY: 1}
	inner := &mauview.ProxyScreen{Parent: outer, OffsetX: 1, OffsetY: 2}
	if x, y := absOffset(inner); x != 31 || y != 3 {
		t.Errorf("got %d,%d", x, y)
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

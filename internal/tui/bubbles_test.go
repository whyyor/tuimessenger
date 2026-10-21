package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/util/jsontime"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/pkg/rpc/store"
	"go.mau.fi/gomuks/tui/config"
	"go.mau.fi/gomuks/tui/messages"
	"go.mau.fi/gomuks/tui/messages/tstring"
)

const me = id.UserID("@me:beeper.com")

func bubbleView(t *testing.T) (*MessageView, *store.RoomStore) {
	t.Helper()
	gs := &store.GomuksStore{}
	gs.UserID = me
	room := store.NewRoomStore(gs, &database.Room{ID: "!room:beeper.com"})
	return &MessageView{parent: &RoomView{Room: room}, config: &config.Config{}}, room
}

func textMsg(room *store.RoomStore, sender id.UserID, at time.Time, text string) *messages.UIMessage {
	evt := &database.Event{ID: id.EventID("$" + text), Sender: sender, Timestamp: jsontime.UM(at)}
	return messages.NewExpandedTextMessage(evt, room, tstring.NewTString(text))
}

func screenText(sim tcell.SimulationScreen, w, h int) []string {
	lines := make([]string, h)
	for y := range h {
		var b strings.Builder
		for x := 0; x < w; x++ {
			r, _, _, _ := sim.GetContent(x, y)
			b.WriteRune(r)
		}
		lines[y] = strings.TrimRight(b.String(), " ")
	}
	return lines
}

func TestBubbleLayoutSides(t *testing.T) {
	view, room := bubbleView(t)
	now := time.Date(2026, 10, 21, 17, 20, 0, 0, time.Local)
	theirs := textMsg(room, "@peter:beeper.com", now, "hello there")
	mine := textMsg(room, me, now.Add(time.Minute), "hi")
	const width = 60

	lt := view.layoutBubble(theirs, width, false, nil, true)
	if lt.own || lt.barX != 0 || lt.x != 2 || !lt.name || lt.gap {
		t.Errorf("theirs: %+v", lt)
	}
	// "hello there" (11) + 2 + "17:20" (5) fits on one line.
	if lt.inner != 18 || lt.metaRow || lt.rows != 2 {
		t.Errorf("theirs size: inner=%d metaRow=%v rows=%d", lt.inner, lt.metaRow, lt.rows)
	}
	lm := view.layoutBubble(mine, width, false, theirs, false)
	if !lm.own || lm.barX != width-1 || lm.x+lm.inner != width-2 || lm.name || !lm.gap {
		t.Errorf("mine: %+v", lm)
	}

	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(width, 6)
	view.drawBubble(sim, theirs, lt, 0, width)
	view.drawBubble(sim, mine, lm, 2, width)
	got := screenText(sim, width, 5)
	want := []string{
		" peter",
		"┃ hello there  17:20",
		"",
		strings.Repeat(" ", width-11) + "hi  17:21 ┃",
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("row %d:\n got %q\nwant %q", i, got[i], w)
		}
	}
}

func TestBubbleRunsAndWrapping(t *testing.T) {
	view, room := bubbleView(t)
	now := time.Date(2026, 10, 21, 17, 20, 0, 0, time.Local)
	a := textMsg(room, "@bob:x", now, "one")
	b := textMsg(room, "@bob:x", now.Add(2*time.Minute), "two")
	c := textMsg(room, "@bob:x", now.Add(30*time.Minute), "three")
	if l := view.layoutBubble(b, 60, false, a, false); l.name || l.gap {
		t.Errorf("same sender within 10 min should continue the run: %+v", l)
	}
	if l := view.layoutBubble(c, 60, false, b, false); !l.name || !l.gap {
		t.Errorf("a 28 min pause should start a new run: %+v", l)
	}
	if l := view.layoutBubble(a, 60, true, nil, true); l.name {
		t.Error("DMs never show names")
	}
	long := textMsg(room, "@bob:x", now, strings.Repeat("word ", 40))
	l := view.layoutBubble(long, 60, true, nil, true)
	if max := bubbleInnerMax(60); l.inner > max || l.inner < max-5 || long.Height() < 2 {
		t.Errorf("long text should wrap near the max width: inner=%d height=%d", l.inner, long.Height())
	}
}

func TestBubbleInnerMax(t *testing.T) {
	if got := bubbleInnerMax(100); got != 73 {
		t.Errorf("100 cols: %d", got)
	}
	if got := bubbleInnerMax(24); got != 18 {
		t.Errorf("narrow pane keeps the 20-col minimum: %d", got)
	}
	if got := bubbleInnerMax(10); got != 8 {
		t.Errorf("tiny pane: %d", got)
	}
}

func TestDateRule(t *testing.T) {
	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(30, 1)
	drawDateRule(sim, "October 20, 2026", 0, 30)
	if got := screenText(sim, 30, 1)[0]; got != "──── October 20, 2026 ────────" && got != "───── October 20, 2026 ───────" {
		if !strings.Contains(got, " October 20, 2026 ") || !strings.HasPrefix(got, "──") {
			t.Errorf("got %q", got)
		}
	}
}

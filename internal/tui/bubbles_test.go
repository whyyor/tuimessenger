package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/util/jsontime"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli/cmdspec"
	"github.com/whyyor/tuimessenger/internal/hicli/database"
	"github.com/whyyor/tuimessenger/internal/rpc/store"
	"github.com/whyyor/tuimessenger/internal/tui/config"
	"github.com/whyyor/tuimessenger/internal/tui/messages"
	"github.com/whyyor/tuimessenger/internal/tui/messages/tstring"
)

const me = id.UserID("@me:beeper.com")

func bubbleView(t *testing.T) (*MessageView, *store.RoomStore) {
	t.Helper()
	gs := &store.GomuksStore{}
	gs.UserID = me
	room := store.NewRoomStore(gs, &database.Room{ID: "!room:beeper.com"})
	cfg := &config.Config{}
	cfg.Preferences.TimestampStyle = "absolute"
	return &MessageView{parent: &RoomView{Room: room}, config: cfg}, room
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

	// Known status: no background lookup in tests.
	deliveries.entries[mine.ID] = &deliveryEntry{state: deliveryDelivered, checked: time.Now()}
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
		strings.Repeat(" ", width-14) + "hi  17:21 ✓✓ ┃",
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
	sim.SetSize(200, 1)
	drawDateRule(sim, "October 20, 2026", 0, 200)
	got := screenText(sim, 200, 1)[0]
	trimmed := strings.TrimLeft(got, " ")
	ruleW := len([]rune(trimmed))
	if ruleW != 70 || !strings.Contains(trimmed, "─ October 20, 2026 ─") {
		t.Errorf("want a centred 70-col rule (35%% of 200), got %d: %q", ruleW, trimmed)
	}
	if lead := len(got) - len(trimmed); lead != 65 {
		t.Errorf("rule should start at col 65, got %d", lead)
	}
	// A narrow pane still fits the label with a few dashes each side.
	sim.SetSize(30, 1)
	sim.Clear()
	drawDateRule(sim, "October 20, 2026", 0, 30)
	if row := strings.TrimSpace(screenText(sim, 30, 1)[0]); !strings.HasPrefix(row, "────") || !strings.HasSuffix(row, "────") {
		t.Errorf("narrow: %q", row)
	}
}

func TestMarkRead(t *testing.T) {
	view, room := bubbleView(t)
	now := time.Now()
	a := textMsg(room, me, now, "a")
	b := textMsg(room, me, now.Add(time.Second), "b")
	reply := textMsg(room, "@peter:x", now.Add(2*time.Second), "reply")
	c := textMsg(room, me, now.Add(3*time.Second), "c")
	msgs := []*messages.UIMessage{a, b, reply, c}
	layouts := map[*messages.UIMessage]*bubbleLayout{}
	var prev *messages.UIMessage
	for _, m := range msgs {
		layouts[m] = view.layoutBubble(m, 60, true, prev, prev == nil)
		prev = m
	}
	view.markRead(msgs, layouts)
	if !layouts[a].read || !layouts[b].read {
		t.Error("messages before their reply are read")
	}
	if layouts[c].read {
		t.Error("c has no receipt or later reply: not read")
	}
}

func TestParseDelivery(t *testing.T) {
	mk := func(status string, ts int64) *database.Event {
		return &database.Event{Content: []byte(`{"status":"` + status + `"}`), Timestamp: jsontime.UM(time.UnixMilli(ts))}
	}
	if got := parseDelivery(nil); got != deliverySent {
		t.Errorf("no status: %v", got)
	}
	if got := parseDelivery([]*database.Event{mk("FAIL_RETRIABLE", 1), mk("SUCCESS", 2)}); got != deliveryDelivered {
		t.Errorf("success wins: %v", got)
	}
	if got := parseDelivery([]*database.Event{mk("FAIL_PERMANENT", 1)}); got != deliveryFailed {
		t.Errorf("permanent failure: %v", got)
	}
	if got := parseDelivery([]*database.Event{mk("FAIL_RETRIABLE", 1)}); got != deliverySent {
		t.Errorf("retriable stays sent: %v", got)
	}
}

func TestFormatRelative(t *testing.T) {
	now := time.Date(2026, 10, 21, 17, 0, 0, 0, time.Local)
	for _, c := range []struct {
		ago  time.Duration
		want string
	}{
		{30 * time.Second, "now"},
		{5 * time.Minute, "5m"},
		{59 * time.Minute, "59m"},
		{3 * time.Hour, "3h"},
		{23*time.Hour + 59*time.Minute, "23h"},
		{50 * time.Hour, "2d"},
	} {
		if got := formatRelative(now.Add(-c.ago), now); got != c.want {
			t.Errorf("%v ago: got %q, want %q", c.ago, got, c.want)
		}
	}
}

func TestDayLabel(t *testing.T) {
	now := time.Date(2026, 10, 21, 0, 30, 0, 0, time.Local)
	for _, c := range []struct {
		day  time.Time
		want string
	}{
		{time.Date(2026, 10, 21, 0, 1, 0, 0, time.Local), "Today"},
		{time.Date(2026, 10, 20, 23, 59, 0, 0, time.Local), "Yesterday"},
		{time.Date(2026, 10, 19, 12, 0, 0, 0, time.Local), "Monday, 19 October"},
		{time.Date(2025, 12, 31, 12, 0, 0, 0, time.Local), "Wednesday, 31 December 2025"},
	} {
		if got := dayLabel(c.day, now); got != c.want {
			t.Errorf("%v: got %q, want %q", c.day, got, c.want)
		}
	}
}

func TestTimestampStyleOff(t *testing.T) {
	view, room := bubbleView(t)
	view.config.Preferences.TimestampStyle = "off"
	msg := textMsg(room, "@bob:x", time.Now(), "hey")
	lay := view.layoutBubble(msg, 60, true, nil, true)
	if lay.meta != "" || lay.inner != 3 {
		t.Errorf("off: meta %q inner %d, want no time and a snug bubble", lay.meta, lay.inner)
	}
	view.selected = 7
	msg.RowID = 7
	if lay = view.layoutBubble(msg, 60, true, nil, true); lay.meta != msg.FormatTime() {
		t.Errorf("off but selected should show the time, got %q", lay.meta)
	}
}

// Regression: a gomuks notice ("Seen in this chat: on") or a bridge bot's
// message after yours made gomuks show blue ticks nobody had earned.
func TestMarkReadIgnoresGomuksAndBots(t *testing.T) {
	view, room := bubbleView(t)
	now := time.Now()
	mine := textMsg(room, me, now, "hsr")
	notice := textMsg(room, cmdspec.FakeGomuksSender, now.Add(time.Second), "Seen in this chat: on")
	notice.ID = "$gomuks-internal-abc"
	bot := textMsg(room, "@instagramgobot:beeper.local", now.Add(2*time.Second), "bridge notice")
	layout := func(msgs ...*messages.UIMessage) map[*messages.UIMessage]*bubbleLayout {
		out := map[*messages.UIMessage]*bubbleLayout{}
		var prev *messages.UIMessage
		for _, m := range msgs {
			out[m] = view.layoutBubble(m, 60, true, prev, prev == nil)
			prev = m
		}
		return out
	}
	msgs := []*messages.UIMessage{mine, notice, bot}
	layouts := layout(msgs...)
	view.markRead(msgs, layouts)
	if layouts[mine].read {
		t.Error("a gomuks notice or bot message is not the other person reading")
	}
	reply := textMsg(room, "@instagramgo_100000000000003:beeper.local", now.Add(3*time.Second), "haan")
	msgs = append(msgs, reply)
	layouts = layout(msgs...)
	view.markRead(msgs, layouts)
	if !layouts[mine].read {
		t.Error("a real reply means they read it")
	}
}

func TestSelectionPaddingAndColours(t *testing.T) {
	view, room := bubbleView(t)
	for _, own := range []bool{false, true} {
		sender := me
		if !own {
			sender = "@peter:x"
		}
		msg := textMsg(room, sender, time.Now(), "20-25 min ka")
		msg.RowID = 5
		view.selected = 5
		lay := view.layoutBubble(msg, 60, true, nil, true)
		if own {
			deliveries.entries[msg.ID] = &deliveryEntry{state: deliveryDelivered, checked: time.Now()}
		}
		sim := tcell.NewSimulationScreen("")
		_ = sim.Init()
		sim.SetSize(60, 3)
		view.drawBubble(sim, msg, lay, 0, 60)
		bgAt := func(x int) tcell.Color {
			_, _, st, _ := sim.GetContent(x, 0)
			_, bg, _ := st.Decompose()
			return bg
		}
		if bgAt(lay.x-1) != messages.SelectionBackground || bgAt(lay.x+lay.inner) != messages.SelectionBackground {
			t.Errorf("own=%v: one padding cell each side", own)
		}
		_, _, st, _ := sim.GetContent(lay.x, 0)
		if fg, _, _ := st.Decompose(); fg != messages.SelectionForeground {
			t.Errorf("own=%v: selected text should be white, got %v", own, fg)
		}
		if r, _, _, _ := sim.GetContent(lay.barX, 0); r != '┃' {
			t.Errorf("own=%v: the bar stays", own)
		}
	}
}

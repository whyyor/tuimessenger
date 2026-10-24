package tui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/mauview"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/cmdspec"
	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/tui/config"
	"go.mau.fi/gomuks/tui/messages"
)

func TestChatSearch(t *testing.T) {
	mv, room := bubbleView(t)
	now := time.Now()
	mk := func(row database.EventRowID, sender id.UserID, text string) *messages.UIMessage {
		m := textMsg(room, sender, now.Add(time.Duration(row)*time.Minute), text)
		m.RowID = row
		return m
	}
	a := mk(1, "@peter:x", "bring the Harness")
	b := mk(2, me, "ok")
	c := mk(3, "@peter:x", "harness and chalk")
	notice := mk(4, cmdspec.FakeGomuksSender, "harness notice")
	mv.msgBuffer = []*messages.UIMessage{a, a, b, c, notice}

	if got := mv.SearchRows("HARNESS"); len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Fatalf("case-insensitive, oldest first, no gomuks notices: %v", got)
	}

	cfg := &config.Config{}
	cfg.LoadKeybindings()
	view := &RoomView{Room: room, config: cfg, content: mv, input: mauview.NewInputArea()}
	mv.parent = view
	mv.selected = 2
	view.search = &chatSearch{cur: -1, prevSelected: mv.selected}
	key := func(k tcell.Key, r rune) { view.OnKeyEvent(tcell.NewEventKey(k, r, tcell.ModNone)) }
	for _, r := range "harn" {
		key(tcell.KeyRune, r)
	}
	if mv.selected != 3 {
		t.Errorf("typing jumps to the newest match, selected %d", mv.selected)
	}
	key(tcell.KeyUp, 0)
	if mv.selected != 1 {
		t.Errorf("↑ goes to the older match, selected %d", mv.selected)
	}
	key(tcell.KeyUp, 0)
	if mv.selected != 1 {
		t.Error("↑ stops at the oldest")
	}
	// Escape restores where we were.
	key(tcell.KeyEscape, 0)
	if view.search != nil || mv.selected != 2 {
		t.Errorf("escape: search %v, selected %d", view.search, mv.selected)
	}
	// Enter keeps the match selected, in visual mode.
	view.search = &chatSearch{cur: -1}
	for _, r := range "chalk" {
		key(tcell.KeyRune, r)
	}
	key(tcell.KeyEnter, 0)
	if view.search != nil || !view.selecting || mv.selected != 3 {
		t.Errorf("enter: search %v, selecting %v, selected %d", view.search, view.selecting, mv.selected)
	}
}

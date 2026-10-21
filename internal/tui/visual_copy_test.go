package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/mauview"

	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/tui/messages"
)

func TestVisualRangeCopiesEveryMessage(t *testing.T) {
	copied := make(chan string, 1)
	orig := writeClipboard
	writeClipboard = func(text, _ string) error { copied <- text; return nil }
	defer func() { writeClipboard = orig }()

	mv, room := bubbleView(t)
	mv.config.LoadKeybindings()
	now := time.Date(2026, 10, 22, 9, 0, 0, 0, time.Local)
	msgs := []*messages.UIMessage{
		textMsg(room, "@peter:x", now, "first"),
		textMsg(room, me, now.Add(time.Minute), "second"),
		textMsg(room, "@peter:x", now.Add(2*time.Minute), "third"),
	}
	for i, m := range msgs {
		m.RowID = database.EventRowID(i + 1)
		mv.msgBuffer = append(mv.msgBuffer, m)
	}
	view := &RoomView{Room: room, config: mv.config, content: mv, input: mauview.NewInputArea(), selecting: true}
	mv.parent = view

	// Cursor on the newest, v anchors, k k extends up to the oldest, y copies.
	mv.selected = msgs[2].RowID
	key := func(r rune) { view.OnKeyEvent(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone)) }
	key('v')
	mv.selected = msgs[0].RowID
	key('y')

	select {
	case got := <-copied:
		for _, want := range []string{"first", "second", "third"} {
			if !strings.Contains(got, want) {
				t.Errorf("range copy missing %q:\n%s", want, got)
			}
		}
		if lines := strings.Count(got, "\n") + 1; lines != 3 {
			t.Errorf("want 3 lines, got %d:\n%s", lines, got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("nothing copied")
	}
	if view.selecting {
		t.Error("visual mode should end after copying")
	}
}

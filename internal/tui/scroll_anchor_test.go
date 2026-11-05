package tui

import (
	"fmt"
	"testing"
	"time"

	"go.mau.fi/util/jsontime"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
	"go.mau.fi/gomuks/pkg/rpc/store"
)

const anchorWidth = 60

// addMsgs appends n peer messages with row IDs from..from+n-1.
func addMsgs(room *store.RoomStore, from, n int) {
	var evts []*database.Event
	var tl []database.TimelineRowTuple
	base := time.Now().Add(-time.Hour)
	for i := from; i < from+n; i++ {
		evts = append(evts, &database.Event{RowID: database.EventRowID(i), TimelineRowID: database.TimelineRowID(i),
			ID: id.EventID(fmt.Sprintf("$m%d:x", i)), RoomID: room.ID, Sender: "@peter:x", Type: "m.room.message",
			Content:   []byte(fmt.Sprintf(`{"msgtype":"m.text","body":"message %d"}`, i)),
			Timestamp: jsontime.UM(base.Add(time.Duration(i) * time.Second))})
		tl = append(tl, database.TimelineRowTuple{Timeline: database.TimelineRowID(i), Event: database.EventRowID(i)})
	}
	room.ApplySync(&jsoncmd.SyncRoom{Events: evts, Timeline: tl})
}

// screenRowOf is where row's last line sits, counted up from the viewport bottom.
func screenRowOf(view *MessageView, row database.EventRowID) int {
	last := lastLineOf(view.msgBuffer, row)
	return len(view.msgBuffer) - 1 - view.GetScrollOffset() - last
}

func anchorView(t *testing.T) (*MessageView, *store.RoomStore) {
	t.Helper()
	view, room := bubbleView(t)
	addMsgs(room, 1, 30)
	view.update(anchorWidth)
	if len(view.msgBuffer) == 0 {
		t.Skip("timeline didn't render in this setup")
	}
	return view, room
}

func TestScrolledUpStaysPutWhenMessagesArrive(t *testing.T) {
	view, room := anchorView(t)
	view.ScrollOffset.Store(12)
	bottom := view.msgBuffer[len(view.msgBuffer)-1-12].RowID
	before := screenRowOf(view, bottom)

	addMsgs(room, 31, 3)
	view.update(anchorWidth)
	if got := screenRowOf(view, bottom); got != before {
		t.Errorf("message %d moved from row %d to %d", bottom, before, got)
	}
}

func TestVisualSelectionStaysPutAtBottom(t *testing.T) {
	view, room := anchorView(t)
	view.selected = 28
	before := screenRowOf(view, 28)

	addMsgs(room, 31, 4)
	view.update(anchorWidth)
	if got := screenRowOf(view, 28); got != before {
		t.Errorf("selected message moved from row %d to %d", before, got)
	}
	if view.GetScrollOffset() == 0 {
		t.Error("the new messages should sit below the viewport")
	}
}

func TestBottomFollowsNewMessages(t *testing.T) {
	view, room := anchorView(t)
	addMsgs(room, 31, 2)
	view.update(anchorWidth)
	if view.GetScrollOffset() != 0 {
		t.Errorf("at the bottom and not selecting: follow, got offset %d", view.GetScrollOffset())
	}
	if view.msgBuffer[len(view.msgBuffer)-1].RowID != 32 {
		t.Error("the newest message is at the bottom")
	}
}

// The previous last event vanishing (a pending send replaced) used to break it.
func TestAnchorSurvivesLastEventReplaced(t *testing.T) {
	view, room := anchorView(t)
	pending := &database.Event{RowID: -5, ID: "$pending:x", RoomID: room.ID, Sender: me, Type: "m.room.message",
		Content: []byte(`{"msgtype":"m.text","body":"mine"}`), Timestamp: jsontime.UM(time.Now())}
	room.ApplyPending(pending)
	view.update(anchorWidth)
	view.ScrollOffset.Store(10)
	bottom := view.msgBuffer[len(view.msgBuffer)-1-10].RowID
	before := screenRowOf(view, bottom)

	sent := *pending
	sent.RowID, sent.TimelineRowID, sent.ID = 40, 40, "$sent:x"
	room.ApplySendComplete(&sent)
	addMsgs(room, 41, 2)
	view.update(anchorWidth)
	if got := screenRowOf(view, bottom); got != before {
		t.Errorf("message %d moved from row %d to %d", bottom, before, got)
	}
}

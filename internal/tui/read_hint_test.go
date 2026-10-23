package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/mauview"
	"go.mau.fi/util/jsontime"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
)

func TestReadHint(t *testing.T) {
	view, room := bubbleView(t)
	rv := &RoomView{Room: room, config: view.config, input: mauview.NewInputArea(),
		status: mauview.NewTextField(), content: view}
	view.parent = rv
	at := time.Now().Add(-time.Minute)
	mine := &database.Event{RowID: 1, TimelineRowID: 1, ID: "$mine:x", RoomID: room.ID,
		Sender: me, Type: "m.room.message", Timestamp: jsontime.UM(at)}
	theirs := &database.Event{RowID: 2, TimelineRowID: 2, ID: "$theirs:x", RoomID: room.ID,
		Sender: "@whatsapp_lid-1:beeper.local", Type: "m.room.message", Timestamp: jsontime.UM(at.Add(time.Second))}
	room.ApplySync(&jsoncmd.SyncRoom{
		Events:   []*database.Event{mine, theirs},
		Timeline: []database.TimelineRowTuple{{Timeline: 1, Event: 1}, {Timeline: 2, Event: 2}},
	})
	if room.TimelineCache.Current() == nil {
		t.Skip("timeline cache not built by ApplySync in this setup")
	}
	if !rv.needsReadHint() {
		t.Fatal("their message has no public read yet: hint should show")
	}

	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(60, 1)
	rv.drawReadHint(sim, 0, 60)
	row := screenText(sim, 60, 1)[0]
	if lead := len(row) - len(strings.TrimLeft(row, " ")); lead != (60-len(readHint))/2 || strings.TrimSpace(row) != readHint {
		t.Errorf("hint should be centred: %q", row)
	}

	// A private read changes nothing; our public receipt (Ctrl+m, any device) hides it.
	notePublicReceipts(room, map[id.EventID][]*database.Receipt{
		"$theirs:x": {{UserID: me, ReceiptType: event.ReceiptTypeReadPrivate}},
	})
	if !rv.needsReadHint() {
		t.Error("a private read doesn't count")
	}
	notePublicReceipts(room, map[id.EventID][]*database.Receipt{
		"$theirs:x": {{UserID: me, ReceiptType: event.ReceiptTypeRead}},
	})
	if rv.needsReadHint() {
		t.Error("after a public read the hint goes away")
	}

	// /seen on chats read publicly by themselves: never a hint.
	publicReads.Lock()
	delete(publicReads.at, room.ID)
	publicReads.Unlock()
	room.ApplySync(&jsoncmd.SyncRoom{AccountData: map[event.Type]*database.AccountData{
		AccountDataPrivacy: {Type: AccountDataPrivacy.Type, Content: []byte(`{"read_receipts":true}`)},
	}})
	if rv.needsReadHint() {
		t.Error("/seen on: no hint")
	}
}

func TestNoReadHintWhenLastMessageIsMine(t *testing.T) {
	view, room := bubbleView(t)
	rv := &RoomView{Room: room, config: view.config}
	at := time.Now().Add(-time.Minute)
	evts := []*database.Event{
		{RowID: 1, TimelineRowID: 1, ID: "$theirs:x", RoomID: room.ID, Sender: "@whatsapp_lid-1:beeper.local",
			Type: "m.room.message", Timestamp: jsontime.UM(at)},
		{RowID: 2, TimelineRowID: 2, ID: "$reply:x", RoomID: room.ID, Sender: me,
			Type: "m.room.message", Timestamp: jsontime.UM(at.Add(time.Second))},
		// The bridge's status event after our reply doesn't count as theirs.
		{RowID: 3, TimelineRowID: 3, ID: "$mss:x", RoomID: room.ID, Sender: "@whatsappbot:beeper.local",
			Type: "com.beeper.message_send_status", Timestamp: jsontime.UM(at.Add(2 * time.Second))},
	}
	room.ApplySync(&jsoncmd.SyncRoom{
		Events:   evts,
		Timeline: []database.TimelineRowTuple{{Timeline: 1, Event: 1}, {Timeline: 2, Event: 2}, {Timeline: 3, Event: 3}},
	})
	if rv.needsReadHint() {
		t.Error("our reply is the newest message: no hint")
	}
}

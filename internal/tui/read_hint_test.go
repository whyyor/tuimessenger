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
	theirs := &database.Event{RowID: 1, TimelineRowID: 1, ID: "$theirs:x", RoomID: room.ID,
		Sender: "@whatsapp_lid-1:beeper.local", Type: "m.room.message", Timestamp: jsontime.UM(at)}
	mine := &database.Event{RowID: 2, TimelineRowID: 2, ID: "$mine:x", RoomID: room.ID,
		Sender: me, Type: "m.room.message", Timestamp: jsontime.UM(at.Add(time.Second))}
	room.ApplySync(&jsoncmd.SyncRoom{
		Events:   []*database.Event{theirs, mine},
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
	if row := screenText(sim, 60, 1)[0]; !strings.HasSuffix(row, readHint) {
		t.Errorf("hint should sit at the right end: %q", row)
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

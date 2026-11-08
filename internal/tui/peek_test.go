package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/mauview"
	"go.mau.fi/util/jsontime"
	"maunium.net/go/mautrix/event"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
	"github.com/whyyor/tuimessenger/internal/hicli/jsoncmd"
	"github.com/whyyor/tuimessenger/internal/rpc/store"
)

func seenOn(room *store.RoomStore) {
	room.ApplySync(&jsoncmd.SyncRoom{AccountData: map[event.Type]*database.AccountData{
		AccountDataPrivacy: {Type: AccountDataPrivacy.Type, Content: []byte(`{"read_receipts":true}`)},
	}})
}

func TestPeekNeverSendsPublicRead(t *testing.T) {
	_, room := bubbleView(t)
	rv := &RoomView{Room: room}
	if sendsPublicRead(rv) {
		t.Error("/seen off: reads stay local")
	}
	seenOn(room)
	if !sendsPublicRead(rv) {
		t.Error("/seen on: reads go public")
	}
	rv.peek = true
	if sendsPublicRead(rv) {
		t.Error("peeking a /seen-on chat must not send seen")
	}
}

// Opening a chat: /seen on tells the network, /seen off reads it in tuimessenger only, peek leaves no trace.
func TestReadModes(t *testing.T) {
	_, room := bubbleView(t)
	rv := &RoomView{Room: room}
	if readModeFor(rv) != readLocal {
		t.Error("/seen off (the default): read here only")
	}
	rv.peek = true
	if readModeFor(rv) != readNone {
		t.Error("peek with /seen off: no trace, stays in quick access")
	}
	seenOn(room)
	if readModeFor(rv) != readNone {
		t.Error("peek with /seen on: still no trace")
	}
	rv.peek = false
	if readModeFor(rv) != readPublic {
		t.Error("/seen on: public read")
	}
}

func TestPeekShowsPillAndReadHint(t *testing.T) {
	view, room := bubbleView(t)
	rv := &RoomView{Room: room, config: view.config, input: mauview.NewInputArea(),
		status: mauview.NewTextField(), content: view}
	view.parent = rv
	theirs := &database.Event{RowID: 1, TimelineRowID: 1, ID: "$peek:x", RoomID: room.ID,
		Sender: "@whatsapp_lid-1:beeper.local", Type: "m.room.message", Timestamp: jsontime.UM(time.Now())}
	room.ApplySync(&jsoncmd.SyncRoom{
		Events:   []*database.Event{theirs},
		Timeline: []database.TimelineRowTuple{{Timeline: 1, Event: 1}},
	})
	if room.TimelineCache.Current() == nil {
		t.Skip("timeline cache not built by ApplySync in this setup")
	}
	seenOn(room)
	if rv.needsReadHint() {
		t.Fatal("precondition: /seen on chats don't need the hint")
	}
	rv.peek = true
	if !rv.needsReadHint() {
		t.Error("peeking: the ctrl+m hint is back, like /seen off")
	}
	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(90, 1)
	rv.drawReadHint(sim, 0, 90)
	row := screenText(sim, 90, 1)[0]
	if !strings.Contains(row, "peek · not sending seen") || !strings.Contains(row, readHint) {
		t.Errorf("both pills should show: %q", row)
	}
}

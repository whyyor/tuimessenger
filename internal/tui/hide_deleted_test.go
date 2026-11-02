package tui

import (
	"slices"
	"testing"
	"time"

	"go.mau.fi/util/jsontime"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/cmdspec"
	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
)

func bufferRows(view *MessageView) []database.EventRowID {
	var rows []database.EventRowID
	for _, msg := range view.msgBuffer {
		if msg.RowID != 0 && !slices.Contains(rows, msg.RowID) {
			rows = append(rows, msg.RowID)
		}
	}
	return rows
}

func TestHideDeletedPerChat(t *testing.T) {
	view, room := bubbleView(t)
	base := time.Now().Add(-time.Hour)
	mk := func(row int, redacted bool) *database.Event {
		evt := &database.Event{RowID: database.EventRowID(row), TimelineRowID: database.TimelineRowID(row),
			ID: id.EventID("$" + string(rune('a'+row)) + ":x"), RoomID: room.ID, Sender: "@peter:x", Type: "m.room.message",
			Content: []byte(`{"msgtype":"m.text","body":"hi"}`), Timestamp: jsontime.UM(base.Add(time.Duration(row) * time.Minute))}
		if redacted {
			evt.RedactedBy = "$redaction:x"
		}
		return evt
	}
	room.ApplySync(&jsoncmd.SyncRoom{
		Events:   []*database.Event{mk(1, false), mk(2, true), mk(3, false)},
		Timeline: []database.TimelineRowTuple{{Timeline: 1, Event: 1}, {Timeline: 2, Event: 2}, {Timeline: 3, Event: 3}},
	})
	view.update(60)
	if got := bufferRows(view); !slices.Equal(got, []database.EventRowID{1, 2, 3}) {
		t.Fatalf("by default the deleted message shows: %v", got)
	}
	setView := func(content string) {
		room.ApplySync(&jsoncmd.SyncRoom{AccountData: map[event.Type]*database.AccountData{
			AccountDataChatView: {Type: AccountDataChatView.Type, Content: []byte(content)},
		}})
		view.update(60) // no new messages: the setting alone must rebuild
	}
	setView(`{"hide_deleted":true}`)
	if got := bufferRows(view); !slices.Equal(got, []database.EventRowID{1, 3}) {
		t.Errorf("hide_deleted leaves out only the deleted message: %v", got)
	}
	setView(`{}`)
	if got := bufferRows(view); !slices.Equal(got, []database.EventRowID{1, 2, 3}) {
		t.Errorf("turning it off brings it back: %v", got)
	}
}

func TestHideDeletedCommandParses(t *testing.T) {
	for _, cmd := range LocalCommands {
		if cmd.Command != CmdHideDel {
			continue
		}
		for _, input := range []string{"/hide-deleted", "/hide-deleted on", "/hide-deleted off"} {
			if parsed, err := cmd.ParseInput(cmdspec.FakeGomuksSender, cmdSigils, input); err != nil || parsed == nil {
				t.Errorf("%s: %v", input, err)
			}
		}
		return
	}
	t.Fatal("no /hide-deleted command")
}

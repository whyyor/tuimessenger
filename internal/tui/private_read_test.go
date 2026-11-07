package tui

import (
	"context"
	"encoding/json"
	"testing"

	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/event"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
	"github.com/whyyor/tuimessenger/internal/hicli/jsoncmd"
	"github.com/whyyor/tuimessenger/internal/rpc"
	"github.com/whyyor/tuimessenger/internal/rpc/store"
)

// A private read (/seen off or peek) must send nothing a bridge passes to the network: no receipt, and not the
// marked-unread flag either, which WhatsApp turns into "mark chat as read".
// recordingAPI answers every command with success and lists the account data types written.
func recordingAPI(sent *[]string) *rpc.Commands {
	return rpc.New(func(_ context.Context, cmd *jsoncmd.Container[json.RawMessage]) *jsoncmd.Container[json.RawMessage] {
		var params jsoncmd.SetAccountDataParams
		if json.Unmarshal(cmd.Data, &params) == nil && params.Type != "" {
			*sent = append(*sent, params.Type)
		} else {
			*sent = append(*sent, string(cmd.Command))
		}
		return &jsoncmd.Container[json.RawMessage]{Command: jsoncmd.RespSuccess, RequestID: cmd.RequestID, Data: json.RawMessage("{}")}
	})
}

func TestPrivateReadLeavesMarkedUnreadAlone(t *testing.T) {
	var sent []string
	api := recordingAPI(&sent)
	gs := store.NewStore()
	room := store.NewRoomStore(gs, &database.Room{ID: "!roy:x", MarkedUnread: ptr.Ptr(true)})
	if !saveLocalSeen(api, room, &jsoncmd.MarkReadParams{RoomID: room.ID, EventID: "$last"}) {
		t.Fatal("the seen marker should be written")
	}
	if len(sent) != 1 || sent[0] != AccountDataSeen.Type {
		t.Errorf("a private read may only save tuimessenger's own seen marker, sent %v", sent)
	}
	for _, s := range sent {
		if s == event.AccountDataMarkedUnread.Type {
			t.Error("clearing marked-unread reaches WhatsApp as a read")
		}
	}
}

// Marking a chat unread here must also forget its local read, or the local read would hide the flag right away.
func TestMarkUnreadForgetsLocalRead(t *testing.T) {
	var sent []string
	api := recordingAPI(&sent)
	gs := store.NewStore()
	room := store.NewRoomStore(gs, &database.Room{ID: "!r:x"})
	room.ApplySync(&jsoncmd.SyncRoom{AccountData: map[event.Type]*database.AccountData{
		AccountDataSeen: {Type: AccountDataSeen.Type, Content: []byte(`{"event_id":"$last","timestamp":1}`)},
	}})
	if err := writeMarkedUnread(api, room); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 2 || sent[0] != AccountDataSeen.Type || sent[1] != event.AccountDataMarkedUnread.Type {
		t.Errorf("want the local read cleared, then the flag set; sent %v", sent)
	}
	if (seenMarker{}).coveredBy(&database.Event{ID: "$last"}) {
		t.Error("an empty marker must cover nothing")
	}
}

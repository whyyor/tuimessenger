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
func TestPrivateReadLeavesMarkedUnreadAlone(t *testing.T) {
	var sent []string
	api := rpc.New(func(_ context.Context, cmd *jsoncmd.Container[json.RawMessage]) *jsoncmd.Container[json.RawMessage] {
		var params jsoncmd.SetAccountDataParams
		if json.Unmarshal(cmd.Data, &params) == nil && params.Type != "" {
			sent = append(sent, params.Type)
		} else {
			sent = append(sent, string(cmd.Command))
		}
		return &jsoncmd.Container[json.RawMessage]{Command: jsoncmd.RespSuccess, RequestID: cmd.RequestID, Data: json.RawMessage("{}")}
	})
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

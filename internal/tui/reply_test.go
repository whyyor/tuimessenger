package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/util/jsontime"
	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
	"github.com/whyyor/tuimessenger/internal/hicli/jsoncmd"
	"github.com/whyyor/tuimessenger/internal/rpc/client"
	"github.com/whyyor/tuimessenger/internal/rpc/store"
	"github.com/whyyor/tuimessenger/internal/tui/config"
	"github.com/whyyor/tuimessenger/internal/tui/messages"
)

const (
	replyDM    = id.RoomID("!dm:x")
	replyGroup = id.RoomID("!group:x")
)

// fakeEventAPI answers GetEvent from a fixed set of events; every other backend call is unused.
type fakeEventAPI struct {
	jsoncmd.API
	events map[id.EventID]*database.Event
	calls  atomic.Int32
	asked  chan id.RoomID
	// release, when set, holds GetEvent open until closed.
	release chan struct{}
}

func (f *fakeEventAPI) GetEvent(_ context.Context, params *jsoncmd.GetEventParams) (*database.Event, error) {
	f.calls.Add(1)
	f.asked <- params.RoomID
	if f.release != nil {
		<-f.release
	}
	return f.events[params.EventID], nil
}

var eventSeq atomic.Int64

// freshID keeps fetched quotes unique across -count runs, since fetches are deduped process-wide.
func freshID(name string) id.EventID {
	return id.EventID(fmt.Sprintf("$%s-%d-%d:x", name, time.Now().UnixNano(), eventSeq.Add(1)))
}

func textEvent(rowID database.EventRowID, evtID id.EventID, roomID id.RoomID, sender id.UserID, body string, inReplyTo map[string]any) *database.Event {
	content := map[string]any{"msgtype": "m.text", "body": body}
	if inReplyTo != nil {
		content["m.relates_to"] = map[string]any{"m.in_reply_to": inReplyTo}
	}
	raw, _ := json.Marshal(content)
	return &database.Event{RowID: rowID, ID: evtID, RoomID: roomID, Sender: sender, Type: "m.room.message",
		Content: raw, Timestamp: jsontime.UM(time.Now())}
}

// replyClient has a DM and a "Project group" room, with groupEvents loaded into the group.
func replyClient(t *testing.T, groupEvents ...*database.Event) (*client.Client, *fakeEventAPI) {
	t.Helper()
	gs := store.NewStore()
	gs.UserID = me
	gs.ApplySync(&jsoncmd.SyncComplete{Rooms: map[id.RoomID]*jsoncmd.SyncRoom{
		replyDM:    {Meta: &database.Room{ID: replyDM}},
		replyGroup: {Meta: &database.Room{ID: replyGroup, Name: ptr.Ptr("Project group")}, Events: groupEvents},
	}})
	api := &fakeEventAPI{events: map[id.EventID]*database.Event{}, asked: make(chan id.RoomID, 4)}
	return &client.Client{API: api, Store: gs}, api
}

func replyHeader(t *testing.T, msg *messages.UIMessage) string {
	t.Helper()
	msg.CalculateBuffer(config.UserPreferences{}, 70)
	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(70, msg.Height())
	msg.Draw(sim)
	return screenText(sim, 70, 1)[0]
}

// WhatsApp "reply privately": the DM message quotes a group message the bridge marks with cross_room_id.
func TestCrossRoomReplyShowsQuoteAndChat(t *testing.T) {
	quoted := textEvent(1, "$quoted:x", replyGroup, me, "did you read my message?", nil)
	matrix, _ := replyClient(t, quoted)
	reply := textEvent(2, "$reply1:x", replyDM, "@friend:x", "haha nice",
		map[string]any{"event_id": "$quoted:x", "com.beeper.cross_room_id": string(replyGroup)})

	msg := messages.ParseEvent(matrix, &config.UserPreferences{}, matrix.GetRoom(replyDM), reply)
	if msg.ReplyTo == nil || msg.ReplyTo.PlainText() != "did you read my message?" {
		t.Fatalf("quote should be the group message, got %+v", msg.ReplyTo)
	}
	if header := replyHeader(t, msg); !strings.Contains(header, "· in Project group") {
		t.Errorf("header should name the group: %q", header)
	}
}

// A quote that isn't loaded shows a placeholder, is fetched from its own room, then renders for real.
func TestUnloadedCrossRoomReplyIsFetched(t *testing.T) {
	matrix, api := replyClient(t)
	quotedID := freshID("quoted")
	api.events[quotedID] = textEvent(3, quotedID, replyGroup, me, "fetched later", nil)
	rendered := make(chan struct{}, 1)
	messages.RequestRender = func() { rendered <- struct{}{} }
	t.Cleanup(func() { messages.RequestRender = func() {} })

	dm := matrix.GetRoom(replyDM)
	reply := textEvent(4, "$reply2:x", replyDM, "@friend:x", "ok",
		map[string]any{"event_id": string(quotedID), "com.beeper.cross_room_id": string(replyGroup)})
	msg := messages.ParseEvent(matrix, &config.UserPreferences{}, dm, reply)
	if msg.ReplyTo == nil || msg.ReplyTo.PlainText() != "Original message isn't loaded" {
		t.Fatalf("expected the placeholder, got %+v", msg.ReplyTo)
	}
	if room := <-api.asked; room != replyGroup {
		t.Errorf("fetched from %s, want the group", room)
	}
	select {
	case <-rendered:
	case <-time.After(2 * time.Second):
		t.Fatal("no re-render after the fetch")
	}
	if !msg.ReplyArrived() {
		t.Fatal("the fetched quote should mark the message for re-parsing")
	}
	again := messages.ParseEvent(matrix, &config.UserPreferences{}, dm, reply)
	if again.ReplyTo.PlainText() != "fetched later" || again.ReplyArrived() {
		t.Errorf("re-parse should show the fetched quote, got %q", again.ReplyTo.PlainText())
	}
	if again.ReplyRoomName != "Project group" {
		t.Errorf("chat name: %q", again.ReplyRoomName)
	}
}

// A same-room reply to an older, unloaded message is fetched from that room (the old TODO).
func TestUnloadedSameRoomReplyIsFetchedOnce(t *testing.T) {
	matrix, api := replyClient(t)
	oldID := freshID("old")
	api.events[oldID] = textEvent(5, oldID, replyDM, "@friend:x", "old one", nil)
	api.release = make(chan struct{})
	rendered := make(chan struct{}, 1)
	messages.RequestRender = func() { rendered <- struct{}{} }
	t.Cleanup(func() { messages.RequestRender = func() {} })

	dm := matrix.GetRoom(replyDM)
	reply := textEvent(6, "$reply3:x", replyDM, me, "yes", map[string]any{"event_id": string(oldID)})
	msg := messages.ParseEvent(matrix, &config.UserPreferences{}, dm, reply)
	if msg.ReplyRoomName != "" {
		t.Errorf("same-room reply shouldn't name a chat: %q", msg.ReplyRoomName)
	}
	if room := <-api.asked; room != replyDM {
		t.Errorf("fetched from %s, want the DM", room)
	}
	// A second reply to it while the fetch is in flight doesn't ask again.
	_ = messages.ParseEvent(matrix, &config.UserPreferences{}, dm, textEvent(7, "$reply4:x", replyDM, me, "again", map[string]any{"event_id": string(oldID)}))
	close(api.release)
	<-rendered
	if n := api.calls.Load(); n != 1 {
		t.Errorf("the same quote should be fetched once, got %d", n)
	}
	if again := messages.ParseEvent(matrix, &config.UserPreferences{}, dm, reply); again.ReplyTo.PlainText() != "old one" {
		t.Errorf("re-parse should show the fetched quote, got %q", again.ReplyTo.PlainText())
	}
}

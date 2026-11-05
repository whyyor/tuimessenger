package matrixconn

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/media"
	"github.com/whyyor/tuimessenger/internal/store"
	"github.com/whyyor/tuimessenger/internal/typing"
)

const (
	server = "test.localhost"
	owner  = id.UserID("@me:" + server)
)

func newTestConnector(t *testing.T) (*Connector, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(dir, "store.db"), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	med, err := media.New(filepath.Join(dir, "media"))
	if err != nil {
		t.Fatal(err)
	}
	return New(zerolog.Nop(), st, med, typing.New(), "whatsapp", server, owner), st
}

func text(body string) *event.Content {
	return &event.Content{Parsed: &event.MessageEventContent{MsgType: event.MsgText, Body: body}}
}

func createRoom(t *testing.T, c *Connector, ghost id.UserID) id.RoomID {
	t.Helper()
	roomID, err := c.bot.CreateRoom(context.Background(), &mautrix.ReqCreateRoom{
		BeeperLocalRoomID:    c.GenerateDeterministicRoomID(networkid.PortalKey{ID: "chat1"}),
		BeeperInitialMembers: []id.UserID{owner, ghost},
		InitialState: []*event.Event{{
			Type:    event.StateRoomName,
			Content: event.Content{Parsed: &event.RoomNameEventContent{Name: "Chat"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return roomID
}

func TestGhostIDs(t *testing.T) {
	c, _ := newTestConnector(t)
	mxid := c.FormatGhostMXID("15550100001")
	if mxid != "@whatsapp_15550100001:"+server {
		t.Errorf("FormatGhostMXID = %s", mxid)
	}
	if userID, ok := c.ParseGhostMXID(mxid); !ok || userID != "15550100001" {
		t.Errorf("ParseGhostMXID = %q %v", userID, ok)
	}
	for _, notGhost := range []id.UserID{owner, c.bot.mxid, "@whatsapp_1:other.server", "@telegram_1:" + server} {
		if _, ok := c.ParseGhostMXID(notGhost); ok {
			t.Errorf("%s should not parse as a ghost", notGhost)
		}
	}
	if c.bot.mxid != "@whatsappbot:"+server {
		t.Errorf("bot = %s", c.bot.mxid)
	}
}

func TestCreateRoomAndMembers(t *testing.T) {
	ctx := context.Background()
	c, st := newTestConnector(t)
	ghost := c.GhostIntent("alice")
	if err := ghost.SetDisplayName(ctx, "Alice"); err != nil {
		t.Fatal(err)
	}
	roomID := createRoom(t, c, ghost.GetMXID())
	if again := createRoom(t, c, ghost.GetMXID()); again != roomID {
		t.Errorf("creating the same room twice gave %s and %s", roomID, again)
	}

	members, err := c.GetMembers(ctx, roomID)
	if err != nil || len(members) != 3 {
		t.Fatalf("GetMembers: %d members, %v", len(members), err)
	}
	if name := members[ghost.GetMXID()].Displayname; name != "Alice" {
		t.Errorf("ghost displayname = %q", name)
	}
	if evt, _ := st.GetState(ctx, roomID, event.StateRoomName, ""); evt == nil {
		t.Error("room name was not saved")
	}

	// A rename shows up in every room the ghost is in.
	if err = ghost.SetDisplayName(ctx, "Alice Smith"); err != nil {
		t.Fatal(err)
	}
	info, err := c.GetMemberInfo(ctx, roomID, ghost.GetMXID())
	if err != nil || info.Displayname != "Alice Smith" {
		t.Errorf("after rename: %+v %v", info, err)
	}
}

func TestSendMessageIDs(t *testing.T) {
	ctx := context.Background()
	c, st := newTestConnector(t)
	ghost := c.GhostIntent("alice")
	roomID := createRoom(t, c, ghost.GetMXID())
	msg := &database.Message{Room: networkid.PortalKey{ID: "chat1"}, ID: "msg1"}

	first, err := ghost.SendMessage(ctx, roomID, event.EventMessage, text("hi"), &bridgev2.MatrixSendExtra{MessageMeta: msg})
	if err != nil {
		t.Fatal(err)
	}
	before := st.LastPos()
	again, err := ghost.SendMessage(ctx, roomID, event.EventMessage, text("hi"), &bridgev2.MatrixSendExtra{MessageMeta: msg})
	if err != nil || again.EventID != first.EventID || st.LastPos() != before {
		t.Errorf("resending: id %s vs %s, err %v, pos %d -> %d", first.EventID, again.EventID, err, before, st.LastPos())
	}

	// Edits and redactions carry the original's metadata but must get their own IDs.
	edit := text("* hello")
	edit.Parsed.(*event.MessageEventContent).RelatesTo = (&event.RelatesTo{}).SetReplace(first.EventID)
	edited, err := ghost.SendMessage(ctx, roomID, event.EventMessage, edit, &bridgev2.MatrixSendExtra{MessageMeta: msg})
	if err != nil || edited.EventID == first.EventID {
		t.Errorf("edit: id %s, err %v", edited.EventID, err)
	}
	redaction := &event.Content{Parsed: &event.RedactionEventContent{Redacts: first.EventID}}
	redacted, err := ghost.SendMessage(ctx, roomID, event.EventRedaction, redaction, &bridgev2.MatrixSendExtra{MessageMeta: msg})
	if err != nil || redacted.EventID == first.EventID {
		t.Errorf("redaction: id %s, err %v", redacted.EventID, err)
	}
	if evt, _ := st.GetEvent(ctx, redacted.EventID); evt == nil || evt.Redacts != first.EventID {
		t.Errorf("redaction target not saved: %+v", evt)
	}

	if _, err = ghost.GetEvent(ctx, roomID, first.EventID); err != nil {
		t.Errorf("GetEvent: %v", err)
	}
	if _, err = ghost.GetEvent(ctx, roomID, "$missing"); err == nil {
		t.Error("GetEvent for a missing event should fail")
	}
}

func TestBatchSendOrder(t *testing.T) {
	ctx := context.Background()
	c, st := newTestConnector(t)
	ghost := c.GhostIntent("alice").GetMXID()
	roomID := createRoom(t, c, ghost)

	live := &event.Event{ID: "$live", Sender: ghost, Type: event.EventMessage, Content: *text("live"), Timestamp: time.Now().UnixMilli()}
	_, err := c.BatchSend(ctx, roomID, &mautrix.ReqBeeperBatchSend{Forward: true, SendNotification: true, Events: []*event.Event{live}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	old := []*event.Event{
		{ID: "$old1", Sender: ghost, Type: event.EventMessage, Content: *text("old 1"), Timestamp: 1000},
		{ID: "$old2", Sender: ghost, Type: event.EventMessage, Content: *text("old 2"), Timestamp: 2000},
	}
	_, err = c.BatchSend(ctx, roomID, &mautrix.ReqBeeperBatchSend{Events: old, MarkReadBy: owner}, nil)
	if err != nil {
		t.Fatal(err)
	}

	order := func(eventID id.EventID) int64 {
		evt, err := st.GetEvent(ctx, eventID)
		if err != nil || evt == nil {
			t.Fatalf("GetEvent(%s): %v", eventID, err)
		}
		return evt.Unsigned.BeeperHSOrder
	}
	if !(order("$old1") < order("$old2") && order("$old2") < 0 && order("$live") > 0) {
		t.Errorf("orders: old1 %d, old2 %d, live %d", order("$old1"), order("$old2"), order("$live"))
	}
	oldEvt, _ := st.GetEvent(ctx, "$old1")
	if oldEvt.Content.Raw["org.matrix.msc2716.historical"] != true {
		t.Error("backfilled message should be marked historical")
	}
	liveEvt, _ := st.GetEvent(ctx, "$live")
	if liveEvt.Content.Raw["org.matrix.msc2716.historical"] != nil {
		t.Error("fresh live message should not be marked historical")
	}
	receipts, _ := st.ReceiptSince(ctx, 0)
	if len(receipts) != 1 || receipts[0].UserID != owner || receipts[0].EventID != "$old2" {
		t.Errorf("MarkReadBy receipt: %+v", receipts)
	}
}

func TestOwnerSettings(t *testing.T) {
	ctx := context.Background()
	c, st := newTestConnector(t)
	me, _, err := c.NewUserIntent(ctx, owner, "")
	if err != nil || !me.IsDoublePuppet() {
		t.Fatalf("NewUserIntent: %v", err)
	}
	if _, _, err = c.NewUserIntent(ctx, "@someone:else", ""); err == nil {
		t.Error("only the owner may be double puppeted")
	}
	roomID := createRoom(t, c, c.GhostIntent("alice").GetMXID())

	if err = me.MuteRoom(ctx, roomID, event.MutedForever); err != nil {
		t.Fatal(err)
	}
	var mute event.BeeperMuteEventContent
	raw, _ := st.GetAccountData(ctx, roomID, event.AccountDataBeeperMute)
	if json.Unmarshal(raw, &mute) != nil || mute.MutedUntil != -1 {
		t.Errorf("mute forever: %s", raw)
	}

	if err = me.TagRoom(ctx, roomID, event.RoomTagFavourite, true); err != nil {
		t.Fatal(err)
	}
	if err = me.TagRoom(ctx, roomID, event.RoomTagLowPriority, true); err != nil {
		t.Fatal(err)
	}
	if err = me.TagRoom(ctx, roomID, event.RoomTagLowPriority, false); err != nil {
		t.Fatal(err)
	}
	var tags event.TagEventContent
	raw, _ = st.GetAccountData(ctx, roomID, event.AccountDataRoomTags)
	if json.Unmarshal(raw, &tags) != nil || len(tags.Tags) != 1 {
		t.Errorf("tags: %s", raw)
	}

	if err = me.MarkRead(ctx, roomID, "$unknown", time.Now()); err != nil {
		t.Errorf("receipt for an unknown event should be dropped quietly: %v", err)
	}
	if receipts, _ := st.ReceiptSince(ctx, 0); len(receipts) != 0 {
		t.Errorf("unknown-event receipt was saved: %+v", receipts)
	}
	if err = me.DeleteRoom(ctx, roomID, false); err != nil {
		t.Fatal(err)
	}
	if info, _ := c.GetMemberInfo(ctx, roomID, owner); info == nil || info.Membership != event.MembershipLeave {
		t.Errorf("after DeleteRoom owner membership = %+v", info)
	}
}

func TestStateTimelineVisibility(t *testing.T) {
	ctx := context.Background()
	c, st := newTestConnector(t)
	roomID := createRoom(t, c, c.GhostIntent("alice").GetMXID())
	ghost := c.GhostIntent("alice")

	shared := map[string]any{"custom": "kept"}
	synced, err := ghost.SendState(ctx, roomID, event.StateRoomName, "", &event.Content{
		Parsed: &event.RoomNameEventContent{Name: "Synced"}, Raw: shared,
	}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := ghost.SendState(ctx, roomID, event.StateRoomName, "", &event.Content{
		Parsed: &event.RoomNameEventContent{Name: "Renamed"},
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	hidden := func(eventID id.EventID) bool {
		evt, _ := st.GetEvent(ctx, eventID)
		return evt.Content.Raw["com.beeper.exclude_from_timeline"] == true
	}
	if !hidden(synced.EventID) {
		t.Error("state without a timestamp (a sync) should be hidden from the timeline")
	}
	if hidden(changed.EventID) {
		t.Error("a real rename (with a timestamp) should show in the timeline")
	}
	if len(shared) != 1 {
		t.Errorf("the caller's map was changed: %v", shared)
	}
	if evt, _ := st.GetEvent(ctx, synced.EventID); evt.Content.Raw["custom"] != "kept" {
		t.Error("existing extra fields should be kept")
	}
}

func TestMessageStatus(t *testing.T) {
	ctx := context.Background()
	c, st := newTestConnector(t)
	roomID := createRoom(t, c, c.GhostIntent("alice").GetMXID())
	info := &bridgev2.MessageStatusEventInfo{RoomID: roomID, SourceEventID: "$mine"}

	before := st.LastPos()
	c.SendMessageStatus(ctx, &bridgev2.MessageStatus{Status: event.MessageStatusPending}, info)
	if st.LastPos() != before {
		t.Error("a pending status should not be saved")
	}
	c.SendMessageStatus(ctx, &bridgev2.MessageStatus{Status: event.MessageStatusSuccess}, info)
	if st.LastPos() != before+1 {
		t.Errorf("a success status should add exactly one event, position went %d -> %d", before, st.LastPos())
	}
}

func TestMediaRoundTrip(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestConnector(t)
	ghost := c.GhostIntent("alice")
	uri, _, err := ghost.UploadMedia(ctx, "", []byte("photo"), "a.jpg", "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}
	data, err := ghost.DownloadMedia(ctx, uri, nil)
	if err != nil || string(data) != "photo" {
		t.Errorf("DownloadMedia: %q %v", data, err)
	}
	if _, err = ghost.DownloadMedia(ctx, "mxc://other.server/abc", nil); err == nil {
		t.Error("media from another server should be refused")
	}
}

// Sending on media that came from a network (e.g. a received sticker) downloads it through FetchDirect.
func TestDownloadDirectMedia(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestConnector(t)
	uri, err := c.GenerateContentURI(ctx, networkid.MediaID("sticker-1"))
	if err != nil {
		t.Fatal(err)
	}
	ghost := c.GhostIntent("alice")
	if _, err = ghost.DownloadMedia(ctx, uri, nil); err == nil {
		t.Error("without FetchDirect, direct media can't be downloaded")
	}

	var asked string
	c.FetchDirect = func(_ context.Context, fileID string) (*os.File, error) {
		asked = fileID
		path := filepath.Join(t.TempDir(), "sticker")
		if err := os.WriteFile(path, []byte("webp"), 0o600); err != nil {
			return nil, err
		}
		return os.Open(path)
	}
	data, err := ghost.DownloadMedia(ctx, uri, nil)
	if err != nil || string(data) != "webp" {
		t.Errorf("DownloadMedia: %q %v", data, err)
	} else if parsed, _ := uri.Parse(); asked != parsed.FileID {
		t.Errorf("FetchDirect got %q, want %q", asked, parsed.FileID)
	}
}

func TestDirectMediaURIs(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestConnector(t)
	uri, err := c.GenerateContentURI(ctx, networkid.MediaID("media-1"))
	if err != nil {
		t.Fatal(err)
	}
	mediaID, err := c.ParseContentURI(ctx, uri)
	if err != nil || string(mediaID) != "media-1" {
		t.Errorf("round trip: %q %v", mediaID, err)
	}
	other := New(zerolog.Nop(), nil, nil, nil, "telegram", server, owner)
	if _, err = other.ParseContentURI(ctx, uri); err == nil {
		t.Error("telegram must not accept whatsapp's media links")
	}
	if _, err = c.ParseContentURI(ctx, "mxc://"+server+"/0123abcd"); err == nil {
		t.Error("a stored file isn't direct media")
	}
}

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.mau.fi/util/dbutil"
	"go.mau.fi/util/jsontime"
	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
)

const me = id.UserID("@me:tuimessenger.localhost")

// fixture writes a client.db like the TUI's, then opens it read-only through Open.
type fixture struct {
	t    *testing.T
	ctx  context.Context
	dir  string
	db   *database.Database
	next time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "tui"), 0o700); err != nil {
		t.Fatal(err)
	}
	raw, err := dbutil.NewFromConfig("test", dbutil.Config{PoolConfig: dbutil.PoolConfig{
		Type: "sqlite3-fk-wal", URI: "file:" + filepath.Join(dir, "tui", "client.db") + "?_txlock=immediate", MaxOpenConns: 1,
	}}, dbutil.NoopLogger)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, ctx: context.Background(), dir: dir, db: database.New(raw), next: time.UnixMilli(1_700_000_000_000)}
	if err = f.db.Upgrade(f.ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	if err = f.db.Account.Put(f.ctx, &database.Account{UserID: me, DeviceID: "TMTUI", AccessToken: "x", HomeserverURL: "http://127.0.0.1:1"}); err != nil {
		t.Fatal(err)
	}
	return f
}

// room makes a chat with members (localpart → display name) and messages from those members ("" is you).
func (f *fixture) room(roomID id.RoomID, name string, members map[string]string, msgs ...[2]string) []id.EventID {
	f.t.Helper()
	if err := f.db.Room.CreateRow(f.ctx, roomID); err != nil {
		f.t.Fatal(err)
	}
	var heroes []id.UserID
	for localpart, display := range members {
		userID := id.NewUserID(localpart, "tuimessenger.localhost")
		heroes = append(heroes, userID)
		f.state(roomID, userID, &event.MemberEventContent{Membership: event.MembershipJoin, Displayname: display})
	}
	var ids []id.EventID
	var rowIDs []database.EventRowID
	for _, m := range msgs {
		sender := me
		if m[0] != "" {
			sender = id.NewUserID(m[0], "tuimessenger.localhost")
		}
		rowID, evtID := f.insert(roomID, sender, event.EventMessage.Type, &event.MessageEventContent{MsgType: event.MsgText, Body: m[1]})
		ids, rowIDs = append(ids, evtID), append(rowIDs, rowID)
	}
	if _, err := f.db.Timeline.Append(f.ctx, roomID, rowIDs); err != nil {
		f.t.Fatal(err)
	}
	room := &database.Room{ID: roomID, Name: ptr.Ptr(name), NameQuality: database.NameQualityExplicit,
		LazyLoadSummary: &mautrix.LazyLoadSummary{Heroes: heroes, JoinedMemberCount: ptr.Ptr(len(heroes) + 2)}, SortingTimestamp: jsontime.UM(f.next),
		UnreadCounts: database.UnreadCounts{UnreadMessages: 1}}
	if len(rowIDs) > 0 {
		room.PreviewEventRowID = rowIDs[len(rowIDs)-1]
	}
	if err := f.db.Room.Update(f.ctx, room); err != nil {
		f.t.Fatal(err)
	}
	return ids
}

func (f *fixture) insert(roomID id.RoomID, sender id.UserID, evtType string, content any) (database.EventRowID, id.EventID) {
	f.t.Helper()
	raw, _ := json.Marshal(content)
	f.next = f.next.Add(time.Minute)
	evtID := id.EventID("$" + f.next.Format("150405") + string(roomID[1:4]))
	evt := &database.Event{RoomID: roomID, ID: evtID, Sender: sender, Type: evtType, Timestamp: jsontime.UM(f.next), Content: raw, Unsigned: json.RawMessage("{}")}
	var rel struct {
		RelatesTo *event.RelatesTo `json:"m.relates_to"`
	}
	_ = json.Unmarshal(raw, &rel)
	if rel.RelatesTo != nil && rel.RelatesTo.Type != "" {
		evt.RelatesTo, evt.RelationType = rel.RelatesTo.EventID, rel.RelatesTo.Type
	}
	rowID, err := f.db.Event.Insert(f.ctx, evt)
	if err != nil {
		f.t.Fatal(err)
	}
	return rowID, evtID
}

func (f *fixture) state(roomID id.RoomID, userID id.UserID, member *event.MemberEventContent) {
	f.t.Helper()
	raw, _ := json.Marshal(member)
	f.next = f.next.Add(time.Second)
	rowID, err := f.db.Event.Insert(f.ctx, &database.Event{RoomID: roomID, ID: id.EventID("$m" + userID.Localpart() + string(roomID[1:4])),
		Sender: userID, Type: event.StateMember.Type, StateKey: ptr.Ptr(userID.String()), Timestamp: jsontime.UM(f.next), Content: raw, Unsigned: json.RawMessage("{}")})
	if err != nil {
		f.t.Fatal(err)
	}
	if err = f.db.CurrentState.Set(f.ctx, roomID, event.StateMember, userID.String(), rowID, member.Membership); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) open() *Reader {
	f.t.Helper()
	r, err := Open(f.ctx, f.dir)
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { _ = r.Close() })
	return r
}

func TestChatsMessagesSearch(t *testing.T) {
	f := newFixture(t)
	maya := f.room("!maya:x", "Maya", map[string]string{"whatsapp_15550100011": "Maya"},
		[2]string{"whatsapp_15550100011", "dinner at 8?"}, [2]string{"", "sounds good"}, [2]string{"whatsapp_15550100011", "see you at the lake"})
	f.room("!mayaig:x", "maya.draws", map[string]string{"instagram_100000000031": "maya.draws"},
		[2]string{"instagram_100000000031", "new sketch"})
	f.room("!design:x", "#design", map[string]string{"slackgo_t0000000000-u0000000041": "Priya"},
		[2]string{"slackgo_t0000000000-u0000000041", "mockups are up"})
	// An edit replaces the text; the edit itself isn't a message.
	editRow, _ := f.insert("!maya:x", me, event.EventMessage.Type, map[string]any{
		"msgtype": "m.text", "body": "* sounds great", "m.new_content": map[string]any{"msgtype": "m.text", "body": "sounds great"},
		"m.relates_to": map[string]any{"rel_type": "m.replace", "event_id": maya[1]},
	})
	if _, err := f.db.Exec(f.ctx, "UPDATE event SET last_edit_rowid=$1 WHERE event_id=$2", editRow, maya[1]); err != nil {
		t.Fatal(err)
	}
	r := f.open()
	ctx := f.ctx

	chats, err := r.Chats(ctx, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(chats) != 3 || chats[0].Name != "#design" || chats[0].Network != "slack" || chats[2].Network != "whatsapp" || !chats[2].Direct {
		t.Fatalf("chats = %+v", chats)
	}
	if last := chats[2].LastMessage; last == nil || last.Sender != "Maya" || last.Text != "see you at the lake" {
		t.Fatalf("last message = %+v", last)
	}

	msgs, err := r.Messages(ctx, "!maya:x", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 3 || msgs[0].Text != "dinner at 8?" || msgs[1].Text != "sounds great" || !msgs[1].Edited || !msgs[1].FromMe || msgs[1].Sender != "you" {
		t.Fatalf("messages = %+v %+v", msgs[0], msgs[1])
	}
	if msgs, _ = r.Messages(ctx, "!maya:x", 1); len(msgs) != 1 || msgs[0].Text != "see you at the lake" {
		t.Fatalf("limit 1 = %+v", msgs)
	}

	found, err := r.Search(ctx, "lak", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Chat != "!maya:x" {
		t.Fatalf("search = %+v", found)
	}

	for query, want := range map[string]id.RoomID{"Maya": "!maya:x", "instagram:maya": "!mayaig:x", "design": "!design:x", "!mayaig:x": "!mayaig:x"} {
		if chat, err := r.Resolve(ctx, query); err != nil || chat.ID != want {
			t.Errorf("Resolve(%q) = %v, %v; want %s", query, chat, err, want)
		}
	}
	var amb *AmbiguousError
	if _, err = r.Resolve(ctx, "may"); !errors.As(err, &amb) || len(amb.Matches) != 2 {
		t.Errorf("Resolve(may) = %v, want two matches", err)
	}
	if _, err = r.Resolve(ctx, "nobody"); err == nil {
		t.Error("Resolve(nobody) found something")
	}
}

// Chats read in tuimessenger only (/seen off) count as read, like in the TUI; /clear'd chats are hidden.
func TestLocalSeenAndCleared(t *testing.T) {
	f := newFixture(t)
	maya := f.room("!maya:x", "Maya", map[string]string{"whatsapp_15550100011": "Maya"}, [2]string{"whatsapp_15550100011", "hi"})
	f.room("!old:x", "Old", map[string]string{"telegram_10000000021": "Old"}, [2]string{"telegram_10000000021", "bye"})
	put := func(roomID id.RoomID, evtType event.Type, content any) {
		raw, _ := json.Marshal(content)
		if _, err := f.db.AccountData.PutRoom(f.ctx, me, roomID, evtType, raw); err != nil {
			t.Fatal(err)
		}
	}
	put("!maya:x", accountDataSeen, map[string]any{"event_id": maya[0]})
	put("!old:x", accountDataCleared, map[string]any{"ts": f.next.UnixMilli()})
	r := f.open()
	all, err := r.Chats(f.ctx, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].ID != "!maya:x" || all[0].Unread != 0 {
		t.Fatalf("chats = %+v", all)
	}
	if unread, _ := r.Chats(f.ctx, true, 0); len(unread) != 0 {
		t.Fatalf("unread = %+v", unread)
	}
}

func TestOpenWithoutClient(t *testing.T) {
	if _, err := Open(context.Background(), t.TempDir()); !errors.Is(err, ErrNoClient) {
		t.Fatalf("err = %v", err)
	}
}

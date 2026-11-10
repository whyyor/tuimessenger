// Package agent is what `tuimessenger agent` runs on: it reads chats from the terminal client's database (client.db,
// read-only, so it works with the TUI open or closed) and sends through the backend with a login of its own.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"go.mau.fi/util/dbutil"
	_ "go.mau.fi/util/dbutil/litestream" // registers the SQLite drivers
	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
)

// These match the TUI's own account data types (internal/tui and internal/rpc/store).
var (
	accountDataAlias   = event.Type{Type: "com.whyyor.tuimessenger.alias", Class: event.AccountDataEventType}
	accountDataSeen    = event.Type{Type: "com.whyyor.tuimessenger.seen", Class: event.AccountDataEventType}
	accountDataCleared = event.Type{Type: "com.whyyor.tuimessenger.cleared", Class: event.AccountDataEventType}
)

// ErrNoClient means the terminal client was never set up in this data folder.
var ErrNoClient = errors.New("no tuimessenger client data here: run `tuimessenger tui` and log in first")

// Reader answers questions from client.db.
type Reader struct {
	db       *database.Database
	path     string
	Account  *database.Account
	nameMemo map[id.RoomID]map[id.UserID]string
}

// Open opens <dataDir>/tui/client.db read-only.
func Open(ctx context.Context, dataDir string) (*Reader, error) {
	path := filepath.Join(dataDir, "tui", "client.db")
	if _, err := os.Stat(path); err != nil {
		return nil, ErrNoClient
	}
	raw, err := dbutil.NewFromConfig("tuimessenger-agent", dbutil.Config{
		PoolConfig: dbutil.PoolConfig{Type: "sqlite3-fk-wal", URI: "file:" + path + "?mode=ro", MaxOpenConns: 2, MaxIdleConns: 1},
	}, dbutil.NoopLogger)
	if err != nil {
		return nil, fmt.Errorf("opening the client database: %w", err)
	}
	r := &Reader{db: database.New(raw), path: path, nameMemo: map[id.RoomID]map[id.UserID]string{}}
	userID, err := r.db.Account.GetFirstUserID(ctx)
	if err == nil && userID != "" {
		r.Account, err = r.db.Account.Get(ctx, userID)
	}
	if err != nil || r.Account == nil {
		_ = raw.Close()
		return nil, ErrNoClient
	}
	return r, nil
}

func (r *Reader) Close() error { return r.db.Close() }

// SyncedAt is when the client last wrote its database: data is as fresh as that.
func (r *Reader) SyncedAt() time.Time {
	var latest time.Time
	for _, p := range []string{r.path, r.path + "-wal"} {
		if st, err := os.Stat(p); err == nil && st.ModTime().After(latest) {
			latest = st.ModTime()
		}
	}
	return latest
}

// Chat is one entry of the chat list.
type Chat struct {
	ID           id.RoomID `json:"id"`
	Name         string    `json:"name"`
	Alias        string    `json:"alias,omitempty"`
	Network      string    `json:"network,omitempty"`
	Direct       bool      `json:"direct"`
	Unread       int       `json:"unread"`
	MarkedUnread bool      `json:"marked_unread,omitempty"`
	LastActivity time.Time `json:"last_activity"`
	LastMessage  *Message  `json:"last_message,omitempty"`
}

// Chats lists chats newest first, like the TUI's list (spaces and replaced rooms left out).
func (r *Reader) Chats(ctx context.Context, unreadOnly bool, limit int) ([]*Chat, error) {
	rooms, err := r.db.Room.GetBySortTS(ctx, time.Now().Add(time.Hour), 100000)
	if err != nil {
		return nil, err
	}
	var out []*Chat
	for _, room := range rooms {
		if limit > 0 && len(out) >= limit {
			break
		}
		if room.Tombstone != nil && room.Tombstone.ReplacementRoom != "" {
			continue
		}
		chat, err := r.chat(ctx, room)
		if err != nil {
			return nil, err
		}
		if chat != nil && (!unreadOnly || chat.Unread > 0 || chat.MarkedUnread) {
			out = append(out, chat)
		}
	}
	return out, nil
}

func (r *Reader) chat(ctx context.Context, room *database.Room) (*Chat, error) {
	var cleared struct {
		TS int64 `json:"ts"`
	}
	r.roomData(ctx, room.ID, accountDataCleared, &cleared)
	chat := &Chat{
		ID: room.ID, Name: ptr.Val(room.Name), Network: networkOf(room), Direct: isDirect(room),
		Unread: room.UnreadMessages, MarkedUnread: ptr.Val(room.MarkedUnread), LastActivity: room.SortingTimestamp.Time,
	}
	if chat.Name == "" {
		chat.Name = "Unnamed chat"
	}
	var alias struct {
		Alias string `json:"alias"`
	}
	r.roomData(ctx, room.ID, accountDataAlias, &alias)
	chat.Alias = alias.Alias
	if room.PreviewEventRowID != 0 {
		evt, err := r.db.Event.GetByRowID(ctx, room.PreviewEventRowID)
		if err != nil {
			return nil, err
		}
		if evt != nil {
			if cleared.TS > 0 && evt.Timestamp.UnixMilli() <= cleared.TS {
				return nil, nil // /clear'd and nothing new since: hidden in the TUI too
			}
			chat.LastMessage = r.message(ctx, evt)
			var seen struct {
				EventID   id.EventID `json:"event_id"`
				Timestamp int64      `json:"timestamp"`
			}
			// Read in tuimessenger only (/seen off): the TUI shows it as read.
			if r.roomData(ctx, room.ID, accountDataSeen, &seen) && seen.EventID != "" &&
				(seen.EventID == evt.ID || seen.Timestamp >= evt.Timestamp.UnixMilli()) {
				chat.Unread, chat.MarkedUnread = 0, false
			}
		}
	}
	return chat, nil
}

func (r *Reader) roomData(ctx context.Context, roomID id.RoomID, evtType event.Type, into any) bool {
	ad, err := r.db.AccountData.GetRoom(ctx, r.Account.UserID, roomID, evtType)
	if err != nil || ad == nil {
		return false
	}
	return json.Unmarshal(ad.Content, into) == nil
}

// isDirect: a one-to-one chat, i.e. one other person besides you and the network's bot.
func isDirect(room *database.Room) bool {
	if room.DMUserID != nil {
		return true
	}
	sum := room.LazyLoadSummary
	return sum != nil && len(sum.Heroes) == 1 && sum.JoinedMemberCount != nil && *sum.JoinedMemberCount <= 3
}

// networkOf is the network a chat is on, from its members' bridge IDs (@whatsapp_…), like the TUI.
func networkOf(room *database.Room) string {
	if room.LazyLoadSummary == nil {
		return ""
	}
	for _, hero := range room.LazyLoadSummary.Heroes {
		localpart := hero.Localpart()
		if idx := strings.IndexByte(localpart, '_'); idx > 0 {
			return strings.TrimSuffix(localpart[:idx], "go")
		}
		if trimmed := strings.TrimSuffix(localpart, "bot"); trimmed != localpart && trimmed != "" {
			return strings.TrimSuffix(trimmed, "go")
		}
	}
	return ""
}

// Message is one message, already edited and with names filled in.
type Message struct {
	ID        id.EventID     `json:"id"`
	Chat      id.RoomID      `json:"chat,omitempty"`
	Sender    string         `json:"sender"`
	FromMe    bool           `json:"from_me"`
	Time      time.Time      `json:"time"`
	Kind      string         `json:"kind"` // text, image, video, audio, file, sticker, location, notice, emote
	Text      string         `json:"text"`
	ReplyTo   id.EventID     `json:"reply_to,omitempty"`
	Edited    bool           `json:"edited,omitempty"`
	Reactions map[string]int `json:"reactions,omitempty"`
	Failed    string         `json:"failed,omitempty"`
}

// Messages returns a chat's newest messages, oldest first.
func (r *Reader) Messages(ctx context.Context, roomID id.RoomID, limit int) ([]*Message, error) {
	var out []*Message
	var before database.TimelineRowID
	for len(out) < limit {
		batch, err := r.db.Timeline.Get(ctx, roomID, 200, before)
		if err != nil {
			return nil, err
		}
		for _, evt := range batch {
			if msg := r.message(ctx, evt); msg != nil && len(out) < limit {
				out = append(out, msg)
			}
		}
		if len(batch) < 200 {
			break
		}
		before = batch[len(batch)-1].TimelineRowID
	}
	slices.Reverse(out)
	return out, nil
}

// Search finds messages containing every word (as word prefixes), newest first.
func (r *Reader) Search(ctx context.Context, text string, rooms []id.RoomID, limit int) ([]*Message, error) {
	var parts []string
	for _, word := range strings.Fields(text) {
		parts = append(parts, `"`+strings.ReplaceAll(word, `"`, `""`)+`"*`)
	}
	if len(parts) == 0 {
		return nil, errors.New("nothing to search for")
	}
	evts, err := r.db.Event.Search(ctx, strings.Join(parts, " "), "", rooms, nil, time.Time{}, time.Time{}, false, true, limit*2, 0)
	if err != nil {
		return nil, err
	}
	var out []*Message
	for _, evt := range evts {
		if msg := r.message(ctx, evt); msg != nil && len(out) < limit {
			msg.Chat = evt.RoomID
			out = append(out, msg)
		}
	}
	return out, nil
}

// message turns a timeline event into a Message, or nil for things that aren't messages (state, edits, reactions…).
func (r *Reader) message(ctx context.Context, evt *database.Event) *Message {
	if evt == nil || evt.RedactedBy != "" || evt.RelationType == event.RelReplace {
		return nil
	}
	evtType := evt.Type
	raw := evt.Content
	if evt.DecryptedType != "" {
		evtType, raw = evt.DecryptedType, evt.Decrypted
	}
	if evtType != event.EventMessage.Type && evtType != event.EventSticker.Type {
		return nil
	}
	var content event.MessageEventContent
	if json.Unmarshal(raw, &content) != nil {
		return nil
	}
	msg := &Message{
		ID: evt.ID, Sender: r.senderName(ctx, evt.RoomID, evt.Sender), FromMe: evt.Sender == r.Account.UserID,
		Time: evt.Timestamp.Time, Kind: kindOf(evtType, content.MsgType), Text: content.Body, Reactions: evt.Reactions,
		Failed: evt.SendError,
	}
	if content.RelatesTo != nil {
		msg.ReplyTo = content.RelatesTo.GetReplyTo()
	}
	if evt.LastEditRowID != nil && *evt.LastEditRowID != 0 {
		if edit, err := r.db.Event.GetByRowID(ctx, *evt.LastEditRowID); err == nil && edit != nil {
			editRaw := edit.Content
			if edit.DecryptedType != "" {
				editRaw = edit.Decrypted
			}
			var editContent event.MessageEventContent
			if json.Unmarshal(editRaw, &editContent) == nil && editContent.NewContent != nil {
				msg.Text, msg.Edited = editContent.NewContent.Body, true
			}
		}
	}
	return msg
}

func kindOf(evtType string, msgType event.MessageType) string {
	if evtType == event.EventSticker.Type {
		return "sticker"
	}
	switch msgType {
	case event.MsgImage:
		return "image"
	case event.MsgVideo:
		return "video"
	case event.MsgAudio:
		return "audio"
	case event.MsgFile:
		return "file"
	case event.MsgLocation:
		return "location"
	case event.MsgNotice:
		return "notice"
	case event.MsgEmote:
		return "emote"
	}
	return "text"
}

// senderName is the member's display name in that chat, "you" for yourself.
func (r *Reader) senderName(ctx context.Context, roomID id.RoomID, userID id.UserID) string {
	if userID == r.Account.UserID {
		return "you"
	}
	names := r.nameMemo[roomID]
	if names == nil {
		names = map[id.UserID]string{}
		r.nameMemo[roomID] = names
	}
	if name, ok := names[userID]; ok {
		return name
	}
	name := userID.Localpart()
	if evt, err := r.db.CurrentState.Get(ctx, roomID, event.StateMember, userID.String()); err == nil && evt != nil {
		var member event.MemberEventContent
		if json.Unmarshal(evt.Content, &member) == nil && member.Displayname != "" {
			name = member.Displayname
		}
	}
	names[userID] = name
	return name
}

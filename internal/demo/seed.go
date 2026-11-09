package demo

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	"os"
	"path/filepath"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/matrixconn"
)

// room is a seeded chat at run time: where it is and who's in it.
type room struct {
	*chat
	id     id.RoomID
	conn   *matrixconn.Connector
	ghosts []bridgev2.MatrixAPI
	next   int // which of chat.replies comes next
}

// seed creates the chats unless they're already there (the room IDs are deterministic), and returns them by room ID.
func seed(ctx context.Context, conns map[string]*matrixconn.Connector, owner id.UserID, mediaDir string) (map[id.RoomID]*room, error) {
	rooms := make(map[id.RoomID]*room)
	now := time.Now()
	for _, c := range chats() {
		conn := conns[c.network]
		r := &room{chat: c, conn: conn, id: conn.GenerateDeterministicRoomID(networkid.PortalKey{ID: networkid.PortalID(c.key)})}
		for _, p := range c.people {
			ghost := conn.GhostIntent(networkid.UserID(p.id))
			if err := ghost.SetDisplayName(ctx, p.name); err != nil {
				return nil, err
			}
			r.ghosts = append(r.ghosts, ghost)
		}
		rooms[r.id] = r
		if member, err := conn.GetMemberInfo(ctx, r.id, owner); err == nil && member != nil {
			continue // seeded on an earlier run
		}
		if err := seedRoom(ctx, r, owner, mediaDir, now); err != nil {
			return nil, fmt.Errorf("seeding %s: %w", c.key, err)
		}
	}
	return rooms, nil
}

func seedRoom(ctx context.Context, r *room, owner id.UserID, mediaDir string, now time.Time) error {
	members := []id.UserID{owner}
	for _, g := range r.ghosts {
		members = append(members, g.GetMXID())
	}
	if _, err := r.conn.BotIntent().CreateRoom(ctx, &mautrix.ReqCreateRoom{
		BeeperLocalRoomID: r.id, BeeperInitialMembers: members, Name: r.name,
	}); err != nil {
		return err
	}
	me, _, err := r.conn.NewUserIntent(ctx, owner, "")
	if err != nil {
		return err
	}
	var ids []id.EventID
	var lastRead id.EventID
	for i, m := range r.msgs {
		sender := me
		if m.from != you {
			sender = r.ghosts[m.from]
		}
		at := now.Add(-m.ago)
		content, err := messageContent(ctx, sender, r.id, m, mediaDir)
		if err != nil {
			return err
		}
		if m.reply > 0 && m.reply <= len(ids) {
			content.RelatesTo = (&event.RelatesTo{}).SetReplyTo(ids[m.reply-1])
		}
		resp, err := sender.SendMessage(ctx, r.id, event.EventMessage, &event.Content{Parsed: content}, &bridgev2.MatrixSendExtra{Timestamp: at})
		if err != nil {
			return err
		}
		ids = append(ids, resp.EventID)
		for who, emoji := range m.reacts {
			reactor := me
			if who != you {
				reactor = r.ghosts[who]
			}
			reaction := &event.ReactionEventContent{RelatesTo: event.RelatesTo{Type: event.RelAnnotation, EventID: resp.EventID, Key: emoji}}
			if _, err = reactor.SendMessage(ctx, r.id, event.EventReaction, &event.Content{Parsed: reaction},
				&bridgev2.MatrixSendExtra{Timestamp: at.Add(time.Minute)}); err != nil {
				return err
			}
		}
		if i < r.unreadAt {
			lastRead = resp.EventID
		}
	}
	if lastRead != "" {
		if err = me.MarkRead(ctx, r.id, lastRead, now.Add(-time.Minute)); err != nil {
			return err
		}
	}
	if r.readByAll && len(ids) > 0 {
		for _, g := range r.ghosts {
			_ = g.MarkRead(ctx, r.id, ids[len(ids)-1], now.Add(-time.Minute))
		}
	}
	return nil
}

// messageContent is a text, or an image/video uploaded from the media folder (text only if the file is missing).
func messageContent(ctx context.Context, sender bridgev2.MatrixAPI, roomID id.RoomID, m msg, mediaDir string) (*event.MessageEventContent, error) {
	name := m.image
	if name == "" {
		name = m.video
	}
	content := &event.MessageEventContent{MsgType: event.MsgText, Body: m.text}
	if name == "" {
		return content, nil
	}
	data, err := os.ReadFile(filepath.Join(mediaDir, name))
	if err != nil {
		return content, nil // no media downloaded: keep the text
	}
	mime := mediaFiles[name]
	uri, file, err := sender.UploadMedia(ctx, roomID, data, name, mime)
	if err != nil {
		return nil, err
	}
	content = &event.MessageEventContent{MsgType: event.MsgImage, Body: name, FileName: name, URL: uri, File: file,
		Info: &event.FileInfo{MimeType: mime, Size: len(data)}}
	if m.video != "" {
		content.MsgType = event.MsgVideo
	} else if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
		content.Info.Width, content.Info.Height = cfg.Width, cfg.Height
	}
	if m.text != "" {
		content.Body = m.text // a caption, like WhatsApp's
	}
	return content, nil
}

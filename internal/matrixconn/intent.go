package matrixconn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/store"
)

// Intent is one user acting in rooms: a contact (ghost), the bridge bot, or you (owner).
type Intent struct {
	c     *Connector
	mxid  id.UserID
	owner bool
}

var _ bridgev2.MatrixAPI = (*Intent)(nil)

var errEncryptedMedia = errors.New("encrypted media is not supported")

func (i *Intent) GetMXID() id.UserID   { return i.mxid }
func (i *Intent) IsDoublePuppet() bool { return i.owner }

func (i *Intent) SendMessage(ctx context.Context, roomID id.RoomID, evtType event.Type, content *event.Content, extra *bridgev2.MatrixSendExtra) (*mautrix.RespSendEvent, error) {
	if roomID == "" {
		return nil, errors.New("no room ID")
	}
	evt := &event.Event{
		ID:      i.c.eventIDFor(roomID, evtType, content, extra),
		RoomID:  roomID,
		Sender:  i.mxid,
		Type:    evtType,
		Content: *content,
	}
	if extra != nil && !extra.Timestamp.IsZero() {
		evt.Timestamp = extra.Timestamp.UnixMilli()
	}
	if redaction, ok := content.Parsed.(*event.RedactionEventContent); ok {
		evt.Redacts = redaction.Redacts
	}
	err := i.c.store.Write(ctx, func(ctx context.Context, w *store.Writer) error {
		if err := i.c.ensureJoined(ctx, w, roomID, i.mxid); err != nil {
			return err
		}
		return i.c.addEventOnce(ctx, w, evt)
	})
	if err != nil {
		return nil, err
	}
	i.c.log.Debug().Stringer("room_id", roomID).Stringer("sender", i.mxid).Str("type", evtType.Type).Msg("Saved event")
	return &mautrix.RespSendEvent{EventID: evt.ID}, nil
}

func (i *Intent) SendState(ctx context.Context, roomID id.RoomID, evtType event.Type, stateKey string, content *event.Content, ts time.Time) (*mautrix.RespSendEvent, error) {
	evt := &event.Event{
		ID:       i.c.randomEventID(),
		RoomID:   roomID,
		Sender:   i.mxid,
		Type:     evtType,
		StateKey: &stateKey,
		Content:  *content,
	}
	if !ts.IsZero() {
		evt.Timestamp = ts.UnixMilli()
	}
	err := i.c.store.Write(ctx, func(ctx context.Context, w *store.Writer) error {
		// A join without a name gets the user's known profile, like a real server would.
		if member, ok := content.Parsed.(*event.MemberEventContent); ok && member.Membership == event.MembershipJoin && member.Displayname == "" {
			profile, err := i.c.store.GetProfile(ctx, id.UserID(stateKey))
			if err != nil {
				return err
			}
			filled := *member
			filled.Displayname, filled.AvatarURL = profile.Displayname, profile.AvatarURL
			evt.Content.Parsed = &filled
		}
		_, err := w.AddEvent(ctx, evt)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &mautrix.RespSendEvent{EventID: evt.ID}, nil
}

// MarkRead saves "this user read up to eventID"; receipts for events we never saw are dropped.
func (i *Intent) MarkRead(ctx context.Context, roomID id.RoomID, eventID id.EventID, ts time.Time) error {
	return i.c.store.Write(ctx, func(ctx context.Context, w *store.Writer) error {
		target, err := i.c.store.GetEvent(ctx, eventID)
		if err != nil || target == nil {
			return err
		}
		receipt := &store.Receipt{RoomID: roomID, UserID: i.mxid, Type: event.ReceiptTypeRead, EventID: eventID}
		if !ts.IsZero() {
			receipt.Timestamp = ts.UnixMilli()
		}
		return w.SetReceipt(ctx, receipt)
	})
}

func (i *Intent) MarkUnread(ctx context.Context, roomID id.RoomID, unread bool) error {
	return i.c.store.Write(ctx, func(ctx context.Context, w *store.Writer) error {
		return w.SetAccountData(ctx, roomID, event.AccountDataMarkedUnread, &event.MarkedUnreadEventContent{Unread: unread})
	})
}

// Typing is short-lived, so it isn't stored; the /sync server (next milestone) will keep it in memory.
func (i *Intent) MarkTyping(ctx context.Context, roomID id.RoomID, typingType bridgev2.TypingType, timeout time.Duration) error {
	return nil
}

func (i *Intent) DownloadMedia(ctx context.Context, uri id.ContentURIString, file *event.EncryptedFileInfo) ([]byte, error) {
	if file != nil {
		return nil, errEncryptedMedia
	}
	f, err := i.openMedia(uri)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func (i *Intent) DownloadMediaToFile(ctx context.Context, uri id.ContentURIString, file *event.EncryptedFileInfo, writable bool, callback func(*os.File) error) error {
	if file != nil {
		return errEncryptedMedia
	}
	f, err := i.openMedia(uri)
	if err != nil {
		return err
	}
	defer f.Close()
	if !writable {
		return callback(f)
	}
	// The callback may change the file, so give it a copy instead of our stored original.
	tmp, err := os.CreateTemp("", "tuimessenger-media-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err = io.Copy(tmp, f); err != nil {
		return err
	}
	if _, err = tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return callback(tmp)
}

func (i *Intent) openMedia(uri id.ContentURIString) (*os.File, error) {
	parsed, err := uri.Parse()
	if err != nil {
		return nil, err
	}
	if parsed.Homeserver != i.c.serverName {
		return nil, fmt.Errorf("media %s is not stored here", uri)
	}
	return i.c.media.Open(parsed.FileID)
}

func (i *Intent) UploadMedia(ctx context.Context, roomID id.RoomID, data []byte, fileName, mimeType string) (id.ContentURIString, *event.EncryptedFileInfo, error) {
	mediaID, err := i.c.media.Save(data)
	if err != nil {
		return "", nil, err
	}
	return i.contentURI(mediaID), nil, nil
}

func (i *Intent) UploadMediaStream(ctx context.Context, roomID id.RoomID, size int64, requireFile bool, cb bridgev2.FileStreamCallback) (id.ContentURIString, *event.EncryptedFileInfo, error) {
	tmp, err := i.c.media.TempFile()
	if err != nil {
		return "", nil, err
	}
	res, err := cb(tmp)
	_ = tmp.Close()
	if err != nil {
		_ = os.Remove(tmp.Name())
		return "", nil, bridgev2.CallbackError{Type: "write", Wrapped: err}
	}
	path := tmp.Name()
	if res != nil && res.ReplacementFile != "" {
		_ = os.Remove(tmp.Name())
		path = res.ReplacementFile
	}
	mediaID, err := i.c.media.SaveFile(path)
	if err != nil {
		return "", nil, err
	}
	return i.contentURI(mediaID), nil, nil
}

func (i *Intent) contentURI(mediaID string) id.ContentURIString {
	return id.ContentURI{Homeserver: i.c.serverName, FileID: mediaID}.CUString()
}

func (i *Intent) SetDisplayName(ctx context.Context, name string) error {
	return i.updateProfile(ctx, func(p *store.Profile) { p.Displayname = name })
}

func (i *Intent) SetAvatarURL(ctx context.Context, avatarURL id.ContentURIString) error {
	return i.updateProfile(ctx, func(p *store.Profile) { p.AvatarURL = avatarURL })
}

// updateProfile saves the new profile and, if it changed, refreshes the member event in every room the user is in.
func (i *Intent) updateProfile(ctx context.Context, change func(*store.Profile)) error {
	return i.c.store.Write(ctx, func(ctx context.Context, w *store.Writer) error {
		profile, err := i.c.store.GetProfile(ctx, i.mxid)
		if err != nil {
			return err
		}
		change(&profile)
		changed, err := w.SetProfile(ctx, i.mxid, profile)
		if err != nil || !changed {
			return err
		}
		memberships, err := i.c.store.GetStateByKey(ctx, event.StateMember, i.mxid.String())
		if err != nil {
			return err
		}
		for _, evt := range memberships {
			if evt.Content.ParseRaw(event.StateMember) != nil || evt.Content.AsMember().Membership != event.MembershipJoin {
				continue
			}
			if err = i.c.addMember(ctx, w, evt.RoomID, i.mxid, event.MembershipJoin, profile, true); err != nil {
				return err
			}
		}
		return nil
	})
}

func (i *Intent) SetExtraProfileMeta(ctx context.Context, data any) error { return nil }
func (i *Intent) SetProfile(ctx context.Context, data any) error          { return nil }

// CreateRoom writes the room's first events: create, power levels, the initial state and everyone's join.
func (i *Intent) CreateRoom(ctx context.Context, req *mautrix.ReqCreateRoom) (id.RoomID, error) {
	roomID := req.BeeperLocalRoomID
	if roomID == "" {
		roomID = i.c.randomRoomID()
	}
	members := req.BeeperInitialMembers
	if members == nil {
		members = req.Invite
	}
	err := i.c.store.Write(ctx, func(ctx context.Context, w *store.Writer) error {
		if existing, err := i.c.store.GetState(ctx, roomID, event.StateCreate, ""); err != nil || existing != nil {
			return err
		}
		creation := req.CreationContent
		if creation == nil {
			creation = make(map[string]any)
		}
		state := []*event.Event{{Type: event.StateCreate, Content: event.Content{Raw: creation}}}
		if req.PowerLevelOverride != nil {
			state = append(state, &event.Event{Type: event.StatePowerLevels, Content: event.Content{Parsed: req.PowerLevelOverride}})
		}
		if req.Name != "" {
			state = append(state, &event.Event{Type: event.StateRoomName, Content: event.Content{Parsed: &event.RoomNameEventContent{Name: req.Name}}})
		}
		if req.Topic != "" {
			state = append(state, &event.Event{Type: event.StateTopic, Content: event.Content{Parsed: &event.TopicEventContent{Topic: req.Topic}}})
		}
		for n, evt := range append(state, req.InitialState...) {
			saved := *evt
			saved.ID, saved.RoomID, saved.Sender = i.c.randomEventID(), roomID, i.mxid
			if saved.StateKey == nil {
				saved.StateKey = new("")
			}
			if _, err := w.AddEvent(ctx, &saved); err != nil {
				return err
			}
			if n == 0 {
				// The creator joins right after the create event, before anything else.
				if err := i.c.ensureJoined(ctx, w, roomID, i.mxid); err != nil {
					return err
				}
			}
		}
		for _, member := range members {
			if err := i.c.ensureJoined(ctx, w, roomID, member); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	i.c.log.Debug().Stringer("room_id", roomID).Str("name", req.Name).Int("members", len(members)).Msg("Created room")
	return roomID, nil
}

// DeleteRoom makes you leave the room, so it disappears from your chat list.
func (i *Intent) DeleteRoom(ctx context.Context, roomID id.RoomID, puppetsOnly bool) error {
	return i.c.store.Write(ctx, func(ctx context.Context, w *store.Writer) error {
		profile, err := i.c.store.GetProfile(ctx, i.c.owner)
		if err != nil {
			return err
		}
		return i.c.addMember(ctx, w, roomID, i.c.owner, event.MembershipLeave, profile, false)
	})
}

func (i *Intent) EnsureJoined(ctx context.Context, roomID id.RoomID, params ...bridgev2.EnsureJoinedParams) error {
	return i.c.store.Write(ctx, func(ctx context.Context, w *store.Writer) error {
		return i.c.ensureJoined(ctx, w, roomID, i.mxid)
	})
}

// Rooms here are joined automatically, so invites aren't needed.
func (i *Intent) EnsureInvited(ctx context.Context, roomID id.RoomID, userID id.UserID) error {
	return nil
}

func (i *Intent) TagRoom(ctx context.Context, roomID id.RoomID, tag event.RoomTag, isTagged bool) error {
	return i.c.store.Write(ctx, func(ctx context.Context, w *store.Writer) error {
		raw, err := i.c.store.GetAccountData(ctx, roomID, event.AccountDataRoomTags)
		if err != nil {
			return err
		}
		var content event.TagEventContent
		if raw != nil {
			if err = json.Unmarshal(raw, &content); err != nil {
				return fmt.Errorf("failed to read room tags: %w", err)
			}
		}
		if content.Tags == nil {
			content.Tags = make(event.Tags)
		}
		if isTagged {
			content.Tags[tag] = event.TagMetadata{}
		} else {
			delete(content.Tags, tag)
		}
		return w.SetAccountData(ctx, roomID, event.AccountDataRoomTags, &content)
	})
}

// MuteRoom saves com.beeper.mute, the same setting the gomuks fork writes with /mute.
func (i *Intent) MuteRoom(ctx context.Context, roomID id.RoomID, until time.Time) error {
	content := &event.BeeperMuteEventContent{}
	switch {
	case until.IsZero():
		// Not muted.
	case !until.Before(event.MutedForever):
		content.MutedUntil = -1
	default:
		content.MutedUntil = until.UnixMilli()
	}
	return i.c.store.Write(ctx, func(ctx context.Context, w *store.Writer) error {
		return w.SetAccountData(ctx, roomID, event.AccountDataBeeperMute, content)
	})
}

func (i *Intent) GetEvent(ctx context.Context, roomID id.RoomID, eventID id.EventID) (*event.Event, error) {
	evt, err := i.c.store.GetEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if evt == nil || evt.RoomID != roomID {
		return nil, mautrix.MNotFound.WithMessage("event %s not found", eventID)
	}
	return evt.Event, nil
}

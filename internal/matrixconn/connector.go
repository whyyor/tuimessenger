// Package matrixconn is our stand-in for a Matrix homeserver: bridges talk to it, and it saves everything in the store.
package matrixconn

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/status"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/media"
	"github.com/whyyor/tuimessenger/internal/store"
)

// Connector is what one network's bridge sees as "the Matrix server".
type Connector struct {
	br         *bridgev2.Bridge
	log        zerolog.Logger
	store      *store.Store
	media      *media.Store
	bridgeKey  string
	serverName string
	owner      id.UserID
	bot        *Intent
}

var (
	_ bridgev2.MatrixConnector                       = (*Connector)(nil)
	_ bridgev2.MatrixConnectorWithArbitraryRoomState = (*Connector)(nil)
)

// New creates the connector for one network; bridgeKey (e.g. "whatsapp") prefixes its ghost and bot IDs.
func New(log zerolog.Logger, st *store.Store, med *media.Store, bridgeKey, serverName string, owner id.UserID) *Connector {
	c := &Connector{
		log:        log.With().Str("component", "matrix").Logger(),
		store:      st,
		media:      med,
		bridgeKey:  bridgeKey,
		serverName: serverName,
		owner:      owner,
	}
	c.bot = &Intent{c: c, mxid: id.NewUserID(bridgeKey+"bot", serverName)}
	return c
}

func (c *Connector) Init(br *bridgev2.Bridge) { c.br = br }
func (c *Connector) PreStop()                 {}
func (c *Connector) Stop()                    {}
func (c *Connector) ServerName() string       { return c.serverName }

func (c *Connector) Start(ctx context.Context) error {
	name := c.br.Network.GetName().DisplayName + " bridge bot"
	return c.store.Write(ctx, func(ctx context.Context, w *store.Writer) error {
		_, err := w.SetProfile(ctx, c.bot.mxid, store.Profile{Displayname: name})
		return err
	})
}

func (c *Connector) GetCapabilities() *bridgev2.MatrixCapabilities {
	// BatchSending must be true, otherwise bridgev2 skips history backfill.
	return &bridgev2.MatrixCapabilities{AutoJoinInvites: true, BatchSending: true}
}

func (c *Connector) GhostIntent(userID networkid.UserID) bridgev2.MatrixAPI {
	return &Intent{c: c, mxid: c.FormatGhostMXID(userID)}
}

func (c *Connector) BotIntent() bridgev2.MatrixAPI {
	return c.bot
}

// NewUserIntent is how bridgev2 asks to act as you, so messages you send from your phone appear as yours.
func (c *Connector) NewUserIntent(ctx context.Context, userID id.UserID, accessToken string) (bridgev2.MatrixAPI, string, error) {
	if userID != c.owner {
		return nil, "", fmt.Errorf("can't act as %s, only as the owner", userID)
	}
	return &Intent{c: c, mxid: c.owner, owner: true}, accessToken, nil
}

func (c *Connector) SendBridgeStatus(ctx context.Context, state *status.BridgeState) error {
	c.log.Info().
		Str("state", string(state.StateEvent)).
		Str("error", string(state.Error)).
		Str("remote_name", state.RemoteName).
		Msg("Bridge status")
	return nil
}

// SendMessageStatus saves "delivered" / "failed" for a message you sent; the gomuks fork shows it as ticks.
func (c *Connector) SendMessageStatus(ctx context.Context, st *bridgev2.MessageStatus, evt *bridgev2.MessageStatusEventInfo) {
	// "Pending" changes nothing the client shows, so don't spend a disk write on it.
	if evt.RoomID == "" || evt.SourceEventID == "" || st.Status == event.MessageStatusPending {
		return
	}
	content := &event.Content{Parsed: st.ToMSSEvent(evt)}
	if _, err := c.bot.SendMessage(ctx, evt.RoomID, event.BeeperMessageStatus, content, nil); err != nil {
		c.log.Err(err).Stringer("event_id", evt.SourceEventID).Msg("Failed to save message status")
	}
}

// Direct media (fetching files from the network only when opened) comes later.
func (c *Connector) GenerateContentURI(ctx context.Context, mediaID networkid.MediaID) (id.ContentURIString, error) {
	return "", bridgev2.ErrDirectMediaNotEnabled
}

func (c *Connector) ParseContentURI(ctx context.Context, uri id.ContentURIString) (networkid.MediaID, error) {
	return nil, bridgev2.ErrDirectMediaNotEnabled
}

func (c *Connector) GetStateEvent(ctx context.Context, roomID id.RoomID, evtType event.Type, stateKey string) (*event.Event, error) {
	evt, err := c.store.GetState(ctx, roomID, evtType, stateKey)
	if err != nil || evt == nil {
		return nil, err
	}
	return evt.Event, nil
}

func (c *Connector) GetPowerLevels(ctx context.Context, roomID id.RoomID) (*event.PowerLevelsEventContent, error) {
	evt, err := c.parsedState(ctx, roomID, event.StatePowerLevels, "")
	if evt == nil {
		return nil, err
	}
	return evt.Content.AsPowerLevels(), nil
}

func (c *Connector) GetMemberInfo(ctx context.Context, roomID id.RoomID, userID id.UserID) (*event.MemberEventContent, error) {
	evt, err := c.parsedState(ctx, roomID, event.StateMember, userID.String())
	if evt == nil {
		return nil, err
	}
	return evt.Content.AsMember(), nil
}

func (c *Connector) GetMembers(ctx context.Context, roomID id.RoomID) (map[id.UserID]*event.MemberEventContent, error) {
	events, err := c.store.GetStateOfType(ctx, roomID, event.StateMember)
	if err != nil {
		return nil, err
	}
	members := make(map[id.UserID]*event.MemberEventContent, len(events))
	for _, evt := range events {
		if err = evt.Content.ParseRaw(event.StateMember); err != nil {
			return nil, fmt.Errorf("failed to parse member event %s: %w", evt.ID, err)
		}
		members[id.UserID(*evt.StateKey)] = evt.Content.AsMember()
	}
	return members, nil
}

// parsedState reads a state event and turns its JSON into the matching Go struct; nil if it doesn't exist.
func (c *Connector) parsedState(ctx context.Context, roomID id.RoomID, evtType event.Type, stateKey string) (*store.Event, error) {
	evt, err := c.store.GetState(ctx, roomID, evtType, stateKey)
	if err != nil || evt == nil {
		return nil, err
	}
	if err = evt.Content.ParseRaw(evtType); err != nil {
		return nil, fmt.Errorf("failed to parse %s in %s: %w", evtType.Type, roomID, err)
	}
	return evt, nil
}

// BatchSend saves many messages at once: new ones (Forward) or older history that goes before everything else.
func (c *Connector) BatchSend(ctx context.Context, roomID id.RoomID, req *mautrix.ReqBeeperBatchSend, extras []*bridgev2.MatrixSendExtra) (*mautrix.RespBeeperBatchSend, error) {
	notify := req.Forward && req.SendNotification && req.MarkReadBy == ""
	cutoff := time.Now().Add(-time.Hour).UnixMilli()
	ids := make([]id.EventID, len(req.Events))
	for i, evt := range req.Events {
		evt.RoomID = roomID
		if evt.ID == "" {
			evt.ID = c.randomEventID()
		}
		if !notify || evt.Timestamp < cutoff {
			// Marks history, so clients don't notify for old messages.
			setRaw(&evt.Content, "org.matrix.msc2716.historical", true)
		}
		ids[i] = evt.ID
	}

	err := c.store.Write(ctx, func(ctx context.Context, w *store.Writer) error {
		var order int64
		if !req.Forward {
			lowest, _, err := c.store.RoomOrderRange(ctx, roomID)
			if err != nil {
				return err
			}
			// Live events have positive orders, so older history counts down from below zero.
			order = min(lowest, 0) - int64(len(req.Events))
		}
		joined := make(map[id.UserID]bool)
		for _, evt := range req.Events {
			if !joined[evt.Sender] {
				if err := c.ensureJoined(ctx, w, roomID, evt.Sender); err != nil {
					return err
				}
				joined[evt.Sender] = true
			}
			if !req.Forward {
				evt.Unsigned.BeeperHSOrder = order
				order++
			}
			if err := c.addEventOnce(ctx, w, evt); err != nil {
				return err
			}
		}
		if req.MarkReadBy != "" && len(req.Events) > 0 {
			last := req.Events[len(req.Events)-1]
			return w.SetReceipt(ctx, &store.Receipt{RoomID: roomID, UserID: req.MarkReadBy, Type: event.ReceiptTypeRead, EventID: last.ID})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	c.log.Debug().Stringer("room_id", roomID).Int("events", len(ids)).Bool("forward", req.Forward).Msg("Saved batch")
	return &mautrix.RespBeeperBatchSend{EventIDs: ids}, nil
}

// The helpers below run inside store.Write. Rule: never call back into bridgev2 from inside a write,
// because bridgev2 may be holding its own locks while calling us, which could deadlock.

// ensureJoined adds a join event for userID unless they're already in the room.
func (c *Connector) ensureJoined(ctx context.Context, w *store.Writer, roomID id.RoomID, userID id.UserID) error {
	current, err := c.store.GetState(ctx, roomID, event.StateMember, userID.String())
	if err != nil {
		return err
	}
	if current != nil && current.Content.ParseRaw(event.StateMember) == nil &&
		current.Content.AsMember().Membership == event.MembershipJoin {
		return nil
	}
	profile, err := c.store.GetProfile(ctx, userID)
	if err != nil {
		return err
	}
	return c.addMember(ctx, w, roomID, userID, event.MembershipJoin, profile, true)
}

// addMember saves a membership event; hidden ones are bookkeeping that clients shouldn't show in the chat.
func (c *Connector) addMember(ctx context.Context, w *store.Writer, roomID id.RoomID, userID id.UserID, membership event.Membership, profile store.Profile, hidden bool) error {
	key := userID.String()
	content := event.Content{Parsed: &event.MemberEventContent{
		Membership:  membership,
		Displayname: profile.Displayname,
		AvatarURL:   profile.AvatarURL,
	}}
	if hidden {
		setRaw(&content, "com.beeper.exclude_from_timeline", true)
	}
	_, err := w.AddEvent(ctx, &event.Event{
		ID: c.randomEventID(), RoomID: roomID, Sender: userID,
		Type: event.StateMember, StateKey: &key, Content: content,
	})
	return err
}

// addEventOnce saves an event unless one with the same ID exists, so a resent message isn't stored twice.
func (c *Connector) addEventOnce(ctx context.Context, w *store.Writer, evt *event.Event) error {
	existing, err := c.store.GetEvent(ctx, evt.ID)
	if err != nil || existing != nil {
		return err
	}
	_, err = w.AddEvent(ctx, evt)
	return err
}

// setRaw adds an extra JSON field to an event's content without losing what's already there.
func setRaw(content *event.Content, key string, value any) {
	if content.Raw == nil && content.Parsed == nil && len(content.VeryRaw) > 0 {
		_ = json.Unmarshal(content.VeryRaw, &content.Raw)
	}
	// Copy first: the map may belong to bridgev2, and changing it there would surprise it.
	raw := maps.Clone(content.Raw)
	if raw == nil {
		raw = make(map[string]any)
	}
	raw[key] = value
	content.Raw = raw
}

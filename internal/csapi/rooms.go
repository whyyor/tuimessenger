package csapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/store"
)

// How long a message may take to reach the bridge, and how long scrolling up may wait for older history.
const (
	forwardTimeout  = 2 * time.Minute
	loadOlderTimout = 30 * time.Second
)

func (s *Server) send(r *http.Request) (any, error) {
	body, err := readBody(r)
	if err != nil {
		return nil, err
	}
	evtType := event.Type{Type: r.PathValue("type"), Class: event.MessageEventType}
	eventID, err := s.sendFromClient(r.Context(), id.RoomID(r.PathValue("roomID")), evtType, nil, r.PathValue("txnID"), body, "")
	if err != nil {
		return nil, err
	}
	return &mautrix.RespSendEvent{EventID: eventID}, nil
}

func (s *Server) putState(r *http.Request) (any, error) {
	body, err := readBody(r)
	if err != nil {
		return nil, err
	}
	evtType := event.Type{Type: r.PathValue("type"), Class: event.StateEventType}
	stateKey := r.PathValue("stateKey")
	eventID, err := s.sendFromClient(r.Context(), id.RoomID(r.PathValue("roomID")), evtType, &stateKey, "", body, "")
	if err != nil {
		return nil, err
	}
	return &mautrix.RespSendEvent{EventID: eventID}, nil
}

func (s *Server) redact(r *http.Request) (any, error) {
	body, err := readBody(r)
	if err != nil {
		return nil, err
	}
	target := id.EventID(r.PathValue("eventID"))
	content := map[string]any{}
	if len(body) > 0 {
		if err = parseJSON(body, &content); err != nil {
			return nil, err
		}
	}
	// Newer room versions keep the target in the content too.
	content["redacts"] = target
	body, _ = json.Marshal(content)
	eventID, err := s.sendFromClient(r.Context(), id.RoomID(r.PathValue("roomID")), event.EventRedaction, nil, r.PathValue("txnID"), body, target)
	if err != nil {
		return nil, err
	}
	return &mautrix.RespSendEvent{EventID: eventID}, nil
}

// sendFromClient saves an event you sent, so it shows right away, then hands it to the room's bridge.
func (s *Server) sendFromClient(ctx context.Context, roomID id.RoomID, evtType event.Type, stateKey *string, txnID string, body []byte, redacts id.EventID) (id.EventID, error) {
	deviceID, _ := ctx.Value(deviceKey).(id.DeviceID)
	txnKey := ""
	if txnID != "" {
		// A retried request (same transaction) gets the first attempt's event instead of sending again.
		txnKey = string(deviceID) + "/" + txnID
		if existing, err := s.Store.GetTransaction(ctx, txnKey); err != nil || existing != "" {
			return existing, err
		}
	}
	if !json.Valid(body) || len(body) == 0 || body[0] != '{' {
		return "", mautrix.MNotJSON.WithMessage("Event content must be a JSON object")
	}
	if membership, err := s.ownMembership(ctx, roomID); err != nil {
		return "", err
	} else if membership != event.MembershipJoin {
		return "", mautrix.MForbidden.WithMessage("You're not in this room")
	}
	evt := &event.Event{
		ID:        id.EventID("$" + randomString(24) + ":" + s.ServerName),
		RoomID:    roomID,
		Sender:    s.Owner,
		Type:      evtType,
		StateKey:  stateKey,
		Redacts:   redacts,
		Timestamp: time.Now().UnixMilli(),
		Content:   event.Content{VeryRaw: body},
	}
	evt.Unsigned.TransactionID = txnID
	err := s.Store.Write(ctx, func(ctx context.Context, w *store.Writer) error {
		if _, err := w.AddEvent(ctx, evt); err != nil {
			return err
		}
		if txnKey != "" {
			return w.SaveTransaction(ctx, txnKey, evt.ID)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	go s.forward(evt)
	return evt.ID, nil
}

// forward hands an event to the bridge in the background. Delivery results come back as message status events.
func (s *Server) forward(evt *event.Event) {
	if evt.Type == event.StateMember {
		// Leaving goes through leave(), which waits for the network's answer; other membership changes stay local.
		return
	}
	ctx, cancel := context.WithTimeout(s.Log.WithContext(context.Background()), forwardTimeout)
	defer cancel()
	bridged := *evt
	bridged.Content = event.Content{VeryRaw: evt.Content.VeryRaw}
	if err := bridged.Content.ParseRaw(bridged.Type); err != nil && !errors.Is(err, event.ErrUnsupportedContentType) {
		s.Log.Warn().Err(err).Stringer("event_id", evt.ID).Msg("Failed to parse event for the bridge")
	}
	if err := s.Bridges.Queue(ctx, &bridged); err != nil {
		s.Log.Err(err).Stringer("event_id", evt.ID).Msg("Failed to hand event to the bridge")
		if evt.StateKey == nil {
			s.saveFailure(ctx, evt, err)
		}
	}
}

// saveFailure stores a "failed" delivery status, which the tuimessenger client shows as a red mark on the message.
func (s *Server) saveFailure(ctx context.Context, evt *event.Event, cause error) {
	content := &event.BeeperMessageStatusEventContent{
		RelatesTo: event.RelatesTo{Type: event.RelReference, EventID: evt.ID},
		Status:    event.MessageStatusFail,
		Reason:    event.MessageStatusGenericError,
		Message:   cause.Error(),
	}
	status := &event.Event{
		ID:      id.EventID("$" + randomString(24) + ":" + s.ServerName),
		RoomID:  evt.RoomID,
		Sender:  s.Owner,
		Type:    event.BeeperMessageStatus,
		Content: event.Content{Parsed: content},
	}
	err := s.Store.Write(ctx, func(ctx context.Context, w *store.Writer) error {
		_, err := w.AddEvent(ctx, status)
		return err
	})
	if err != nil {
		s.Log.Err(err).Msg("Failed to save failed message status")
	}
}

// messages pages backwards through a room's history; when the store runs out, it asks the network for more.
func (s *Server) messages(r *http.Request) (any, error) {
	ctx := r.Context()
	roomID := id.RoomID(r.PathValue("roomID"))
	query := r.URL.Query()
	from := query.Get("from")
	resp := &mautrix.RespMessages{Start: from, Chunk: []*event.Event{}, State: []*event.Event{}}
	if query.Get("dir") != "b" {
		// Only scrolling up is needed: new messages arrive through /sync.
		return resp, nil
	}
	limit, _ := strconv.Atoi(query.Get("limit"))
	limit = min(max(limit, 1), maxTimelineLimit)
	if limit == 1 && query.Get("limit") == "" {
		limit = 10
	}
	before := parseOrderToken(from)

	history, err := s.Store.RoomHistory(ctx, roomID, before, limit)
	if err != nil {
		return nil, err
	}
	networkHasMore := false
	if len(history) < limit {
		loadCtx, cancel := context.WithTimeout(ctx, loadOlderTimout)
		networkHasMore, err = s.Bridges.LoadOlder(loadCtx, roomID)
		cancel()
		if err != nil {
			s.Log.Warn().Err(err).Stringer("room_id", roomID).Msg("Failed to load older history from the network")
		} else if history, err = s.Store.RoomHistory(ctx, roomID, before, limit); err != nil {
			return nil, err
		}
	}

	senders := make(map[id.UserID]bool)
	for _, evt := range history {
		resp.Chunk = append(resp.Chunk, evt.Event)
		senders[evt.Sender] = true
	}
	if len(history) > 0 && (len(history) == limit || networkHasMore) {
		resp.End = orderToken(history[len(history)-1])
	}
	// The senders' member events, so the client can show names (it asked for members lazily).
	for sender := range senders {
		member, err := s.Store.GetState(ctx, roomID, event.StateMember, sender.String())
		if err != nil {
			return nil, err
		} else if member != nil {
			resp.State = append(resp.State, member.Event)
		}
	}
	return resp, nil
}

func (s *Server) getEvent(r *http.Request) (any, error) {
	evt, err := s.Store.GetEvent(r.Context(), id.EventID(r.PathValue("eventID")))
	if err != nil {
		return nil, err
	} else if evt == nil || evt.RoomID != id.RoomID(r.PathValue("roomID")) {
		return nil, mautrix.MNotFound.WithMessage("Event not found")
	}
	return evt.Event, nil
}

func (s *Server) getRoomState(r *http.Request) (any, error) {
	state, err := s.Store.RoomState(r.Context(), id.RoomID(r.PathValue("roomID")))
	if err != nil {
		return nil, err
	}
	return plain(state), nil
}

func (s *Server) getStateEvent(r *http.Request) (any, error) {
	evtType := event.Type{Type: r.PathValue("type"), Class: event.StateEventType}
	evt, err := s.Store.GetState(r.Context(), id.RoomID(r.PathValue("roomID")), evtType, r.PathValue("stateKey"))
	if err != nil {
		return nil, err
	} else if evt == nil {
		return nil, mautrix.MNotFound.WithMessage("State event not found")
	}
	return evt.Content.VeryRaw, nil
}

func (s *Server) members(r *http.Request) (any, error) {
	events, err := s.Store.GetStateOfType(r.Context(), id.RoomID(r.PathValue("roomID")), event.StateMember)
	if err != nil {
		return nil, err
	}
	want := event.Membership(r.URL.Query().Get("membership"))
	resp := &mautrix.RespMembers{Chunk: []*event.Event{}}
	for _, evt := range events {
		if want == "" || membershipOf(evt) == want {
			resp.Chunk = append(resp.Chunk, evt.Event)
		}
	}
	return resp, nil
}

type joinedMember struct {
	DisplayName string              `json:"display_name,omitempty"`
	AvatarURL   id.ContentURIString `json:"avatar_url,omitempty"`
}

func (s *Server) joinedMembers(r *http.Request) (any, error) {
	events, err := s.Store.GetStateOfType(r.Context(), id.RoomID(r.PathValue("roomID")), event.StateMember)
	if err != nil {
		return nil, err
	}
	joined := make(map[id.UserID]joinedMember)
	for _, evt := range events {
		var content event.MemberEventContent
		if json.Unmarshal(evt.Content.VeryRaw, &content) == nil && content.Membership == event.MembershipJoin {
			joined[id.UserID(evt.GetStateKey())] = joinedMember{DisplayName: content.Displayname, AvatarURL: content.AvatarURL}
		}
	}
	return map[string]any{"joined": joined}, nil
}

func (s *Server) joinedRooms(r *http.Request) (any, error) {
	memberships, err := s.Store.GetStateByKey(r.Context(), event.StateMember, s.Owner.String())
	if err != nil {
		return nil, err
	}
	rooms := []id.RoomID{}
	for _, m := range memberships {
		if membershipOf(m) == event.MembershipJoin {
			rooms = append(rooms, m.RoomID)
		}
	}
	return &mautrix.RespJoinedRooms{JoinedRooms: rooms}, nil
}

// receipt saves your read receipt; public ones also go to the network (blue ticks on the other phone).
func (s *Server) receipt(r *http.Request) (any, error) {
	ctx := r.Context()
	roomID := id.RoomID(r.PathValue("roomID"))
	eventID := id.EventID(r.PathValue("eventID"))
	receiptType := event.ReceiptType(r.PathValue("type"))
	if receiptType == "m.fully_read" {
		return nil, s.setFullyRead(ctx, roomID, eventID)
	}
	return nil, s.markRead(ctx, roomID, eventID, receiptType)
}

func (s *Server) readMarkers(r *http.Request) (any, error) {
	ctx := r.Context()
	roomID := id.RoomID(r.PathValue("roomID"))
	var req mautrix.ReqSetReadMarkers
	if err := readJSON(r, &req); err != nil {
		return nil, err
	}
	if req.FullyRead != "" {
		if err := s.setFullyRead(ctx, roomID, req.FullyRead); err != nil {
			return nil, err
		}
	}
	if req.Read != "" {
		if err := s.markRead(ctx, roomID, req.Read, event.ReceiptTypeRead); err != nil {
			return nil, err
		}
	}
	if req.ReadPrivate != "" {
		if err := s.markRead(ctx, roomID, req.ReadPrivate, event.ReceiptTypeReadPrivate); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func (s *Server) markRead(ctx context.Context, roomID id.RoomID, eventID id.EventID, receiptType event.ReceiptType) error {
	if receiptType != event.ReceiptTypeRead && receiptType != event.ReceiptTypeReadPrivate {
		return mautrix.MInvalidParam.WithMessage("Unknown receipt type %s", receiptType)
	}
	target, err := s.Store.GetEvent(ctx, eventID)
	if err != nil {
		return err
	} else if target == nil || target.RoomID != roomID {
		return mautrix.MNotFound.WithMessage("Event not found")
	}
	now := time.Now()
	err = s.Store.Write(ctx, func(ctx context.Context, w *store.Writer) error {
		return w.SetReceipt(ctx, &store.Receipt{RoomID: roomID, UserID: s.Owner, Type: receiptType, EventID: eventID, Timestamp: now.UnixMilli()})
	})
	if err != nil || receiptType != event.ReceiptTypeRead {
		return err
	}
	content := event.ReceiptEventContent{eventID: {event.ReceiptTypeRead: {s.Owner: {Timestamp: now}}}}
	s.forwardEphemeral(roomID, event.EphemeralEventReceipt, &content)
	return nil
}

func (s *Server) setFullyRead(ctx context.Context, roomID id.RoomID, eventID id.EventID) error {
	return s.Store.Write(ctx, func(ctx context.Context, w *store.Writer) error {
		return w.SetAccountData(ctx, roomID, event.AccountDataFullyRead, &event.FullyReadEventContent{EventID: eventID})
	})
}

func (s *Server) typingNotice(r *http.Request) (any, error) {
	if err := s.checkOwnUser(r); err != nil {
		return nil, err
	}
	var req mautrix.ReqTyping
	if err := readJSON(r, &req); err != nil {
		return nil, err
	}
	roomID := id.RoomID(r.PathValue("roomID"))
	var timeout time.Duration
	users := []id.UserID{}
	if req.Typing {
		timeout = max(time.Duration(req.Timeout)*time.Millisecond, time.Second)
		users = append(users, s.Owner)
	}
	s.Typing.Set(roomID, s.Owner, timeout)
	s.forwardEphemeral(roomID, event.EphemeralEventTyping, &event.TypingEventContent{UserIDs: users})
	return nil, nil
}

// forwardEphemeral hands a receipt or typing notice to the bridge in the background.
func (s *Server) forwardEphemeral(roomID id.RoomID, evtType event.Type, content any) {
	evt := &event.Event{Type: evtType, RoomID: roomID, Content: event.Content{Parsed: content}}
	evt.Mautrix.EventSource = event.SourceEphemeral
	go func() {
		ctx, cancel := context.WithTimeout(s.Log.WithContext(context.Background()), forwardTimeout)
		defer cancel()
		if err := s.Bridges.Queue(ctx, evt); err != nil {
			s.Log.Debug().Err(err).Str("type", evtType.Type).Msg("Bridge didn't take ephemeral event")
		}
	}()
}

// leave leaves the chat on its network first; only if that works is the room marked as left here.
// Networks that can't leave chats (e.g. Telegram here) return an error, so nothing changes silently.
func (s *Server) leave(r *http.Request) (any, error) {
	ctx := r.Context()
	roomID := id.RoomID(r.PathValue("roomID"))
	evt, err := s.ownMembershipChange(ctx, roomID, event.MembershipLeave)
	if err != nil || evt == nil {
		return nil, err
	}
	// bridgev2 works out "join -> leave" from the previous membership.
	evt.Unsigned.PrevContent = &event.Content{Parsed: &event.MemberEventContent{Membership: event.MembershipJoin}}
	queueCtx, cancel := context.WithTimeout(s.Log.WithContext(ctx), forwardTimeout)
	defer cancel()
	if err = s.Bridges.Queue(queueCtx, evt); err != nil {
		return nil, mautrix.MForbidden.WithMessage("Couldn't leave the chat on the network: %v", err)
	}
	return nil, s.saveOwnMembership(ctx, evt)
}

// join only rejoins here, e.g. a chat you left by mistake; it can't rejoin the chat on the network.
func (s *Server) join(r *http.Request) (any, error) {
	ctx := r.Context()
	roomID := id.RoomID(r.PathValue("roomID"))
	evt, err := s.ownMembershipChange(ctx, roomID, event.MembershipJoin)
	if err != nil {
		return nil, err
	} else if evt != nil {
		if err = s.saveOwnMembership(ctx, evt); err != nil {
			return nil, err
		}
	}
	return &mautrix.RespJoinRoom{RoomID: roomID}, nil
}

// ownMembershipChange builds your member event for a new membership, or returns nil if it's already that.
func (s *Server) ownMembershipChange(ctx context.Context, roomID id.RoomID, membership event.Membership) (*event.Event, error) {
	current, err := s.ownMembership(ctx, roomID)
	if err != nil {
		return nil, err
	} else if current == "" {
		return nil, mautrix.MNotFound.WithMessage("Room not found")
	} else if current == membership {
		return nil, nil
	}
	profile, err := s.Store.GetProfile(ctx, s.Owner)
	if err != nil {
		return nil, err
	}
	key := s.Owner.String()
	return &event.Event{
		ID: id.EventID("$" + randomString(24) + ":" + s.ServerName), RoomID: roomID, Sender: s.Owner,
		Type: event.StateMember, StateKey: &key, Timestamp: time.Now().UnixMilli(),
		Content: event.Content{Parsed: &event.MemberEventContent{
			Membership: membership, Displayname: profile.Displayname, AvatarURL: profile.AvatarURL,
		}},
	}, nil
}

func (s *Server) saveOwnMembership(ctx context.Context, evt *event.Event) error {
	return s.Store.Write(ctx, func(ctx context.Context, w *store.Writer) error {
		_, err := w.AddEvent(ctx, evt)
		return err
	})
}

// readBody reads a request body of at most 1 MiB, as raw bytes.
func readBody(r *http.Request) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r.Body, 1<<20))
}

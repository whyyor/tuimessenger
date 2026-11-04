package csapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/store"
)

const (
	defaultTimelineLimit = 20
	maxTimelineLimit     = 100
	maxSyncWait          = 30 * time.Second
	// A sync returns at most this many new events; the client syncs again right away for the rest.
	syncBatchSize = 2000
	maxHeroes     = 5
)

// syncToken is the client's bookmark: the store position and typing version it has seen.
type syncToken struct {
	pos, typing int64
}

func (t syncToken) String() string { return fmt.Sprintf("s%d_%d", t.pos, t.typing) }

func parseSyncToken(s string) (syncToken, bool) {
	var t syncToken
	_, err := fmt.Sscanf(s, "s%d_%d", &t.pos, &t.typing)
	return t, err == nil
}

// Room timelines are paged with "t<order>_<suborder>" tokens; "latest" means "start from the newest".
func orderToken(evt *store.Event) string {
	return fmt.Sprintf("t%d_%d", evt.Unsigned.BeeperHSOrder, evt.Unsigned.BeeperHSSuborder)
}

func parseOrderToken(s string) *store.Order {
	var o store.Order
	if _, err := fmt.Sscanf(s, "t%d_%d", &o.Order, &o.Suborder); err != nil {
		return nil
	}
	return &o
}

func (s *Server) sync(r *http.Request) (any, error) {
	ctx := r.Context()
	query := r.URL.Query()
	limit := s.timelineLimit(query.Get("filter"))
	since, ok := parseSyncToken(query.Get("since"))
	if !ok || since.pos > s.Store.LastPos() {
		return s.fullSync(ctx, limit)
	}
	timeoutMS, _ := strconv.Atoi(query.Get("timeout"))
	s.waitForChanges(ctx, since, min(time.Duration(timeoutMS)*time.Millisecond, maxSyncWait))
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.incrementalSync(ctx, since, limit)
}

// waitForChanges blocks until something new happened after since, the timeout passes, or the client leaves.
func (s *Server) waitForChanges(ctx context.Context, since syncToken, timeout time.Duration) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		// Take the channels before checking, so a change between the check and the wait isn't missed.
		storeChanged, typingChanged := s.Store.Changes(), s.Typing.Changes()
		if s.Store.LastPos() > since.pos || s.Typing.Version() > since.typing {
			return
		}
		select {
		case <-storeChanged:
		case <-typingChanged:
		case <-timer.C:
			return
		case <-ctx.Done():
			return
		}
	}
}

// fullSync is the first sync after login: every room you're in, with its recent messages.
func (s *Server) fullSync(ctx context.Context, limit int) (*mautrix.RespSync, error) {
	token := syncToken{pos: s.Store.LastPos(), typing: s.Typing.Version()}
	resp := &mautrix.RespSync{NextBatch: token.String()}
	memberships, err := s.Store.GetStateByKey(ctx, event.StateMember, s.Owner.String())
	if err != nil {
		return nil, err
	}
	for _, m := range memberships {
		switch membershipOf(m) {
		case event.MembershipJoin:
			room, err := s.fullRoom(ctx, m.RoomID, limit)
			if err != nil {
				return nil, err
			}
			addRoom(resp, m.RoomID, room)
		case event.MembershipInvite:
			state, err := s.Store.RoomState(ctx, m.RoomID)
			if err != nil {
				return nil, err
			}
			if resp.Rooms.Invite == nil {
				resp.Rooms.Invite = make(map[id.RoomID]*mautrix.SyncInvitedRoom)
			}
			resp.Rooms.Invite[m.RoomID] = &mautrix.SyncInvitedRoom{State: mautrix.SyncEventsList{Events: plain(state)}}
		}
	}
	if err = s.addReceiptsAndAccountData(ctx, resp, 0, token.pos); err != nil {
		return nil, err
	}
	for roomID, room := range resp.Rooms.Join {
		if users := s.Typing.Current(roomID); len(users) > 0 {
			room.Ephemeral.Events = append(room.Ephemeral.Events, typingEvent(users))
		}
	}
	return resp, nil
}

// fullRoom is everything a client needs for a room it hasn't seen: all state, recent messages, members summary.
func (s *Server) fullRoom(ctx context.Context, roomID id.RoomID, limit int) (*mautrix.SyncJoinedRoom, error) {
	state, err := s.Store.RoomState(ctx, roomID)
	if err != nil {
		return nil, err
	}
	history, err := s.Store.RoomHistory(ctx, roomID, nil, limit+1)
	if err != nil {
		return nil, err
	}
	room := &mautrix.SyncJoinedRoom{Summary: summarize(state, s.Owner)}
	room.Timeline.Limited = len(history) > limit
	if room.Timeline.Limited {
		history = history[:limit]
	}
	slices.Reverse(history)
	inTimeline := make(map[id.EventID]bool)
	for _, evt := range history {
		// Only current state goes in the timeline; an older value would undo the newer one in the client.
		if evt.StateKey != nil {
			if current, err := s.Store.IsCurrentState(ctx, evt); err != nil {
				return nil, err
			} else if !current {
				continue
			}
		}
		inTimeline[evt.ID] = true
		room.Timeline.Events = append(room.Timeline.Events, evt.Event)
	}
	for _, evt := range state {
		if !inTimeline[evt.ID] {
			room.State.Events = append(room.State.Events, evt.Event)
		}
	}
	room.Timeline.PrevBatch = "latest"
	if len(history) > 0 {
		room.Timeline.PrevBatch = orderToken(history[0])
	}
	return room, nil
}

// incrementalSync returns what changed after since: new events, receipts, settings and typing.
func (s *Server) incrementalSync(ctx context.Context, since syncToken, limit int) (*mautrix.RespSync, error) {
	token := syncToken{pos: s.Store.LastPos(), typing: s.Typing.Version()}
	events, err := s.Store.EventsSince(ctx, since.pos, syncBatchSize)
	if err != nil {
		return nil, err
	}
	if len(events) == syncBatchSize {
		token.pos = events[len(events)-1].Pos
	} else if len(events) > 0 {
		token.pos = max(token.pos, events[len(events)-1].Pos)
	}
	resp := &mautrix.RespSync{NextBatch: token.String()}

	var roomOrder []id.RoomID
	byRoom := make(map[id.RoomID][]*store.Event)
	for _, evt := range events {
		if _, seen := byRoom[evt.RoomID]; !seen {
			roomOrder = append(roomOrder, evt.RoomID)
		}
		byRoom[evt.RoomID] = append(byRoom[evt.RoomID], evt)
	}
	for _, roomID := range roomOrder {
		if err = s.syncRoomChanges(ctx, resp, roomID, byRoom[roomID], limit); err != nil {
			return nil, err
		}
	}
	if err = s.addReceiptsAndAccountData(ctx, resp, since.pos, token.pos); err != nil {
		return nil, err
	}
	for roomID, users := range s.Typing.ChangedSince(since.typing) {
		if room, err := s.joinedRoomEntry(ctx, resp, roomID); err != nil {
			return nil, err
		} else if room != nil {
			room.Ephemeral.Events = append(room.Ephemeral.Events, typingEvent(users))
		}
	}
	return resp, nil
}

// syncRoomChanges adds one room's new events to an incremental sync.
func (s *Server) syncRoomChanges(ctx context.Context, resp *mautrix.RespSync, roomID id.RoomID, events []*store.Event, limit int) error {
	membership, err := s.ownMembership(ctx, roomID)
	if err != nil {
		return err
	}
	ownKey := s.Owner.String()
	joinedNow, membersChanged := false, false
	for _, evt := range events {
		if evt.Type == event.StateMember {
			membersChanged = true
			if evt.GetStateKey() == ownKey && membershipOf(evt) == event.MembershipJoin {
				joinedNow = true
			}
		}
	}

	switch {
	case membership == event.MembershipLeave:
		if resp.Rooms.Leave == nil {
			resp.Rooms.Leave = make(map[id.RoomID]*mautrix.SyncLeftRoom)
		}
		left := &mautrix.SyncLeftRoom{}
		for _, evt := range events {
			if evt.Type == event.StateMember && evt.GetStateKey() == ownKey {
				left.Timeline.Events = append(left.Timeline.Events, evt.Event)
			}
		}
		resp.Rooms.Leave[roomID] = left
		return nil
	case membership != event.MembershipJoin:
		return nil
	case joinedNow:
		// A room the client has never seen (e.g. a new chat): send it whole.
		room, err := s.fullRoom(ctx, roomID, limit)
		if err != nil {
			return err
		}
		addRoom(resp, roomID, room)
		return nil
	}

	room := &mautrix.SyncJoinedRoom{}
	var timeline []*store.Event
	for _, evt := range events {
		live := evt.Unsigned.BeeperHSOrder == evt.Pos
		current := true
		if evt.StateKey != nil {
			if current, err = s.Store.IsCurrentState(ctx, evt); err != nil {
				return err
			}
		}
		switch {
		case !current:
			// Superseded within this same batch; the newer value follows.
		case evt.Hidden && evt.StateKey != nil:
			room.State.Events = append(room.State.Events, evt.Event)
		case evt.Hidden || !live:
			// Bookkeeping, or older history that arrived now: clients get that through /messages.
		default:
			timeline = append(timeline, evt)
		}
	}
	if len(timeline) > limit {
		timeline = timeline[len(timeline)-limit:]
		room.Timeline.Limited = true
	}
	for _, evt := range timeline {
		room.Timeline.Events = append(room.Timeline.Events, evt.Event)
	}
	if len(timeline) > 0 {
		room.Timeline.PrevBatch = orderToken(timeline[0])
	}
	if membersChanged {
		state, err := s.Store.RoomState(ctx, roomID)
		if err != nil {
			return err
		}
		room.Summary = summarize(state, s.Owner)
	}
	if len(room.Timeline.Events) > 0 || len(room.State.Events) > 0 || membersChanged {
		addRoom(resp, roomID, room)
	}
	return nil
}

// addReceiptsAndAccountData adds read receipts and settings that changed in (from, to].
func (s *Server) addReceiptsAndAccountData(ctx context.Context, resp *mautrix.RespSync, from, to int64) error {
	receipts, err := s.Store.ReceiptSince(ctx, from)
	if err != nil {
		return err
	}
	receiptsByRoom := make(map[id.RoomID][]*store.Receipt)
	for _, rc := range receipts {
		if rc.Pos <= to {
			receiptsByRoom[rc.RoomID] = append(receiptsByRoom[rc.RoomID], rc)
		}
	}
	for roomID, list := range receiptsByRoom {
		if room, err := s.joinedRoomEntry(ctx, resp, roomID); err != nil {
			return err
		} else if room != nil {
			room.Ephemeral.Events = append(room.Ephemeral.Events, receiptEvent(list))
		}
	}

	accountData, err := s.Store.AccountDataSince(ctx, from)
	if err != nil {
		return err
	}
	for _, ad := range accountData {
		if ad.Pos > to {
			continue
		}
		evt := &event.Event{
			Type:    event.Type{Type: ad.Type, Class: event.AccountDataEventType},
			Content: event.Content{VeryRaw: ad.Content},
		}
		if ad.RoomID == "" {
			resp.AccountData.Events = append(resp.AccountData.Events, evt)
		} else if room, err := s.joinedRoomEntry(ctx, resp, ad.RoomID); err != nil {
			return err
		} else if room != nil {
			room.AccountData.Events = append(room.AccountData.Events, evt)
		}
	}
	return nil
}

// joinedRoomEntry returns the room's entry in the response, creating it if you're in the room; nil otherwise.
func (s *Server) joinedRoomEntry(ctx context.Context, resp *mautrix.RespSync, roomID id.RoomID) (*mautrix.SyncJoinedRoom, error) {
	if room, ok := resp.Rooms.Join[roomID]; ok {
		return room, nil
	}
	membership, err := s.ownMembership(ctx, roomID)
	if err != nil || membership != event.MembershipJoin {
		return nil, err
	}
	room := &mautrix.SyncJoinedRoom{}
	addRoom(resp, roomID, room)
	return room, nil
}

func (s *Server) ownMembership(ctx context.Context, roomID id.RoomID) (event.Membership, error) {
	evt, err := s.Store.GetState(ctx, roomID, event.StateMember, s.Owner.String())
	if err != nil || evt == nil {
		return "", err
	}
	return membershipOf(evt), nil
}

func addRoom(resp *mautrix.RespSync, roomID id.RoomID, room *mautrix.SyncJoinedRoom) {
	if resp.Rooms.Join == nil {
		resp.Rooms.Join = make(map[id.RoomID]*mautrix.SyncJoinedRoom)
	}
	resp.Rooms.Join[roomID] = room
}

// summarize picks the "heroes" clients name a room after (e.g. the other person in a DM) and counts members.
// Bridge bots are left out unless nobody else is there, as the tuimessenger client reads the network from these IDs.
func summarize(state []*store.Event, owner id.UserID) mautrix.LazyLoadSummary {
	var heroes, bots []id.UserID
	joined, invited := 0, 0
	for _, evt := range state {
		if evt.Type != event.StateMember {
			continue
		}
		userID := id.UserID(evt.GetStateKey())
		switch membershipOf(evt) {
		case event.MembershipJoin:
			joined++
		case event.MembershipInvite:
			invited++
		default:
			continue
		}
		switch {
		case userID == owner:
		case isBridgeBot(userID):
			bots = append(bots, userID)
		case len(heroes) < maxHeroes:
			heroes = append(heroes, userID)
		}
	}
	if len(heroes) == 0 {
		heroes = bots
	}
	return mautrix.LazyLoadSummary{Heroes: heroes, JoinedMemberCount: &joined, InvitedMemberCount: &invited}
}

// isBridgeBot matches our bot IDs like "@whatsappbot:server" (ghosts always have a "_").
func isBridgeBot(userID id.UserID) bool {
	localpart := userID.Localpart()
	return strings.HasSuffix(localpart, "bot") && !strings.Contains(localpart, "_")
}

// membershipOf reads a member event's membership without changing the event (it may still be sent to a client).
func membershipOf(evt *store.Event) event.Membership {
	var content event.MemberEventContent
	if json.Unmarshal(evt.Content.VeryRaw, &content) != nil {
		return ""
	}
	return content.Membership
}

func receiptEvent(receipts []*store.Receipt) *event.Event {
	content := event.ReceiptEventContent{}
	for _, rc := range receipts {
		if content[rc.EventID] == nil {
			content[rc.EventID] = event.Receipts{}
		}
		if content[rc.EventID][rc.Type] == nil {
			content[rc.EventID][rc.Type] = event.UserReceipts{}
		}
		content[rc.EventID][rc.Type][rc.UserID] = event.ReadReceipt{Timestamp: time.UnixMilli(rc.Timestamp)}
	}
	return &event.Event{Type: event.EphemeralEventReceipt, Content: event.Content{Parsed: &content}}
}

func typingEvent(users []id.UserID) *event.Event {
	return &event.Event{Type: event.EphemeralEventTyping, Content: event.Content{Parsed: &event.TypingEventContent{UserIDs: users}}}
}

func plain(events []*store.Event) []*event.Event {
	out := make([]*event.Event, len(events))
	for i, evt := range events {
		out[i] = evt.Event
	}
	return out
}

// uploadFilter remembers the timeline limit from a client's sync filter; the rest of the filter is ignored.
func (s *Server) uploadFilter(r *http.Request) (any, error) {
	if err := s.checkOwnUser(r); err != nil {
		return nil, err
	}
	var filter mautrix.Filter
	if err := readJSON(r, &filter); err != nil {
		return nil, err
	}
	limit := defaultTimelineLimit
	if filter.Room != nil && filter.Room.Timeline != nil && filter.Room.Timeline.Limit > 0 {
		limit = min(filter.Room.Timeline.Limit, maxTimelineLimit)
	}
	s.filterLock.Lock()
	defer s.filterLock.Unlock()
	s.filters = append(s.filters, limit)
	return &mautrix.RespCreateFilter{FilterID: strconv.Itoa(len(s.filters) - 1)}, nil
}

// timelineLimit reads the limit from a filter ID or an inline filter, falling back to the default.
func (s *Server) timelineLimit(filter string) int {
	if strings.HasPrefix(filter, "{") {
		var f mautrix.Filter
		if json.Unmarshal([]byte(filter), &f) == nil && f.Room != nil && f.Room.Timeline != nil && f.Room.Timeline.Limit > 0 {
			return min(f.Room.Timeline.Limit, maxTimelineLimit)
		}
		return defaultTimelineLimit
	}
	n, err := strconv.Atoi(filter)
	s.filterLock.Lock()
	defer s.filterLock.Unlock()
	if err != nil || n < 0 || n >= len(s.filters) {
		return defaultTimelineLimit
	}
	return s.filters[n]
}

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// Event is a Matrix event plus its position in the store.
type Event struct {
	Pos int64
	// Hidden events are bookkeeping (e.g. a contact joining a chat) that clients shouldn't show as a line.
	Hidden bool
	*event.Event
}

const eventColumns = `pos, event_id, room_id, sender, type, state_key, content, redacts, unsigned, stream_order, stream_suborder, timestamp, hidden`

// hideMarker is the content field bridgev2 and we use to say "don't show this in the timeline".
type hideMarker struct {
	Hidden bool `json:"com.beeper.exclude_from_timeline"`
}

// AddEvent saves one event. State events also become the room's current state.
// The room order comes from evt.Unsigned.BeeperHSOrder; 0 means "newest", which uses the position.
func (w *Writer) AddEvent(ctx context.Context, evt *event.Event) (int64, error) {
	if evt.ID == "" || evt.RoomID == "" {
		return 0, errors.New("event needs an ID and a room ID")
	}
	if evt.Timestamp == 0 {
		evt.Timestamp = time.Now().UnixMilli()
	}
	content, err := json.Marshal(&evt.Content)
	if err != nil {
		return 0, fmt.Errorf("failed to encode content: %w", err)
	}
	// The order is kept in its own columns, so leave it out of the saved unsigned JSON.
	unsignedCopy := evt.Unsigned
	unsignedCopy.BeeperHSOrder, unsignedCopy.BeeperHSSuborder = 0, 0
	unsigned, err := json.Marshal(&unsignedCopy)
	if err != nil {
		return 0, fmt.Errorf("failed to encode unsigned: %w", err)
	}

	var marker hideMarker
	_ = json.Unmarshal(content, &marker)

	pos := w.nextPos()
	if evt.Unsigned.BeeperHSOrder == 0 {
		evt.Unsigned.BeeperHSOrder = pos
	}
	_, err = w.store.db.Exec(ctx, `INSERT INTO local_events (`+eventColumns+`) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		pos, evt.ID, evt.RoomID, evt.Sender, evt.Type.Type, evt.StateKey, content, dbutil.StrPtr(evt.Redacts),
		unsigned, evt.Unsigned.BeeperHSOrder, evt.Unsigned.BeeperHSSuborder, evt.Timestamp, marker.Hidden)
	if err != nil {
		return 0, fmt.Errorf("failed to insert event %s: %w", evt.ID, err)
	}

	if evt.StateKey != nil {
		_, err = w.store.db.Exec(ctx, `
			INSERT INTO local_current_state (room_id, type, state_key, event_id) VALUES ($1, $2, $3, $4)
			ON CONFLICT (room_id, type, state_key) DO UPDATE SET event_id = excluded.event_id`,
			evt.RoomID, evt.Type.Type, *evt.StateKey, evt.ID)
		if err != nil {
			return 0, fmt.Errorf("failed to update current state: %w", err)
		}
	}
	return pos, nil
}

// GetEvent finds an event by ID. It returns nil (and no error) if there is none.
func (s *Store) GetEvent(ctx context.Context, eventID id.EventID) (*Event, error) {
	row := s.db.QueryRow(ctx, `SELECT `+eventColumns+` FROM local_events WHERE event_id = $1`, eventID)
	return scanEvent(row)
}

// GetState returns the current state event for (type, stateKey) in a room, or nil.
func (s *Store) GetState(ctx context.Context, roomID id.RoomID, evtType event.Type, stateKey string) (*Event, error) {
	row := s.db.QueryRow(ctx, `
		SELECT `+prefixed("e.", eventColumns)+`
		FROM local_current_state AS s JOIN local_events AS e ON e.event_id = s.event_id
		WHERE s.room_id = $1 AND s.type = $2 AND s.state_key = $3`,
		roomID, evtType.Type, stateKey)
	return scanEvent(row)
}

// GetStateOfType returns all current state events of one type in a room, e.g. every m.room.member.
func (s *Store) GetStateOfType(ctx context.Context, roomID id.RoomID, evtType event.Type) ([]*Event, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+prefixed("e.", eventColumns)+`
		FROM local_current_state AS s JOIN local_events AS e ON e.event_id = s.event_id
		WHERE s.room_id = $1 AND s.type = $2
		ORDER BY e.pos`,
		roomID, evtType.Type)
	if err != nil {
		return nil, err
	}
	return scanEvents(rows)
}

// GetStateByKey returns a state event of one type and key from every room, e.g. one user's memberships.
func (s *Store) GetStateByKey(ctx context.Context, evtType event.Type, stateKey string) ([]*Event, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+prefixed("e.", eventColumns)+`
		FROM local_current_state AS s JOIN local_events AS e ON e.event_id = s.event_id
		WHERE s.type = $1 AND s.state_key = $2
		ORDER BY e.pos`,
		evtType.Type, stateKey)
	if err != nil {
		return nil, err
	}
	return scanEvents(rows)
}

func scanEvents(rows dbutil.Rows) ([]*Event, error) {
	defer rows.Close()
	var events []*Event
	for rows.Next() {
		evt, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, evt)
	}
	return events, rows.Err()
}

// RoomOrderRange returns the lowest and highest timeline order in a room (0, 0 if empty).
func (s *Store) RoomOrderRange(ctx context.Context, roomID id.RoomID) (lowest, highest int64, err error) {
	err = s.db.QueryRow(ctx,
		`SELECT COALESCE(MIN(stream_order), 0), COALESCE(MAX(stream_order), 0) FROM local_events WHERE room_id = $1`,
		roomID).Scan(&lowest, &highest)
	return
}

func scanEvent(row dbutil.Scannable) (*Event, error) {
	var (
		evt               event.Event
		pos               int64
		evtType           string
		stateKey, redacts sql.NullString
		content, unsigned []byte
		order, suborder   int64
		hidden            bool
	)
	err := row.Scan(&pos, &evt.ID, &evt.RoomID, &evt.Sender, &evtType, &stateKey, &content, &redacts,
		&unsigned, &order, &suborder, &evt.Timestamp, &hidden)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to read event: %w", err)
	}

	evt.Type = event.Type{Type: evtType, Class: event.MessageEventType}
	if stateKey.Valid {
		evt.StateKey = &stateKey.String
		evt.Type.Class = event.StateEventType
	}
	evt.Redacts = id.EventID(redacts.String)
	if err = json.Unmarshal(content, &evt.Content); err != nil {
		return nil, fmt.Errorf("failed to decode content of %s: %w", evt.ID, err)
	}
	if len(unsigned) > 0 {
		if err = json.Unmarshal(unsigned, &evt.Unsigned); err != nil {
			return nil, fmt.Errorf("failed to decode unsigned of %s: %w", evt.ID, err)
		}
	}
	evt.Unsigned.BeeperHSOrder = order
	evt.Unsigned.BeeperHSSuborder = int16(suborder)
	return &Event{Pos: pos, Hidden: hidden, Event: &evt}, nil
}

// prefixed turns "a, b" into "e.a, e.b" for queries that join two tables.
func prefixed(prefix, columns string) string {
	names := strings.Split(columns, ", ")
	for i := range names {
		names[i] = prefix + names[i]
	}
	return strings.Join(names, ", ")
}

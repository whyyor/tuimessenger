package store

import (
	"context"
	"strconv"

	"maunium.net/go/mautrix/id"
)

// Order is a spot in one room's timeline; older history has lower orders.
type Order struct {
	Order    int64
	Suborder int64
}

// EventsSince returns events saved after the given position, oldest first, at most limit of them.
func (s *Store) EventsSince(ctx context.Context, since int64, limit int) ([]*Event, error) {
	rows, err := s.db.Query(ctx, `SELECT `+eventColumns+` FROM local_events WHERE pos > $1 ORDER BY pos LIMIT $2`, since, limit)
	if err != nil {
		return nil, err
	}
	return scanEvents(rows)
}

// RoomHistory returns a room's visible events before the given spot (or the newest ones if before is nil),
// newest first, at most limit of them.
func (s *Store) RoomHistory(ctx context.Context, roomID id.RoomID, before *Order, limit int) ([]*Event, error) {
	query := `SELECT ` + eventColumns + ` FROM local_events WHERE room_id = $1 AND hidden = 0`
	args := []any{roomID}
	if before != nil {
		query += ` AND (stream_order < $2 OR (stream_order = $2 AND stream_suborder < $3))`
		args = append(args, before.Order, before.Suborder)
	}
	query += ` ORDER BY stream_order DESC, stream_suborder DESC LIMIT ` + strconv.Itoa(max(limit, 1))
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return scanEvents(rows)
}

// RoomState returns every current state event in a room.
func (s *Store) RoomState(ctx context.Context, roomID id.RoomID) ([]*Event, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+prefixed("e.", eventColumns)+`
		FROM local_current_state AS s JOIN local_events AS e ON e.event_id = s.event_id
		WHERE s.room_id = $1
		ORDER BY e.pos`, roomID)
	if err != nil {
		return nil, err
	}
	return scanEvents(rows)
}

// IsCurrentState reports whether an event is still the room's current value for its type and key.
func (s *Store) IsCurrentState(ctx context.Context, evt *Event) (bool, error) {
	if evt.StateKey == nil {
		return false, nil
	}
	current, err := s.GetState(ctx, evt.RoomID, evt.Type, *evt.StateKey)
	if err != nil || current == nil {
		return false, err
	}
	return current.ID == evt.ID, nil
}

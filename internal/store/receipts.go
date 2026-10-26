package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// Receipt says a user has read a room up to an event.
type Receipt struct {
	RoomID    id.RoomID
	UserID    id.UserID
	Type      event.ReceiptType
	EventID   id.EventID
	Timestamp int64
	Pos       int64
}

// SetReceipt saves a receipt, replacing the user's previous one of the same type in that room.
func (w *Writer) SetReceipt(ctx context.Context, r *Receipt) error {
	if r.RoomID == "" || r.UserID == "" || r.EventID == "" {
		return errors.New("receipt needs a room ID, user ID and event ID")
	}

	if r.Timestamp == 0 {
		r.Timestamp = time.Now().UnixMilli()
	}

	r.Pos = w.nextPos()

	_, err := w.store.db.Exec(ctx, `
		INSERT INTO local_receipts (room_id, user_id, type, event_id, timestamp, pos)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (room_id, user_id, type) DO UPDATE
		SET event_id = excluded.event_id, timestamp = excluded.timestamp, pos = excluded.pos
	`, r.RoomID, r.UserID, r.Type, r.EventID, r.Timestamp, r.Pos)
	if err != nil {
		return fmt.Errorf("failed to save receipt: %w", err)
	}

	return nil
}

// ReceiptSince returns all receipts that changed after the given position, oldest first.
func (s *Store) ReceiptSince(ctx context.Context, since int64) ([]*Receipt, error) {
	rows, err := s.db.Query(ctx, `
		SELECT room_id, user_id, type, event_id, timestamp, pos
		FROM local_receipts WHERE pos > $1 ORDER BY pos
	`, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read receipts: %w", err)
	}

	defer rows.Close()

	var receipts []*Receipt
	for rows.Next() {
		var r Receipt
		err := rows.Scan(&r.RoomID, &r.UserID, &r.Type, &r.EventID, &r.Timestamp, &r.Pos)
		if err != nil {
			return nil, err
		}
		receipts = append(receipts, &r)
	}

	return receipts, rows.Err()
}

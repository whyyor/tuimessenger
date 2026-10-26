package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// AccountData is one of your settings, either global (RoomID "") or for one room, e.g. a mute or a tag.
type AccountData struct {
	RoomID  id.RoomID
	Type    string
	Content json.RawMessage
	Pos     int64
}

// SetAccountData saves a setting, replacing the previous value of the same type.
func (w *Writer) SetAccountData(ctx context.Context, roomID id.RoomID, evtType event.Type, content any) error {
	raw, err := json.Marshal(content)
	if err != nil {
		return fmt.Errorf("failed to encode account data: %w", err)
	}
	_, err = w.store.db.Exec(ctx, `
		INSERT INTO local_account_data (room_id, type, content, pos) VALUES ($1, $2, $3, $4)
		ON CONFLICT (room_id, type) DO UPDATE SET content = excluded.content, pos = excluded.pos
	`, roomID, evtType.Type, raw, w.nextPos())
	if err != nil {
		return fmt.Errorf("failed to save account data: %w", err)
	}
	return nil
}

// GetAccountData returns a setting's JSON, or nil if it was never set.
func (s *Store) GetAccountData(ctx context.Context, roomID id.RoomID, evtType event.Type) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.db.QueryRow(ctx, `SELECT content FROM local_account_data WHERE room_id = $1 AND type = $2`, roomID, evtType.Type).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to read account data: %w", err)
	}
	return raw, nil
}

// AccountDataSince returns all settings that changed after the given position, oldest first.
func (s *Store) AccountDataSince(ctx context.Context, since int64) ([]*AccountData, error) {
	rows, err := s.db.Query(ctx, `SELECT room_id, type, content, pos FROM local_account_data WHERE pos > $1 ORDER BY pos`, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read account data: %w", err)
	}
	defer rows.Close()

	var list []*AccountData
	for rows.Next() {
		var ad AccountData
		if err = rows.Scan(&ad.RoomID, &ad.Type, &ad.Content, &ad.Pos); err != nil {
			return nil, err
		}
		list = append(list, &ad)
	}
	return list, rows.Err()
}

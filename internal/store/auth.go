package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"maunium.net/go/mautrix/id"
)

// GetSetting returns a stored setting, or "" if it was never set.
func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRow(ctx, `SELECT value FROM settings WHERE key = $1`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO settings (key, value) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// Only a hash of each access token is stored, so a copy of the database can't be used to log in.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Store) AddAccessToken(ctx context.Context, token string, deviceID id.DeviceID) error {
	_, err := s.db.Exec(ctx, `INSERT INTO access_tokens (token_hash, device_id, created_at) VALUES ($1, $2, $3)`,
		hashToken(token), deviceID, time.Now().UnixMilli())
	if err != nil {
		return fmt.Errorf("failed to save access token: %w", err)
	}
	return nil
}

// CheckAccessToken returns the device a token belongs to, or "" if the token is unknown.
func (s *Store) CheckAccessToken(ctx context.Context, token string) (id.DeviceID, error) {
	var deviceID id.DeviceID
	err := s.db.QueryRow(ctx, `SELECT device_id FROM access_tokens WHERE token_hash = $1`, hashToken(token)).Scan(&deviceID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return deviceID, err
}

func (s *Store) DeleteAccessToken(ctx context.Context, token string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM access_tokens WHERE token_hash = $1`, hashToken(token))
	return err
}

// SaveTransaction remembers which event a client transaction created, so a retried send isn't sent twice.
func (w *Writer) SaveTransaction(ctx context.Context, txnID string, eventID id.EventID) error {
	_, err := w.store.db.Exec(ctx, `INSERT INTO sent_transactions (txn_id, event_id) VALUES ($1, $2)`, txnID, eventID)
	return err
}

// GetTransaction returns the event a transaction created, or "" if it's new.
func (s *Store) GetTransaction(ctx context.Context, txnID string) (id.EventID, error) {
	var eventID id.EventID
	err := s.db.QueryRow(ctx, `SELECT event_id FROM sent_transactions WHERE txn_id = $1`, txnID).Scan(&eventID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return eventID, err
}

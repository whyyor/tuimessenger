package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"maunium.net/go/mautrix/id"
)

// Profile is a user's display name and avatar, copied into their member events.
type Profile struct {
	Displayname string
	AvatarURL   id.ContentURIString
}

// SetProfile saves a user's profile and reports whether it changed.
func (w *Writer) SetProfile(ctx context.Context, userID id.UserID, profile Profile) (bool, error) {
	old, err := w.store.GetProfile(ctx, userID)
	if err != nil {
		return false, err
	}
	if old == profile {
		return false, nil
	}
	_, err = w.store.db.Exec(ctx, `
		INSERT INTO local_profiles (user_id, displayname, avatar_url) VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET displayname = excluded.displayname, avatar_url = excluded.avatar_url
	`, userID, profile.Displayname, profile.AvatarURL)
	if err != nil {
		return false, fmt.Errorf("failed to save profile: %w", err)
	}
	return true, nil
}

// GetProfile returns a user's profile, or an empty one if we don't know them.
func (s *Store) GetProfile(ctx context.Context, userID id.UserID) (Profile, error) {
	var profile Profile
	err := s.db.QueryRow(ctx, `SELECT displayname, avatar_url FROM local_profiles WHERE user_id = $1`, userID).
		Scan(&profile.Displayname, &profile.AvatarURL)
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, nil
	} else if err != nil {
		return Profile{}, fmt.Errorf("failed to read profile: %w", err)
	}
	return profile, nil
}

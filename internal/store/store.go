// Package store keeps the rooms and events our Matrix server shows to clients.
package store

import (
	"context"
	"embed"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog"
	"go.mau.fi/util/dbutil"
	_ "go.mau.fi/util/dbutil/litestream" // registers the SQLite drivers (needs cgo)
)

//go:embed upgrades/*.sql
var upgradeFiles embed.FS

var upgradeTable = dbutil.BuildUpgradeTable().WithFSPath(upgradeFiles, "upgrades").Finish()

// Store is our own SQLite database, separate from each bridge's database.
type Store struct {
	db *dbutil.Database

	writeLock sync.Mutex   // only one write transaction at a time
	lastPos   atomic.Int64 // newest committed position, read by /sync without locking

	notifyLock sync.Mutex
	notifyCh   chan struct{} // closed and replaced after every write
}

// Open opens (or creates) the store file and brings its tables up to date.
func Open(ctx context.Context, path string, log zerolog.Logger) (*Store, error) {
	db, err := dbutil.NewFromConfig("tuimessenger-store", dbutil.Config{
		PoolConfig: dbutil.PoolConfig{
			Type:         "sqlite3-fk-wal", // synchronous=NORMAL: fewer flash writes than the bridges' FULL
			URI:          "file:" + path + "?_txlock=immediate",
			MaxOpenConns: 5,
			MaxIdleConns: 1,
		},
	}, dbutil.ZeroLogger(log.With().Str("component", "store").Logger()))
	if err != nil {
		return nil, fmt.Errorf("failed to open store: %w", err)
	}
	db.UpgradeTable = upgradeTable
	if err = db.Upgrade(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to upgrade store: %w", err)
	}

	s := &Store{db: db, notifyCh: make(chan struct{})}
	var last int64
	err = db.QueryRow(ctx, `
		SELECT MAX(
			(SELECT COALESCE(MAX(pos), 0) FROM local_events),
			(SELECT COALESCE(MAX(pos), 0) FROM local_receipts),
			(SELECT COALESCE(MAX(pos), 0) FROM local_account_data)
		)`).Scan(&last)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to read last position: %w", err)
	}
	s.lastPos.Store(last)
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// LastPos is the newest position that is safely written to disk.
func (s *Store) LastPos() int64 {
	return s.lastPos.Load()
}

// Changes returns a channel that is closed after the next successful write.
// Take the channel first, then check LastPos, so no write can slip in between.
func (s *Store) Changes() <-chan struct{} {
	s.notifyLock.Lock()
	defer s.notifyLock.Unlock()
	return s.notifyCh
}

func (s *Store) wakeUp() {
	s.notifyLock.Lock()
	defer s.notifyLock.Unlock()
	close(s.notifyCh)
	s.notifyCh = make(chan struct{})
}

// Writer hands out positions inside one Write call.
type Writer struct {
	store *Store
	pos   int64
}

func (w *Writer) nextPos() int64 {
	w.pos++
	return w.pos
}

// Write runs fn in one transaction. Everything fn writes is saved together or
// not at all, and readers only see the new positions after the commit.
func (s *Store) Write(ctx context.Context, fn func(ctx context.Context, w *Writer) error) error {
	s.writeLock.Lock()
	defer s.writeLock.Unlock()

	w := &Writer{store: s, pos: s.lastPos.Load()}
	err := s.db.DoTxn(ctx, nil, func(ctx context.Context) error {
		return fn(ctx, w)
	})
	if err != nil {
		return err
	}
	if w.pos != s.lastPos.Load() {
		s.lastPos.Store(w.pos)
		s.wakeUp()
	}
	return nil
}

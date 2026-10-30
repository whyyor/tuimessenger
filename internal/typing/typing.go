// Package typing remembers who is typing where. It only lasts seconds, so it's kept in memory, never on disk.
package typing

import (
	"slices"
	"sync"
	"time"

	"maunium.net/go/mautrix/id"
)

type Tracker struct {
	lock    sync.Mutex
	rooms   map[id.RoomID]map[id.UserID]*entry
	changed map[id.RoomID]int64 // the version at which each room's list last changed
	version int64
	notify  chan struct{}
}

func New() *Tracker {
	return &Tracker{
		rooms:   make(map[id.RoomID]map[id.UserID]*entry),
		changed: make(map[id.RoomID]int64),
		notify:  make(chan struct{}),
	}
}

type entry struct {
	timer *time.Timer
}

// Set marks userID as typing in roomID for the given time; zero means they stopped.
func (t *Tracker) Set(roomID id.RoomID, userID id.UserID, timeout time.Duration) {
	t.lock.Lock()
	defer t.lock.Unlock()
	users := t.rooms[roomID]
	old, wasTyping := users[userID]
	if wasTyping {
		old.timer.Stop()
	}
	if timeout <= 0 {
		if !wasTyping {
			return
		}
		delete(users, userID)
		t.bump(roomID)
		return
	}
	if users == nil {
		users = make(map[id.UserID]*entry)
		t.rooms[roomID] = users
	}
	e := &entry{}
	e.timer = time.AfterFunc(timeout, func() { t.expire(roomID, userID, e) })
	users[userID] = e
	if !wasTyping {
		t.bump(roomID)
	}
}

// expire removes a typing entry when its time runs out, unless a newer Set already replaced it.
func (t *Tracker) expire(roomID id.RoomID, userID id.UserID, e *entry) {
	t.lock.Lock()
	defer t.lock.Unlock()
	if t.rooms[roomID][userID] != e {
		return
	}
	delete(t.rooms[roomID], userID)
	t.bump(roomID)
}

func (t *Tracker) bump(roomID id.RoomID) {
	t.version++
	t.changed[roomID] = t.version
	close(t.notify)
	t.notify = make(chan struct{})
}

func (t *Tracker) Version() int64 {
	t.lock.Lock()
	defer t.lock.Unlock()
	return t.version
}

// Changes returns a channel that is closed at the next change.
func (t *Tracker) Changes() <-chan struct{} {
	t.lock.Lock()
	defer t.lock.Unlock()
	return t.notify
}

// ChangedSince returns every room whose list changed after version, with who is typing there now.
func (t *Tracker) ChangedSince(version int64) map[id.RoomID][]id.UserID {
	t.lock.Lock()
	defer t.lock.Unlock()
	out := make(map[id.RoomID][]id.UserID)
	for roomID, at := range t.changed {
		if at > version {
			out[roomID] = t.unlockedCurrent(roomID)
		}
	}
	return out
}

// Current returns who is typing in a room right now.
func (t *Tracker) Current(roomID id.RoomID) []id.UserID {
	t.lock.Lock()
	defer t.lock.Unlock()
	return t.unlockedCurrent(roomID)
}

func (t *Tracker) unlockedCurrent(roomID id.RoomID) []id.UserID {
	users := make([]id.UserID, 0, len(t.rooms[roomID]))
	for userID := range t.rooms[roomID] {
		users = append(users, userID)
	}
	slices.Sort(users)
	return users
}

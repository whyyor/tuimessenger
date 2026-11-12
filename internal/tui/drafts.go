package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/tui/debug"
)

// draftIcon marks chats with a draft in the sidebar and Ctrl+k (nf-md-pencil).
const draftIcon = "\U000f03eb"

// draftSaveDelay is how long after the last keystroke a draft is written to disk, so a crash loses at most that.
var draftSaveDelay = 2 * time.Second

// draft is what you left in a chat's message box: the text, and the message you were replying to.
type draft struct {
	Text    string     `json:"text"`
	ReplyTo id.EventID `json:"reply_to,omitempty"`
}

func (d draft) empty() bool { return d.Text == "" && d.ReplyTo == "" }

// draftStore keeps each chat's draft, saved in drafts.json next to terminal.yaml so drafts survive restarts.
// Attached files are kept too, but only while tuimessenger runs.
type draftStore struct {
	mu       sync.Mutex
	saveMu   sync.Mutex
	path     string
	loaded   bool
	byRoom   map[id.RoomID]draft
	attached map[id.RoomID][]pendingFile
	timer    *time.Timer
}

var drafts = &draftStore{}

// use sets the save file the first time; an empty dir keeps drafts in memory only.
func (d *draftStore) use(dir string) *draftStore {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.path == "" && dir != "" {
		d.path = filepath.Join(dir, "drafts.json")
	}
	return d
}

func (d *draftStore) loadLocked() {
	if d.byRoom == nil {
		d.byRoom = map[id.RoomID]draft{}
		d.attached = map[id.RoomID][]pendingFile{}
	}
	if d.loaded || d.path == "" {
		return
	}
	d.loaded = true
	if data, err := os.ReadFile(d.path); err == nil {
		_ = json.Unmarshal(data, &d.byRoom)
	}
}

// get is a chat's draft and attached files.
func (d *draftStore) get(roomID id.RoomID) (draft, []pendingFile) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.loadLocked()
	return d.byRoom[roomID], d.attached[roomID]
}

// has reports whether a chat has a draft (text, a reply or files), for the chat list's ✎.
func (d *draftStore) has(roomID id.RoomID) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.loadLocked()
	_, text := d.byRoom[roomID]
	return text || len(d.attached[roomID]) > 0
}

// set replaces a chat's draft (an empty one removes it) and saves it shortly after.
func (d *draftStore) set(roomID id.RoomID, dr draft) {
	d.mu.Lock()
	d.loadLocked()
	if old := d.byRoom[roomID]; old == dr {
		d.mu.Unlock()
		return
	}
	if dr.empty() {
		delete(d.byRoom, roomID)
	} else {
		d.byRoom[roomID] = dr
	}
	if d.path != "" {
		if d.timer != nil {
			d.timer.Stop()
		}
		d.timer = time.AfterFunc(draftSaveDelay, d.save)
	}
	d.mu.Unlock()
}

// setFiles keeps a chat's attached files for when you come back (nil removes them).
func (d *draftStore) setFiles(roomID id.RoomID, files []pendingFile) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.loadLocked()
	if len(files) == 0 {
		delete(d.attached, roomID)
	} else {
		d.attached[roomID] = files
	}
}

// save writes every draft now.
func (d *draftStore) save() {
	defer debug.Recover()
	d.saveMu.Lock()
	defer d.saveMu.Unlock()
	d.mu.Lock()
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
	path := d.path
	raw, _ := json.Marshal(d.byRoom)
	d.mu.Unlock()
	if path == "" {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		debug.Print("Failed to save drafts:", err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		debug.Print("Failed to save drafts:", err)
	}
}

// currentDraft is what's in this chat's message box now.
func (view *RoomView) currentDraft() draft {
	return view.draftOf(view.input.GetText())
}

// draftOf is the draft for this text in the message box; while editing a message, the draft parked for it.
func (view *RoomView) draftOf(text string) draft {
	if view.editing != nil {
		text = view.editMoveText
	}
	d := draft{Text: text}
	if view.replying != nil {
		d.ReplyTo = view.replying.ID
	}
	return d
}

// keepDraft saves this chat's message box (and attached files) as you leave it.
func (view *RoomView) keepDraft() {
	drafts.set(view.Room.ID, view.currentDraft())
	view.attached.Lock()
	files := view.attached.items
	view.attached.items = nil
	view.attached.Unlock()
	drafts.setFiles(view.Room.ID, files)
}

// restoreDraft puts back what you left in this chat: text (cursor at the end), the reply and attached files. It
// doesn't announce typing.
func (view *RoomView) restoreDraft() {
	d, files := drafts.get(view.Room.ID)
	if d.Text != "" {
		view.restoring = true
		view.input.SetTextAndMoveCursor(d.Text)
		view.restoring = false
	}
	if d.ReplyTo != "" {
		if evt := view.Room.GetEventByID(d.ReplyTo); evt != nil {
			view.replying, view.replyingName = evt, view.Room.GetDisplayname(evt.Sender)
		}
	}
	if len(files) > 0 {
		view.attached.Lock()
		view.attached.items = files
		view.attached.Unlock()
		drafts.setFiles(view.Room.ID, nil)
	}
}

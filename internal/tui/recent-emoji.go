package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/mattn/go-runewidth"

	"go.mau.fi/gomuks/tui/debug"
)

const recentEmojiMax = 24

// recentEmojis is the Ctrl+e quick row: last-used emoji, newest first,
// saved next to terminal.yaml so it survives restarts.
type recentEmojis struct {
	mu     sync.Mutex
	saveMu sync.Mutex
	path   string
	list   []string
	loaded bool
}

var recentEmoji = &recentEmojis{}

// use sets the save file the first time; an empty dir keeps it in memory only.
func (r *recentEmojis) use(dir string) *recentEmojis {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.path == "" && dir != "" {
		r.path = filepath.Join(dir, "recent-emoji.json")
	}
	return r
}

func (r *recentEmojis) loadLocked() {
	if r.loaded || r.path == "" {
		return
	}
	r.loaded = true
	if data, err := os.ReadFile(r.path); err == nil {
		_ = json.Unmarshal(data, &r.list)
	}
}

// Top returns n emoji for the quick row: recent first, then the defaults.
func (r *recentEmojis) Top(n int) []string {
	r.mu.Lock()
	r.loadLocked()
	candidates := append(slices.Clone(r.list), quickReactions...)
	r.mu.Unlock()
	out := make([]string, 0, n)
	for _, e := range candidates {
		if len(out) == n {
			break
		}
		if !slices.ContainsFunc(out, func(o string) bool { return sameReaction(o, e) }) {
			out = append(out, e)
		}
	}
	return out
}

// Add moves e to the front and saves in the background.
func (r *recentEmojis) Add(e string) {
	r.mu.Lock()
	r.loadLocked()
	r.list = slices.DeleteFunc(r.list, func(o string) bool { return sameReaction(o, e) })
	r.list = append([]string{e}, r.list...)
	if len(r.list) > recentEmojiMax {
		r.list = r.list[:recentEmojiMax]
	}
	path := r.path
	r.mu.Unlock()
	if path == "" {
		return
	}
	go func() {
		defer debug.Recover()
		// Each save writes the list as it is now, so the last one wins.
		r.saveMu.Lock()
		defer r.saveMu.Unlock()
		r.mu.Lock()
		raw, _ := json.Marshal(r.list)
		r.mu.Unlock()
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, raw, 0o600); err == nil {
			err = os.Rename(tmp, path)
			if err != nil {
				debug.Print("Failed to save recent emoji:", err)
			}
		}
	}()
}

// textBeforeWidth cuts s at a cursor offset, measured like mauview's input.
func textBeforeWidth(s string, w int) string {
	width := 0
	for i, r := range s {
		rw := runewidth.RuneWidth(r)
		if r == '\n' {
			rw = 1
		}
		if width+rw > w {
			return s[:i]
		}
		width += rw
	}
	return s
}

// insertAtCursor types s into the composer where the cursor is.
func (view *RoomView) insertAtCursor(s string) {
	text := view.input.GetText()
	left := textBeforeWidth(text, view.input.GetCursorOffset())
	view.input.SetTextAndMoveCursor(left + s + text[len(left):])
}

package tui

import (
	"strings"
	"testing"
)

func TestToolHint(t *testing.T) {
	only := func(names ...string) func(string) bool {
		return func(n string) bool {
			for _, name := range names {
				if n == name {
					return true
				}
			}
			return false
		}
	}
	for _, c := range []struct {
		goos string
		have func(string) bool
		want string
	}{
		{"darwin", only(), "Install ffmpeg for GIF and video previews: brew install ffmpeg"},
		{"linux", only("apt-get", "brew"), "sudo apt install ffmpeg"},
		{"linux", only("dnf"), "sudo dnf install ffmpeg"},
		{"linux", only("pacman"), "sudo pacman -S ffmpeg"},
		{"linux", only("brew"), "brew install ffmpeg"},
		{"linux", only(), "install ffmpeg with your package manager"},
	} {
		if got := toolHint("ffmpeg", c.goos, c.have); !strings.Contains(got, c.want) {
			t.Errorf("%s: %q, want %q", c.goos, got, c.want)
		}
	}
	if got := toolHint("mpv", "darwin", only()); got != "Install mpv for playing GIFs and videos in the terminal: brew install mpv" {
		t.Errorf("mpv: %q", got)
	}
}

// Each tool's hint shows once per session, not for every GIF in a chat.
func TestHintMissingToolOnce(t *testing.T) {
	ts, _, _ := fakeToasts(t)
	shownToolHints.Clear()
	t.Cleanup(shownToolHints.Clear)
	for range 5 {
		hintMissingTool("ffmpeg")
	}
	if !strings.HasPrefix(current(ts), "Install ffmpeg for GIF and video previews") {
		t.Errorf("the hint shows: %q", current(ts))
	}
	ts.mu.Lock()
	queued := len(ts.queue)
	ts.mu.Unlock()
	if queued != 0 {
		t.Errorf("only once per session: %d more queued", queued)
	}
	hintMissingTool("mpv")
	ts.mu.Lock()
	queued = len(ts.queue)
	ts.mu.Unlock()
	if queued != 1 {
		t.Errorf("mpv gets its own hint: %d queued", queued)
	}
}

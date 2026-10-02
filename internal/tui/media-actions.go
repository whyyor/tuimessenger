package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"maunium.net/go/mautrix/event"

	"go.mau.fi/gomuks/tui/debug"
	"go.mau.fi/gomuks/tui/messages"
)

var (
	playbackLock sync.Mutex
	playbackCmd  *exec.Cmd
)

// StopPlayback kills any running media playback. Called when leaving the
// room the playback was started from.
func StopPlayback() {
	playbackLock.Lock()
	defer playbackLock.Unlock()
	if playbackCmd != nil && playbackCmd.Process != nil {
		_ = playbackCmd.Process.Kill()
	}
	playbackCmd = nil
}

// mediaPlayerCommand finds a player able to handle ogg/opus voice notes.
// afplay is last because it cannot play ogg, which is what WhatsApp sends.
func mediaPlayerCommand(path string) *exec.Cmd {
	if mpv, err := exec.LookPath("mpv"); err == nil {
		return exec.Command(mpv, "--no-video", "--really-quiet", path)
	}
	if ffplay, err := exec.LookPath("ffplay"); err == nil {
		return exec.Command(ffplay, "-nodisp", "-autoexit", "-loglevel", "quiet", path)
	}
	if afplay, err := exec.LookPath("afplay"); err == nil {
		return exec.Command(afplay, path)
	}
	return nil
}

func (view *RoomView) fetchMediaToFile(msg *messages.FileMessage, dir string) (string, error) {
	data, err := view.parent.matrix.Download(msg.URL, msg.IsEncrypted)
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(msg.Body)
	if name == "" {
		name = msg.URL.FileID
	}
	if filepath.Ext(name) == "" {
		name += extForMime(msg.MimeType, msg.Type)
	}
	path := filepath.Join(dir, filepath.Base(name))
	return path, os.WriteFile(path, data, 0600)
}

// extForMime picks a file extension from the mimetype, falling back to a
// guess by message type; bridges often send bare bodies.
func extForMime(mimeType string, msgType event.MessageType) string {
	switch mimeType {
	case "video/mp4":
		return ".mp4"
	case "video/quicktime":
		return ".mov"
	case "audio/ogg":
		return ".ogg"
	case "audio/mpeg":
		return ".mp3"
	case "audio/mp4", "audio/m4a":
		return ".m4a"
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	}
	if msgType == event.MsgVideo {
		return ".mp4"
	}
	return ".ogg"
}

// extractURL pulls the first http(s) URL out of a message body. Bridged link
// posts, like Instagram reels, are an image thumbnail with the URL in the
// body, often bracketed.
func extractURL(body string) string {
	idx := strings.Index(body, "http")
	if idx == -1 {
		return ""
	}
	url := body[idx:]
	if end := strings.IndexAny(url, " ]\n"); end != -1 {
		url = url[:end]
	}
	return url
}

// PlayMedia plays an audio or video message: audio in the background, video
// in the terminal via mpv's kitty output. Bridged link posts, which arrive
// as a thumbnail image with the URL in the body, are handed to IINA, which
// streams them through yt-dlp.
func (view *RoomView) PlayMedia(msg *messages.FileMessage) {
	defer debug.Recover()
	isVideo := msg.Type == event.MsgVideo || strings.HasPrefix(msg.MimeType, "video/")
	isAudio := msg.Type == event.MsgAudio || strings.HasPrefix(msg.MimeType, "audio/")
	if !isVideo && !isAudio {
		if url := extractURL(msg.Body); url != "" {
			view.AddServiceMessage("Opening %s", url)
			if err := exec.Command("open", "-a", "IINA", url).Run(); err != nil {
				_ = exec.Command("open", url).Start()
			}
		} else {
			view.AddServiceMessage("Nothing playable in that message")
		}
		view.parent.parent.Render()
		return
	}
	path, err := view.fetchMediaToFile(msg, os.TempDir())
	if err != nil {
		view.AddServiceMessage("Failed to fetch media: %v", err)
		view.parent.parent.Render()
		return
	}
	if isVideo {
		view.playVideoInTerminal(path)
		return
	}
	cmd := mediaPlayerCommand(path)
	if cmd == nil {
		view.AddServiceMessage("No audio player found (tried mpv, ffplay, afplay)")
		view.parent.parent.Render()
		return
	}
	playbackLock.Lock()
	if playbackCmd != nil && playbackCmd.Process != nil {
		_ = playbackCmd.Process.Kill()
	}
	playbackCmd = cmd
	playbackLock.Unlock()
	if err = cmd.Start(); err != nil {
		view.AddServiceMessage("Failed to start playback: %v", err)
		view.parent.parent.Render()
		return
	}
	view.AddServiceMessage("Playing %s…", filepath.Base(path))
	view.parent.parent.Render()
	go func() {
		_ = cmd.Wait()
		_ = os.Remove(path)
	}()
}

// playVideoInTerminal suspends the TUI and hands the terminal to mpv with
// its kitty-graphics output, so the video renders inside the terminal with
// mpv's own controls: space pauses, arrows seek, q returns to the chat.
// tcell and mpv cannot share the tty, which is why the TUI must step aside.
func (view *RoomView) playVideoInTerminal(path string) {
	mpv, err := exec.LookPath("mpv")
	if err != nil {
		// No in-terminal player; hand the file to the system instead.
		_ = exec.Command("open", path).Start()
		return
	}
	StopPlayback()
	view.parent.parent.app.Suspend(func() {
		print("\033[2J\033[0;0H")
		cmd := exec.Command(mpv, "--vo=kitty", "--really-quiet", path)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
		_ = os.Remove(path)
	})
	view.parent.parent.Render()
}

// SaveOrOpenMedia downloads a media message to ~/Downloads, optionally
// opening it with the system handler.
func (view *RoomView) SaveOrOpenMedia(msg *messages.FileMessage, openIt bool) {
	defer debug.Recover()
	dir := os.TempDir()
	if !openIt {
		home, err := os.UserHomeDir()
		if err == nil {
			dir = filepath.Join(home, "Downloads")
		}
	}
	path, err := view.fetchMediaToFile(msg, dir)
	if err != nil {
		view.AddServiceMessage("Failed to fetch media: %v", err)
	} else if openIt {
		_ = exec.Command("open", path).Start()
	} else {
		view.AddServiceMessage("Saved to %s", path)
	}
	view.parent.parent.Render()
}

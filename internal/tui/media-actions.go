package tui

import (
	"fmt"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

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
	name := mediaFileName(msg.FileName, msg.Body, msg.URL.FileID, msg.MimeType, msg.Type)
	path := uniquePath(dir, name)
	return path, os.WriteFile(path, data, 0600)
}

// maxFileNameBytes keeps names well under the 255-byte filesystem limit.
const maxFileNameBytes = 120

// mediaFileName picks an on-disk name the way the spec defines it: the
// filename field when present (the body is then a caption, which can be a
// whole paragraph), otherwise the body, otherwise the media ID.
func mediaFileName(fileName, body, fileID, mimeType string, msgType event.MessageType) string {
	name := sanitizeFileName(fileName)
	if name == "" {
		name = sanitizeFileName(body)
	}
	if name == "" {
		name = sanitizeFileName(fileID)
	}
	if name == "" {
		name = "media"
	}
	if fileExt(name) == "" {
		name += extForMime(mimeType, msgType)
	}
	return name
}

// fileExt is filepath.Ext restricted to plausible extensions, so a caption's
// sentence ("Check this out. Amazing") isn't mistaken for one.
func fileExt(name string) string {
	ext := filepath.Ext(name)
	if len(ext) < 2 || len(ext) > 10 || strings.ContainsAny(ext, " ") {
		return ""
	}
	return ext
}

// sanitizeFileName makes a name safe for disk: no path separators or control
// characters, collapsed whitespace, no leading dots, and a bounded length
// that keeps the extension.
func sanitizeFileName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r == '/' || r == '\\' || r == ':':
			b.WriteRune('-')
		case unicode.IsControl(r) || unicode.IsSpace(r):
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	name = strings.TrimLeft(strings.Join(strings.Fields(b.String()), " "), ".")
	if len(name) <= maxFileNameBytes {
		return name
	}
	ext := fileExt(name)
	stem := name[:len(name)-len(ext)]
	limit := maxFileNameBytes - len(ext)
	for limit > 0 && !utf8.RuneStart(stem[limit]) {
		limit--
	}
	return strings.TrimSpace(stem[:limit]) + ext
}

// uniquePath returns dir/name, or "name (1).ext", "name (2).ext"… if taken,
// so a download never overwrites an existing file.
func uniquePath(dir, name string) string {
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 1; i < 1000; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	return path
}

// extForMime picks a file extension from the mimetype, falling back to a
// guess by message type; bridges often send bare bodies.
func extForMime(mimeType string, msgType event.MessageType) string {
	// Drop parameters: WhatsApp voice notes are "audio/ogg; codecs=opus".
	if base, _, err := mime.ParseMediaType(mimeType); err == nil {
		mimeType = base
	}
	switch mimeType {
	case "application/octet-stream":
		// Means "unknown"; fall through to the message-type guess.
		mimeType = ""
	case "video/mp4":
		return ".mp4"
	case "video/quicktime":
		return ".mov"
	case "video/x-matroska":
		return ".mkv"
	case "text/plain":
		return ".txt"
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
	// Anything else (PDFs, documents…) from the standard MIME table; the
	// explicit cases above avoid its odd first picks like .jfif for JPEG.
	if exts, _ := mime.ExtensionsByType(mimeType); len(exts) > 0 {
		return exts[0]
	}
	switch msgType {
	case event.MsgVideo:
		return ".mp4"
	case event.MsgAudio:
		return ".ogg"
	case event.MsgImage:
		return ".jpg"
	}
	// Unknown file type: no extension beats a wrong one.
	return ""
}

// PlayMedia plays an audio or video message: audio in the background, video
// in the terminal via mpv's kitty output. Bridged link posts (an Instagram
// reel arrives as a thumbnail image plus the reel URL) play from the URL.
func (view *RoomView) PlayMedia(msg *messages.FileMessage) {
	defer debug.Recover()
	isVideo := msg.Type == event.MsgVideo || strings.HasPrefix(msg.MimeType, "video/")
	isAudio := msg.Type == event.MsgAudio || strings.HasPrefix(msg.MimeType, "audio/")
	if !isVideo && !isAudio {
		if msg.LinkURL != "" {
			view.playLink(msg.LinkURL)
		} else {
			view.AddServiceMessage("Nothing playable in that message")
			view.parent.parent.Render()
		}
		return
	}
	path, err := view.fetchMediaToFile(msg, os.TempDir())
	if err != nil {
		view.AddServiceMessage("Failed to fetch media: %v", err)
		view.parent.parent.Render()
		return
	}
	if isVideo {
		view.playVideoInTerminal(path, true)
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

// playLink plays a post's video from its URL. mpv streams it in the terminal
// (it resolves the URL with yt-dlp itself). Without mpv, yt-dlp downloads it,
// with ffmpeg merging Instagram's separate video and audio streams, and
// QuickTime plays the file. IINA isn't used: its bundled youtube-dl is too
// old for Instagram.
func (view *RoomView) playLink(url string) {
	if _, err := exec.LookPath("mpv"); err == nil {
		view.playVideoInTerminal(url, false)
		return
	}
	ytdlp, err := exec.LookPath("yt-dlp")
	if err != nil {
		view.AddServiceMessage("Opening %s in the browser (install mpv or yt-dlp to play it here)", url)
		view.parent.parent.Render()
		_ = exec.Command("open", url).Start()
		return
	}
	view.AddServiceMessage("Fetching video…")
	view.parent.parent.Render()
	// Prefer H.264/AAC: Instagram's best stream is VP9, which QuickTime
	// can't play.
	cmd := exec.Command(ytdlp, "--quiet", "--no-warnings", "--no-playlist",
		"-S", "vcodec:h264,acodec:aac", "--merge-output-format", "mp4",
		"-o", filepath.Join(os.TempDir(), "gomuks-%(id)s.%(ext)s"),
		"--print", "after_move:filepath", url)
	out, err := cmd.Output()
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	path := strings.TrimSpace(lines[len(lines)-1])
	if err != nil || path == "" {
		reason := err
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			reason = fmt.Errorf("%s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		view.AddServiceMessage("Couldn't fetch the video (%v); opening it in the browser", reason)
		view.parent.parent.Render()
		_ = exec.Command("open", url).Start()
		return
	}
	view.AddServiceMessage("Playing in QuickTime (install mpv to play inside the terminal)")
	view.parent.parent.Render()
	_ = exec.Command("open", path).Start()
}

// playVideoInTerminal suspends the TUI and hands the terminal to mpv with
// its kitty-graphics output, so the video renders inside the terminal with
// mpv's own controls: space pauses, arrows seek, q returns to the chat.
// tcell and mpv cannot share the tty, which is why the TUI must step aside.
// target is a local file (deleted afterwards when isFile) or a URL.
func (view *RoomView) playVideoInTerminal(target string, isFile bool) {
	mpv, err := exec.LookPath("mpv")
	if err != nil {
		// No in-terminal player; hand the file to the system instead.
		_ = exec.Command("open", target).Start()
		return
	}
	StopPlayback()
	view.parent.parent.app.Suspend(func() {
		print("\033[2J\033[0;0H")
		cmd := exec.Command(mpv, "--vo=kitty", "--really-quiet", target)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
		if isFile {
			_ = os.Remove(target)
		}
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

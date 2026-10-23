package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"go.mau.fi/mauview"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/gomuks/tui/debug"
	"go.mau.fi/gomuks/tui/lib/emoji"
	"go.mau.fi/gomuks/tui/widget"
)

// pendingFile is a file waiting in the composer; temp ones (clipboard
// images) are deleted once sent or dropped.
type pendingFile struct {
	path string
	temp bool
	// sticker is a copied sticker's content; sent as m.sticker, no upload.
	sticker json.RawMessage
}

// sendFile uploads and sends one file; swapped out in tests.
var sendFile = (*RoomView).uploadAndSend

// sendText sends a plain message; swapped out in tests.
var sendText = func(view *RoomView, text string) { view.SendMessage(event.MsgText, text) }

func (view *RoomView) requestRender() {
	if view.parent != nil && view.parent.parent != nil {
		view.parent.parent.Render()
	}
}

type attachmentList struct {
	sync.Mutex
	items []pendingFile
}

func (view *RoomView) Attachments() []pendingFile {
	view.attached.Lock()
	defer view.attached.Unlock()
	return slices.Clone(view.attached.items)
}

// AttachFiles adds files as chips above the composer; Enter sends them.
func (view *RoomView) AttachFiles(paths ...string) {
	view.attached.Lock()
	for _, path := range paths {
		if !slices.ContainsFunc(view.attached.items, func(a pendingFile) bool { return a.path == path }) {
			view.attached.items = append(view.attached.items, pendingFile{path: path})
		}
	}
	view.attached.Unlock()
	view.requestRender()
}

func (view *RoomView) attachTemp(path string) {
	view.attached.Lock()
	view.attached.items = append(view.attached.items, pendingFile{path: path, temp: true})
	view.attached.Unlock()
	view.requestRender()
}

// takeAttachments empties the composer's chips and returns them.
func (view *RoomView) takeAttachments() []pendingFile {
	view.attached.Lock()
	defer view.attached.Unlock()
	items := view.attached.items
	view.attached.items = nil
	return items
}

func dropTemp(items []pendingFile) {
	for _, a := range items {
		if a.temp {
			_ = os.Remove(a.path)
		}
	}
}

// RemoveLastAttachment is Backspace on an empty composer.
func (view *RoomView) RemoveLastAttachment() bool {
	view.attached.Lock()
	n := len(view.attached.items)
	if n == 0 {
		view.attached.Unlock()
		return false
	}
	last := view.attached.items[n-1]
	view.attached.items = view.attached.items[:n-1]
	view.attached.Unlock()
	dropTemp([]pendingFile{last})
	return true
}

// ClearAttachments is Escape with chips showing.
func (view *RoomView) ClearAttachments() bool {
	items := view.takeAttachments()
	dropTemp(items)
	return len(items) > 0
}

// SendAttachments uploads every chip in order; the caption and the reply go
// on the first file, like an album caption.
func (view *RoomView) SendAttachments(caption string) {
	items := view.takeAttachments()
	if len(items) == 0 {
		return
	}
	var relatesTo *event.RelatesTo
	if view.replying != nil {
		relatesTo = (&event.RelatesTo{}).SetReplyTo(view.replying.ID)
		view.replying = nil
	}
	if caption != "" && !view.config.Preferences.DisableEmojis {
		caption = emoji.Replace(caption)
	}
	go func() {
		defer debug.Recover()
		defer dropTemp(items)
		// The caption and reply ride on the first file; stickers can't
		// carry a caption, so with only stickers it follows as text.
		captionOn := slices.IndexFunc(items, func(a pendingFile) bool { return a.sticker == nil })
		for i, a := range items {
			var rel *event.RelatesTo
			if i == 0 {
				rel = relatesTo
			}
			switch {
			case a.sticker != nil:
				sendSticker(view, a.sticker, rel)
			case i == captionOn:
				sendFile(view, a.path, caption, rel)
			default:
				sendFile(view, a.path, "", rel)
			}
		}
		if captionOn == -1 && caption != "" {
			sendText(view, caption)
		}
	}()
}

// AttachClipboard attaches files copied in Finder, else a copied image.
func (view *RoomView) AttachClipboard() {
	defer debug.Recover()
	if files := clipboardFiles(); len(files) > 0 {
		view.AttachFiles(files...)
		return
	}
	pngpaste, err := exec.LookPath("pngpaste")
	if err != nil {
		view.AddServiceMessage("pngpaste not found; cannot read images from the clipboard")
		view.requestRender()
		return
	}
	// Not the media temp dir: that one is wiped by every playback.
	dir := filepath.Join(os.TempDir(), "gomuks-attach")
	if err := os.MkdirAll(dir, 0700); err != nil {
		view.AddServiceMessage("Failed to prepare a temp folder: %v", err)
		view.requestRender()
		return
	}
	tmp := filepath.Join(dir, fmt.Sprintf("clipboard-%s.png", time.Now().Format("150405.000000")))
	if out, err := exec.Command(pngpaste, tmp).CombinedOutput(); err != nil {
		view.AddServiceMessage("Nothing to attach in the clipboard: %s", strings.TrimSpace(string(out)))
		view.requestRender()
		return
	}
	view.attachTemp(tmp)
}

// AttachPicked opens the native file picker and attaches the choice.
func (view *RoomView) AttachPicked() {
	defer debug.Recover()
	paths, err := pickFiles()
	if err != nil {
		view.AddServiceMessage("Couldn't open the file picker: %v", err)
		view.requestRender()
		return
	}
	if len(paths) > 0 {
		view.AttachFiles(paths...)
	}
}

// fileIcon picks a Nerd Font icon for a chip from the file extension.
func fileIcon(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".heic", ".bmp", ".tiff":
		return "\uf03e" // nf-fa-image
	case ".mp4", ".mov", ".mkv", ".webm", ".avi":
		return "\uf03d" // nf-fa-video_camera
	case ".mp3", ".ogg", ".opus", ".m4a", ".wav", ".flac", ".aac":
		return "\uf001" // nf-fa-music
	case ".pdf":
		return "\uf1c1" // nf-fa-file_pdf_o
	case ".zip", ".tar", ".gz", ".tgz", ".rar", ".7z", ".xz":
		return "\uf1c6" // nf-fa-file_archive_o
	}
	return "\uf15b" // nf-fa-file
}

const chipNameMax = 24

// attachmentChips lays out "[ name]" chips within width, with "+N" for
// the ones that don't fit.
func attachmentChips(items []pendingFile, width int) (chips []string, more int) {
	used := 0
	for i, a := range items {
		name := runewidth.Truncate(filepath.Base(a.path), chipNameMax, "…")
		chip := "[" + fileIcon(a.path) + " " + name + "]"
		if a.sticker != nil {
			chip = "[\uf118 sticker]" // nf-fa-smile_o
		}
		w := runewidth.StringWidth(chip) + 1
		rest := len(items) - i - 1
		reserve := 0
		if rest > 0 {
			reserve = len(fmt.Sprintf("+%d", rest))
		}
		if used+w+reserve > width {
			return chips, len(items) - i
		}
		chips = append(chips, chip)
		used += w
	}
	return chips, 0
}

func (view *RoomView) drawAttachments(screen mauview.Screen, x, y, width int) {
	items := view.Attachments()
	if len(items) == 0 {
		return
	}
	chips, more := attachmentChips(items, width)
	style := tcell.StyleDefault.Foreground(ColorComposerBar)
	for _, chip := range chips {
		widget.WriteLine(screen, mauview.AlignLeft, chip, x, y, width, style)
		x += runewidth.StringWidth(chip) + 1
	}
	if more > 0 {
		widget.WriteLine(screen, mauview.AlignLeft, fmt.Sprintf("+%d", more), x, y, width, tcell.StyleDefault.Foreground(ColorStatusText))
	}
}

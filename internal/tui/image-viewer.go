package tui

import (
	"os"
	"strings"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/gomuks/tui/debug"
	"go.mau.fi/gomuks/tui/messages"
)

// isViewableImage covers photos, stickers and link-post thumbnails.
func isViewableImage(msg *messages.FileMessage) bool {
	return msg.Type == event.MsgImage || strings.HasPrefix(msg.MimeType, "image/")
}

// PreviewImage shows the original image full screen. The chat steps aside
// as it does for video; any key brings it back.
func (view *RoomView) PreviewImage(msg *messages.FileMessage) {
	defer debug.Recover()
	if !isViewableImage(msg) {
		return
	}
	data, err := view.parent.matrix.Download(msg.URL, msg.IsEncrypted)
	if err != nil {
		view.AddServiceMessage("Failed to fetch image: %v", err)
		view.parent.parent.Render()
		return
	}
	fd := int(os.Stdin.Fd())
	ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
	if err != nil || ws.Col == 0 || ws.Row == 0 {
		view.AddServiceMessage("Can't measure the terminal for the image viewer")
		view.parent.parent.Render()
		return
	}
	termCols, termRows := int(ws.Col), int(ws.Row)
	// Pixel size lets the image fill the window sharply; without it, assume
	// the usual 1:2 cell shape.
	cellW, cellH := 10, 20
	if ws.Xpixel > 0 && ws.Ypixel > 0 {
		cellW, cellH = int(ws.Xpixel)/termCols, int(ws.Ypixel)/termRows
	}
	pngData, w, h, err := messages.FullscreenPNG(data, termCols*cellW, termRows*cellH)
	if err != nil {
		view.AddServiceMessage("Can't display that image: %v", err)
		view.parent.parent.Render()
		return
	}
	cols, rows := messages.FitCells(w, h, termCols, termRows, cellW, cellH)

	view.parent.parent.app.Suspend(func() {
		if err := messages.WriteFullscreen(os.Stdout, pngData, cols, rows, termCols, termRows); err != nil {
			debug.Print("Failed to draw image viewer:", err)
		}
		// tcell restored cooked mode on suspend; raw mode returns on the
		// first key press without echoing it.
		if state, err := term.MakeRaw(fd); err == nil {
			// One read takes a whole key sequence (arrows are 3 bytes), so
			// nothing leaks into the chat after it resumes.
			buf := make([]byte, 64)
			_, _ = os.Stdin.Read(buf)
			_ = term.Restore(fd, state)
		}
		messages.ClearFullscreen(os.Stdout)
	})
	RequestRepaint()
	view.parent.parent.Render()
}

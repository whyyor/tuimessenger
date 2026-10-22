package tui

import (
	"os/exec"
	"strings"

	"maunium.net/go/mautrix/event"

	"go.mau.fi/gomuks/tui/messages"
)

type spaceAction int

const (
	spaceNothing spaceAction = iota
	spaceViewImage
	spacePlayAudio
	spaceOpenFile // video (IINA) and anything else: the default macOS app
	spaceOpenLink
)

// spaceActionFor picks what Space does for a selected message.
func spaceActionFor(msg *messages.UIMessage) (spaceAction, string) {
	if msg == nil {
		return spaceNothing, ""
	}
	if fm, ok := msg.Renderer.(*messages.FileMessage); ok {
		switch {
		case isViewableImage(fm):
			return spaceViewImage, ""
		case fm.Type == event.MsgAudio || strings.HasPrefix(fm.MimeType, "audio/"):
			return spacePlayAudio, ""
		case !fm.URL.IsEmpty():
			return spaceOpenFile, ""
		}
		return spaceNothing, ""
	}
	if url := messages.FirstURL(msg.Renderer.PlainText()); url != "" {
		return spaceOpenLink, url
	}
	return spaceNothing, ""
}

// OpenSelected is Space in visual mode: view, play or open the message.
// The selection stays, so the next one is a j/k and Space away.
func (view *RoomView) OpenSelected(msg *messages.UIMessage) {
	action, url := spaceActionFor(msg)
	fm, _ := msg.Renderer.(*messages.FileMessage)
	switch action {
	case spaceViewImage:
		go view.PreviewImage(fm)
	case spacePlayAudio:
		go view.PlayMedia(fm)
	case spaceOpenFile:
		go view.SaveOrOpenMedia(fm, true)
	case spaceOpenLink:
		_ = exec.Command("open", url).Start()
		view.AddServiceMessage("Opening %s", url)
	}
}

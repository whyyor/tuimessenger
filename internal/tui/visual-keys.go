package tui

import (
	"strings"

	"github.com/whyyor/tuimessenger/internal/tui/messages"
)

// visualVerbs are the visual-mode actions that act on the selected message the way the matching command would.
var visualVerbs = map[string]SelectReason{
	"reply":         SelectReply,
	"play":          SelectPlay,
	"open_external": SelectOpen,
	"download":      SelectDownload,
	"copy":          SelectCopy,
	"edit":          SelectEdit,
	"delete":        SelectRedact,
}

// onVisualKey does a visual-mode action (keybindings' visual table); false if it isn't one.
func (view *RoomView) onVisualKey(action string) bool {
	msgView := view.MessageView()
	selected := msgView.GetSelected()
	// done leaves visual mode for the message box, after actions that finish the job.
	done := func() {
		view.StopSelecting()
		view.input.Focus()
	}
	switch action {
	case "clear":
		RequestRepaint()
		// First Escape drops a v range and stays in visual mode.
		if msgView.HasAnchor() {
			msgView.ClearAnchor()
			return true
		}
		view.ClearAllContext()
	case "exit":
		RequestRepaint()
		view.ClearAllContext()
	case "select_prev":
		view.SelectPrevious()
	case "select_next":
		view.SelectNext()
	case "confirm":
		view.OnSelect(selected)
	case "range":
		msgView.ToggleAnchor()
	case "open":
		if selected != nil {
			view.OpenSelected(selected)
		}
	case "react":
		if selected != nil {
			view.OpenReactPicker(selected.ID)
		}
	case "info":
		view.OpenMessageInfo(selected)
	case "retry":
		if selected != nil {
			go view.RetryMessage(selected)
		}
		done()
	case "hide":
		if selected != nil {
			go view.HideMessage(selected.ID)
		}
		done()
	case "copy_link":
		if selected != nil {
			if url := messages.FirstURL(selected.Renderer.PlainText()); url != "" {
				go view.CopyToClipboard(url, "clipboard")
				view.AddServiceMessage("Copied %s", url)
			} else {
				view.AddServiceMessage("No URL in that message")
			}
		}
		done()
	case "copy":
		// With a range, copy every message in it (just a sticker: copy it to send again).
		if msgView.HasAnchor() {
			msgs := msgView.RangeMessages()
			if len(msgs) == 1 && isSticker(msgs[0]) {
				view.CopySticker(msgs[0])
			} else {
				go view.CopyToClipboard(formatRange(msgs), "clipboard")
				view.AddServiceMessage("Copied %d messages", len(msgs))
			}
			done()
			return true
		}
		return view.visualVerb(action, selected)
	default:
		if n, ok := strings.CutPrefix(action, "quick_react_"); ok && len(n) == 1 && n[0] >= '1' && int(n[0]-'1') < len(quickReactions) {
			if selected != nil {
				go view.ToggleReaction(selected.ID, quickReactions[n[0]-'1'])
			}
			done()
			return true
		}
		return view.visualVerb(action, selected)
	}
	return true
}

// visualVerb runs reply, play, open, download, copy, edit or delete on the selected message.
func (view *RoomView) visualVerb(action string, selected *messages.UIMessage) bool {
	reason, ok := visualVerbs[action]
	if !ok {
		return false
	}
	view.selectReason = reason
	view.OnSelect(selected)
	return true
}

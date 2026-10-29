// gomuks - A terminal Matrix client written in Go.
// Copyright (C) 2025 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package tui

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"go.mau.fi/mauview"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/pkg/rpc/client"
	"go.mau.fi/gomuks/pkg/rpc/store"
	"go.mau.fi/gomuks/tui/config"
	"go.mau.fi/gomuks/tui/messages"
	"go.mau.fi/gomuks/tui/widget"
)

type MessageView struct {
	parent *RoomView
	config *config.Config
	matrix *client.GomuksClient
	lock   sync.RWMutex

	SenderWidth     int
	DateFormat      string
	TimestampFormat string
	TimestampWidth  int

	ScrollOffset atomic.Int32
	height       atomic.Uint32
	totalHeight  atomic.Uint32

	msgBuffer    []*messages.UIMessage
	prevTimeline *[]*database.Event
	prevWidth    int
	selected     database.EventRowID
	// anchor is the other end of a visual-mode range (v); 0 means none.
	anchor database.EventRowID
	// clearedAt is the /clear point msgBuffer was built with; clearAD is the
	// account data it came from, replaced by sync on every change.
	clearedAt time.Time
	clearAD   *database.AccountData
	// drawWidth is the width of the last frame, for rebuilding outside Draw.
	drawWidth int
	// followSelection asks the next Draw to scroll the selected message into
	// view; Draw is where msgBuffer's line layout is current.
	followSelection bool
	// Screen rows of the selected message last frame (-1: off screen) and
	// the body column, for anchoring the reaction popup.
	selTop, selBottom int
	lastMessageX      int
	// layouts is set in bubble mode: where each message in msgBuffer sits.
	layouts map[*messages.UIMessage]*bubbleLayout
}

func NewMessageView(parent *RoomView) *MessageView {
	mv := &MessageView{
		parent: parent,
		config: parent.config,
		matrix: parent.parent.matrix,

		SenderWidth:    15,
		TimestampWidth: len(messages.TimeFormat),
	}
	return mv
}

func (view *MessageView) SetSelected(message *messages.UIMessage) {
	if message == nil || (view.selected == message.RowID || message.IsService) {
		view.selected = 0
	} else {
		view.selected = message.RowID
	}
}

// SelectAdjacent moves the selection to the next (newer) or previous (older)
// message passing the filter, starting from the newest when nothing is
// selected yet. Returns whether a selection was made.
func (view *MessageView) SelectAdjacent(forward bool, allow func(*messages.UIMessage) bool) bool {
	view.lock.Lock()
	defer view.lock.Unlock()
	var msgs []*messages.UIMessage
	var firstLines []int
	var prev *messages.UIMessage
	for line, m := range view.msgBuffer {
		if m != prev {
			msgs = append(msgs, m)
			firstLines = append(firstLines, line)
			prev = m
		}
	}
	if len(msgs) == 0 {
		return false
	}
	idx := -1
	if view.selected != 0 {
		for i, m := range msgs {
			if m.RowID == view.selected {
				idx = i
				break
			}
		}
	}
	step := -1
	if forward {
		step = 1
	}
	if idx == -1 {
		if forward {
			idx = -1
		} else {
			// Nothing selected: start from the newest message on screen, not
			// the newest loaded, so entering visual mode while scrolled up
			// doesn't jump the view back to the bottom.
			bottom := bottomVisibleLine(len(view.msgBuffer), view.GetScrollOffset())
			idx = 0
			for i, first := range firstLines {
				if first <= bottom {
					idx = i
				}
			}
			idx++ // the loop below steps back onto it
		}
	}
	for i := idx + step; i >= 0 && i < len(msgs); i += step {
		m := msgs[i]
		if m.IsService || m.ID == "" || (allow != nil && !allow(m)) {
			continue
		}
		view.selected = m.RowID
		view.followSelection = true
		return true
	}
	return false
}

// bottomVisibleLine is the msgBuffer index of the last line on screen.
// ScrollOffset counts lines up from the bottom of the timeline.
func bottomVisibleLine(total, scroll int) int {
	return total - scroll - 1
}

// scrollForSpan returns the scroll offset that brings lines [first, last]
// into a viewport of the given height, moving as little as possible: a span
// above the window is aligned to the top, below it to the bottom, and an
// already visible span leaves the scroll unchanged.
func scrollForSpan(total, height, scroll, first, last int) int {
	top := total - scroll - height
	switch {
	case first < top:
		scroll = total - height - first
	case last >= top+height:
		scroll = total - 1 - last
	}
	if limit := total - height + PaddingAtTop; scroll > limit {
		scroll = limit
	}
	if scroll < 0 {
		scroll = 0
	}
	return scroll
}

// scrollToSelectedLocked applies scrollForSpan to the selected message.
// Caller holds view.lock and has just rebuilt msgBuffer.
func (view *MessageView) scrollToSelectedLocked(height int) {
	if view.selected == 0 {
		return
	}
	first, last := -1, -1
	for i, m := range view.msgBuffer {
		if m.RowID == view.selected {
			if first == -1 {
				first = i
			}
			last = i
		}
	}
	if first == -1 {
		return
	}
	scroll := scrollForSpan(len(view.msgBuffer), height, view.GetScrollOffset(), first, last)
	view.ScrollOffset.Store(int32(scroll))
}

func (view *MessageView) GetSelected() *messages.UIMessage {
	view.lock.RLock()
	defer view.lock.RUnlock()
	evt := view.parent.Room.GetEventByRowID(view.selected)
	if evt == nil || evt.RenderMeta == nil {
		return nil
	}
	return evt.RenderMeta.(*messages.UIMessage)
}

// ToggleAnchor starts a range at the selected message, or drops the range.
func (view *MessageView) ToggleAnchor() {
	if view.anchor != 0 {
		view.anchor = 0
	} else {
		view.anchor = view.selected
	}
}

func (view *MessageView) HasAnchor() bool {
	return view.anchor != 0
}

func (view *MessageView) ClearAnchor() {
	view.anchor = 0
}

// rangeLinesLocked returns the msgBuffer line span from the anchor to the
// selection, whichever comes first, covering both messages fully.
func (view *MessageView) rangeLinesLocked() (lo, hi int, ok bool) {
	if view.anchor == 0 || view.selected == 0 {
		return 0, 0, false
	}
	aFirst, aLast, sFirst, sLast := -1, -1, -1, -1
	for i, m := range view.msgBuffer {
		if m.Event == nil {
			continue
		}
		if m.RowID == view.anchor {
			if aFirst < 0 {
				aFirst = i
			}
			aLast = i
		}
		if m.RowID == view.selected {
			if sFirst < 0 {
				sFirst = i
			}
			sLast = i
		}
	}
	if aFirst < 0 || sFirst < 0 {
		return 0, 0, false
	}
	return min(aFirst, sFirst), max(aLast, sLast), true
}

// RangeMessages returns the real messages in the visual-mode range, oldest
// first, skipping service lines such as date changes.
func (view *MessageView) RangeMessages() []*messages.UIMessage {
	view.lock.RLock()
	defer view.lock.RUnlock()
	lo, hi, ok := view.rangeLinesLocked()
	if !ok {
		return nil
	}
	var out []*messages.UIMessage
	var prev *messages.UIMessage
	for _, m := range view.msgBuffer[lo : hi+1] {
		if m != prev && !m.IsService {
			out = append(out, m)
		}
		prev = m
	}
	return out
}

// formatRange renders messages one per line as "HH:MM Sender: text".
func formatRange(msgs []*messages.UIMessage) string {
	var buf strings.Builder
	for _, m := range msgs {
		// A sticker's plain text is a localhost media link, useless to anyone else.
		text := "[sticker]"
		if !isSticker(m) {
			text = m.PlainText()
		}
		fmt.Fprintf(&buf, "%s %s: %s\n", m.FormatTime(), m.GetRawSenderName(), text)
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

func (view *MessageView) handleMessageClick(message *messages.UIMessage, mod tcell.ModMask) bool {
	//if msg, ok := message.Renderer.(*messages.FileMessage); ok && mod > 0 && !msg.Thumbnail.IsEmpty() {
	//	debug.Print("Opening thumbnail", msg.ThumbnailPath())
	//	open.Open(msg.ThumbnailPath())
	//	// No need to re-render
	//	return false
	//}
	view.parent.ClickSelect(message)
	return true
}

func (view *MessageView) handleUsernameClick(message *messages.UIMessage, prevMessage *messages.UIMessage) bool {
	// TODO this is needed if senders are hidden for messages from the same sender (see Draw method)
	//if prevMessage != nil && prevMessage.SenderName == message.SenderName {
	//	return false
	//}

	senderName := message.GetRawSenderName()
	if senderName == "---" || senderName == "-->" || senderName == "<--" || message.MsgType == event.MsgEmote {
		return false
	}

	sender := format.MarkdownMentionWithName(senderName, message.Sender)

	cursorPos := view.parent.input.GetCursorOffset()
	text := view.parent.input.GetText()
	var buf strings.Builder
	if cursorPos == 0 {
		buf.WriteString(sender)
		buf.WriteRune(':')
		buf.WriteRune(' ')
		buf.WriteString(text)
	} else {
		textBefore := runewidth.Truncate(text, cursorPos, "")
		textAfter := text[len(textBefore):]
		buf.WriteString(textBefore)
		buf.WriteString(sender)
		buf.WriteRune(' ')
		buf.WriteString(textAfter)
	}
	newText := buf.String()
	view.parent.input.SetText(string(newText))
	view.parent.input.SetCursorOffset(cursorPos + len(newText) - len(text))
	return true
}

func (view *MessageView) GetScrollOffset() int {
	return int(view.ScrollOffset.Load())
}

func (view *MessageView) OnMouseEvent(event mauview.MouseEvent) bool {
	if event.HasMotion() {
		return false
	}
	switch event.Buttons() {
	case tcell.WheelUp:
		if view.IsAtTop() {
			go view.parent.parent.LoadHistory(view.parent.Room.ID)
		} else {
			view.AddScrollOffset(WheelScrollOffsetDiff)
			return true
		}
	case tcell.WheelDown:
		view.AddScrollOffset(-WheelScrollOffsetDiff)
		view.parent.parent.MarkRead(view.parent)
		return true
	case tcell.Button1:
		x, y := event.Position()
		line := view.TotalHeight() - int(view.ScrollOffset.Load()) - view.Height() + y
		if line < 0 || line >= view.TotalHeight() {
			return false
		}

		view.lock.RLock()
		message := view.msgBuffer[line]
		var prevMessage *messages.UIMessage
		if y != 0 && line > 0 {
			prevMessage = view.msgBuffer[line-1]
		}
		lay := view.layouts[message]
		offset := 0
		for i := line - 1; i >= 0 && view.msgBuffer[i] == message; i-- {
			offset++
		}
		view.lock.RUnlock()

		if lay != nil {
			switch {
			case lay.service, lay.gap && offset == 0:
				return false
			case lay.name && offset == boolInt(lay.gap):
				return view.handleUsernameClick(message, prevMessage)
			}
			return view.handleMessageClick(message, event.Modifiers())
		}

		usernameX := 0
		if !view.config.Preferences.HideTimestamp {
			usernameX += view.TimestampWidth + TimestampSenderGap
		}
		messageX := usernameX + view.SenderWidth + SenderMessageGap

		if x >= messageX {
			return view.handleMessageClick(message, event.Modifiers())
		} else if x >= usernameX && x < messageX-SenderMessageGap {
			return view.handleUsernameClick(message, prevMessage)
		}
	}
	return false
}

const PaddingAtTop = 5

// scrollAnchor picks what to hold still while the timeline changes: the
// selected message in visual mode, else (scrolled up) the message on the
// viewport's bottom row. rel is that row minus the message's last line.
// At the bottom and not selecting there's no anchor: new messages show.
func (view *MessageView) scrollAnchor(offset int) (database.EventRowID, int, bool) {
	buf := view.msgBuffer
	bottom := len(buf) - 1 - offset
	if len(buf) == 0 || bottom < 0 {
		return 0, 0, false
	}
	row := view.selected
	if row == 0 {
		if offset == 0 {
			return 0, 0, false
		}
		for i := min(bottom, len(buf)-1); i >= 0 && row == 0; i-- {
			row = buf[i].RowID
		}
	}
	last := lastLineOf(buf, row)
	if last < 0 {
		return 0, 0, false
	}
	return row, bottom - last, true
}

// anchoredOffset is the scroll offset that puts the anchor back on its row.
func anchoredOffset(buf []*messages.UIMessage, row database.EventRowID, rel int) (int, bool) {
	last := lastLineOf(buf, row)
	if last < 0 {
		return 0, false
	}
	return max(len(buf)-1-last-rel, 0), true
}

func lastLineOf(buf []*messages.UIMessage, row database.EventRowID) int {
	if row == 0 {
		return -1
	}
	for i := len(buf) - 1; i >= 0; i-- {
		if buf[i].RowID == row {
			return i
		}
	}
	return -1
}

func (view *MessageView) AddScrollOffset(diff int) {
	totalHeight := view.TotalHeight()
	height := view.Height()
	scrollOffset := int(view.ScrollOffset.Load())
	if diff >= 0 && scrollOffset+diff >= totalHeight-height+PaddingAtTop {
		scrollOffset = totalHeight - height + PaddingAtTop
	} else {
		scrollOffset += diff
	}

	if scrollOffset > totalHeight-height+PaddingAtTop {
		scrollOffset = totalHeight - height + PaddingAtTop
	}
	if scrollOffset < 0 {
		scrollOffset = 0
	}
	view.ScrollOffset.Store(int32(scrollOffset))
}

func (view *MessageView) Height() int {
	return int(view.height.Load())
}

func (view *MessageView) TotalHeight() int {
	return int(view.totalHeight.Load())
}

func (view *MessageView) IsAtTop() bool {
	return view.GetScrollOffset() >= view.TotalHeight()-view.Height()+PaddingAtTop
}

const (
	TimestampSenderGap = 1
	SenderSeparatorGap = 1
	SenderMessageGap   = 3
)

func (view *MessageView) getIndexOffset(screen mauview.Screen, height, messageX int) (indexOffset int) {
	indexOffset = view.TotalHeight() - view.GetScrollOffset() - height
	// A cleared chat shows no hint: there's nothing more to load.
	if indexOffset <= -PaddingAtTop && view.clearedAt.IsZero() {
		message := "Scroll up to load more messages."
		if view.parent.Room.Paginating.Load() {
			message = "Loading more messages..."
		}
		widget.WriteLineSimpleColor(screen, message, messageX, 0, tcell.ColorGreen)
	}
	return
}

// CapturePlaintext renders every loaded message as plain text, one line per
// message, for the bare view.
func (view *MessageView) CapturePlaintext() string {
	var buf strings.Builder
	var prevMessage *messages.UIMessage
	view.lock.Lock()
	// msgBuffer is otherwise only rebuilt on draw; pick up history loaded
	// since the last frame.
	if view.drawWidth > 0 {
		view.update(view.drawWidth)
	}
	for _, message := range view.msgBuffer {
		if message != prevMessage {
			var sender string
			if len(message.GetSenderName()) > 0 {
				sender = fmt.Sprintf(" <%s>", message.GetSenderName())
			} else if message.MsgType == event.MsgEmote {
				sender = fmt.Sprintf(" * %s", message.GetRawSenderName())
			}
			fmt.Fprintf(&buf, "%s%s %s\n", message.FormatTime(), sender, message.PlainText())
			prevMessage = message
		}
	}
	view.lock.Unlock()
	return buf.String()
}

func (view *MessageView) Draw(screen mauview.Screen) {
	view.lock.Lock()
	defer view.lock.Unlock()
	width, height := screen.Size()
	view.height.Store(uint32(height))
	view.drawWidth = width
	view.update(width)
	if view.followSelection {
		view.followSelection = false
		view.scrollToSelectedLocked(height)
	}

	// Fill the viewport on open: if the loaded timeline is shorter than the
	// screen, keep paginating until it fills or history runs out. Manual
	// scroll-up only takes over beyond that. LoadMoreHistory's Paginating
	// CompareAndSwap makes redraw-triggered retries harmless.
	if view.TotalHeight() < height {
		if room := view.parent.Room; room.HasMoreHistory() && !room.Paginating.Load() {
			go view.parent.parent.LoadHistory(room.ID)
		}
	}

	if len(view.msgBuffer) == 0 {
		widget.WriteLineSimple(screen, "It's quite empty in here.", 0, height)
		return
	}

	usernameX := 0
	if !view.config.Preferences.HideTimestamp {
		usernameX += view.TimestampWidth + TimestampSenderGap
	}
	messageX := usernameX + view.SenderWidth + SenderMessageGap

	bareMode := view.config.Preferences.BareMessageView
	if bareMode {
		messageX = 0
	}
	if view.layouts != nil {
		messageX = 2
	}

	indexOffset := view.getIndexOffset(screen, height, messageX)

	viewStart := 0
	if indexOffset < 0 {
		viewStart = -indexOffset
	}

	view.selTop, view.selBottom, view.lastMessageX = -1, -1, messageX
	rangeLo, rangeHi, inRange := view.rangeLinesLocked()
	for line := viewStart; line < height && indexOffset+line < len(view.msgBuffer); {
		index := indexOffset + line

		msg := view.msgBuffer[index]
		if line == viewStart {
			for i := index - 1; i >= 0 && view.msgBuffer[i] == msg; i-- {
				line--
			}
		}

		if lay := view.layouts[msg]; lay != nil {
			msg.IsSelected = view.selected != 0 && msg.RowID == view.selected ||
				inRange && index >= rangeLo && index <= rangeHi
			first, last := view.drawBubble(screen, msg, lay, line, width)
			if view.selected != 0 && msg.RowID == view.selected {
				view.selTop, view.selBottom = max(first, 0), min(last, height-1)
				view.lastMessageX = lay.x
			}
			line += lay.rows
			continue
		}
		if len(msg.FormatTime()) > 0 && !view.config.Preferences.HideTimestamp {
			widget.WriteLineSimpleColor(screen, msg.FormatTime(), 0, line, msg.TimestampColor())
		}
		// Only show the sender when it changes, so runs of messages from the
		// same person read as one block instead of repeating the name.
		firstIndex := index
		for firstIndex > 0 && view.msgBuffer[firstIndex-1] == msg {
			firstIndex--
		}
		if firstIndex == 0 || view.msgBuffer[firstIndex-1].Sender != msg.Sender {
			widget.WriteLineColor(
				screen, mauview.AlignRight, msg.GetSenderName(),
				usernameX, line, view.SenderWidth,
				msg.SenderColor())
		}
		if msg.LastEditRef != nil {
			// TODO add better indicator for edits
			screen.SetCell(usernameX+view.SenderWidth, line, tcell.StyleDefault.Foreground(tcell.ColorDarkRed), '*')
		}

		msg.IsSelected = view.selected != 0 && msg.RowID == view.selected ||
			inRange && index >= rangeLo && index <= rangeHi
		msg.Draw(mauview.NewProxyScreen(screen, messageX, line, width-messageX, msg.Height()))
		if view.selected != 0 && msg.RowID == view.selected {
			view.selTop, view.selBottom = max(line, 0), min(line+msg.Height()-1, height-1)
		}

		// Deleted-but-witnessed messages keep their content with a marker.
		if msg.RedactedBy != "" && !msg.IsService {
			label := "(deleted)"
			labelWidth := runewidth.StringWidth(label)
			widget.WriteLineColor(screen, mauview.AlignLeft, label,
				width-labelWidth-1, line, labelWidth, tcell.ColorGray)
		}

		// Read receipt cluster: this event is somebody's latest read position.
		if readers := view.visibleReaders(msg); len(readers) > 0 {
			label := "✓ seen"
			if !view.isDM() {
				label = fmt.Sprintf("✓ %d", len(readers))
			}
			markerWidth := runewidth.StringWidth(label)
			widget.WriteLineColor(screen, mauview.AlignLeft, label,
				width-markerWidth-1, line+msg.Height()-1, markerWidth, tcell.ColorGray)
		}

		line += msg.Height()
	}
}

// visibleReaders filters a receipt cluster down to people worth showing: not
// us, not the message's own sender, and not bridge bots, which mark rooms read
// on their own.
func (view *MessageView) visibleReaders(msg *messages.UIMessage) []id.UserID {
	users := view.parent.Room.ReceiptUsersAt(msg.ID)
	if len(users) == 0 {
		return nil
	}
	own := view.parent.Room.OwnUserID()
	filtered := users[:0]
	for _, user := range users {
		if user == own || user == msg.Sender || strings.HasSuffix(user.Localpart(), "bot") {
			continue
		}
		filtered = append(filtered, user)
	}
	return filtered
}

func (view *MessageView) isDM() bool {
	meta := view.parent.Room.Meta.Current()
	return meta != nil && meta.LazyLoadSummary != nil && len(meta.LazyLoadSummary.Heroes) == 1
}

func (view *MessageView) update(width int) {
	timelinePtr := view.parent.Room.TimelineCache.Current()
	clearAD := view.parent.Room.GetAccountData(store.AccountDataCleared)
	if timelinePtr == nil || timelinePtr == view.prevTimeline && width == view.prevWidth && clearAD == view.clearAD {
		return
	}
	view.clearAD = clearAD
	clearState := view.parent.Room.ClearState()
	cleared := clearState.At()
	view.clearedAt = cleared
	timeline := *timelinePtr
	var prevTimeline []*database.Event
	if view.prevTimeline != nil {
		prevTimeline = *view.prevTimeline
	}

	newBuffer := make([]*messages.UIMessage, 0, len(timeline)*2)
	var lastRowIDInPrevTimeline database.EventRowID
	if len(prevTimeline) > 0 {
		lastRowIDInPrevTimeline = prevTimeline[len(prevTimeline)-1].RowID
	}
	increaseScrollOffset := false
	bare := view.config.Preferences.BareMessageView
	fullWidth := width
	bubbles := !bare && !view.config.Preferences.ClassicLayout
	var layouts map[*messages.UIMessage]*bubbleLayout
	isDM := false
	if bubbles {
		layouts = make(map[*messages.UIMessage]*bubbleLayout, len(timeline))
		isDM = view.isDM()
	}
	var runPrev *messages.UIMessage
	var bubbleMsgs []*messages.UIMessage
	afterService := true
	if !bare && !bubbles {
		width -= view.SenderWidth + SenderMessageGap
		if !view.config.Preferences.HideTimestamp {
			width -= view.TimestampWidth + TimestampSenderGap
		}
	}
	scrollOffset := view.GetScrollOffset()
	newScrollOffset := scrollOffset
	anchorRow, anchorRel, anchored := view.scrollAnchor(scrollOffset)
	appendBuffer := func(msg *messages.UIMessage) {
		if width < 5 {
			return
		}
		var height int
		if bubbles {
			lay := view.layoutBubble(msg, fullWidth, isDM, runPrev, afterService)
			layouts[msg] = lay
			bubbleMsgs = append(bubbleMsgs, msg)
			height = lay.rows
			if msg.IsService {
				afterService = true
			} else {
				runPrev, afterService = msg, false
			}
		} else {
			msg.CalculateBuffer(view.config.Preferences, width)
			height = msg.Height()
		}
		for i := 0; i < height; i++ {
			newBuffer = append(newBuffer, msg)
		}
		if scrollOffset > 0 && increaseScrollOffset {
			newScrollOffset += height
		}
	}
	var prev *messages.UIMessage
	prevLastEventNotFound := lastRowIDInPrevTimeline != 0
	for _, evt := range timeline {
		startIncreasingScrollOffset := false
		if !increaseScrollOffset && scrollOffset > 0 && evt.RowID != 0 && evt.RowID == lastRowIDInPrevTimeline {
			startIncreasingScrollOffset = true
			prevLastEventNotFound = true
		}
		if hiddenByClear(evt, cleared) || clearState.IsHidden(evt.ID) {
			increaseScrollOffset = increaseScrollOffset || startIncreasingScrollOffset
			continue
		}
		if evt.RenderMeta == nil {
			evt.RenderMeta = messages.ParseEvent(view.matrix, &view.config.Preferences, view.parent.Room, evt)
		}
		uiMsg := evt.RenderMeta.(*messages.UIMessage)
		if uiMsg == nil {
			continue
		}
		if !uiMsg.SameDate(prev) {
			dateChange := messages.NewDateChangeMessage(view.parent.Room, fmt.Sprintf("Date changed to %s", uiMsg.FormatDate()))
			// The rule's label (Today, Yesterday…) comes from the day it introduces.
			dateChange.Timestamp = uiMsg.Timestamp
			appendBuffer(dateChange)
		}
		appendBuffer(uiMsg)
		prev = uiMsg
		if startIncreasingScrollOffset {
			increaseScrollOffset = true
		}
	}
	if scrollOffset > 0 && !increaseScrollOffset && !prevLastEventNotFound {
		// Previous last message wasn't found, so reset scroll position
		newScrollOffset = 0
	}
	if anchored {
		if offset, ok := anchoredOffset(newBuffer, anchorRow, anchorRel); ok {
			newScrollOffset = offset
		}
	}
	if newScrollOffset != scrollOffset {
		view.ScrollOffset.Store(int32(newScrollOffset))
	}
	if bubbles {
		view.markRead(bubbleMsgs, layouts)
	}
	view.msgBuffer = newBuffer
	view.layouts = layouts
	view.totalHeight.Store(uint32(len(newBuffer)))
	view.prevTimeline = timelinePtr
}

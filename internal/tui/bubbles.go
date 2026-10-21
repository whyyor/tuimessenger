package tui

import (
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"go.mau.fi/mauview"

	"go.mau.fi/gomuks/tui/messages"
	"go.mau.fi/gomuks/tui/widget"
)

const (
	// bubbleMaxShare is the widest a bubble gets, in percent of the pane.
	bubbleMaxShare = 75
	bubbleMinWidth = 20
	// bubbleRunGap starts a new run (name, blank line) after this pause.
	bubbleRunGap = 10 * time.Minute
)

// bubbleLayout is where one message sits in bubble mode.
type bubbleLayout struct {
	service bool
	own     bool
	gap     bool // blank row before a new run
	name    bool // sender row (groups, others, first of run)
	metaRow bool // time didn't fit on the last line
	read    bool // own message seen by someone else
	meta    string
	inner   int // content width inside the bar and padding
	x       int // content column
	barX    int
	rows    int
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// bubbleInnerMax is the content width a bubble may use in a pane.
func bubbleInnerMax(width int) int {
	limit := max(min(bubbleMinWidth, width), width*bubbleMaxShare/100)
	return max(limit-2, 1)
}

// bubbleMeta is "edited 16:40": the small print; ticks are drawn after it.
func (view *MessageView) bubbleMeta(msg *messages.UIMessage) string {
	var parts []string
	switch {
	case msg.RedactedBy != "":
		parts = append(parts, "deleted")
	case msg.LastEditRef != nil:
		parts = append(parts, "edited")
	}
	parts = append(parts, msg.FormatTime())
	return strings.Join(parts, " ")
}

// startsRun reports whether msg begins a new block of one sender's messages.
func startsRun(msg, prev *messages.UIMessage, afterService bool) bool {
	return afterService || prev == nil || prev.Sender != msg.Sender ||
		msg.Time().Sub(prev.Time()) > bubbleRunGap
}

// layoutBubble wraps msg for the pane and decides its placement.
func (view *MessageView) layoutBubble(msg *messages.UIMessage, width int, isDM bool, prev *messages.UIMessage, afterService bool) *bubbleLayout {
	prefs := view.config.Preferences
	lay := &bubbleLayout{}
	if msg.IsService {
		lay.service = true
		if msg.IsDateChange {
			lay.rows = 1
			return lay
		}
		lay.x, lay.inner = 2, max(width-2, 1)
		msg.CalculateBuffer(prefs, lay.inner)
		lay.rows = msg.Height()
		return lay
	}
	maxInner := bubbleInnerMax(width)
	msg.CalculateBuffer(prefs, maxInner)
	content, last := msg.Measure(maxInner)
	lay.own = msg.Sender == view.parent.Room.OwnUserID()
	lay.meta = view.bubbleMeta(msg)
	metaW := runewidth.StringWidth(lay.meta)
	if lay.own {
		metaW += 1 + tickWidth
	}
	if last+2+metaW <= maxInner {
		lay.inner = max(content, last+2+metaW)
	} else {
		lay.inner = max(content, metaW)
		lay.metaRow = true
	}
	lay.inner = min(max(lay.inner, 1), maxInner)

	first := startsRun(msg, prev, afterService)
	lay.gap = first && prev != nil && !afterService
	lay.name = first && !lay.own && !isDM
	if lay.own {
		lay.barX = width - 1
		lay.x = max(lay.barX-1-lay.inner, 0)
	} else {
		lay.barX, lay.x = 0, 2
	}
	lay.rows = boolInt(lay.gap) + boolInt(lay.name) + msg.Height() + boolInt(lay.metaRow)
	return lay
}

// drawBubble draws msg's block starting at row top; returns the rows of
// the message itself (excluding gap and name), for anchoring popups.
func (view *MessageView) drawBubble(screen mauview.Screen, msg *messages.UIMessage, lay *bubbleLayout, top, width int) (first, last int) {
	dim := tcell.StyleDefault.Foreground(ColorStatusText)
	if lay.service {
		if msg.IsDateChange {
			drawDateRule(screen, strings.TrimPrefix(msg.PlainText(), "Date changed to "), top, width)
		} else {
			msg.Draw(mauview.NewProxyScreen(screen, lay.x, top, lay.inner, msg.Height()))
		}
		return top, top + lay.rows - 1
	}
	row := top + boolInt(lay.gap)
	if lay.name {
		widget.WriteLine(screen, mauview.AlignLeft, msg.GetSenderName(), 1, row, width-1,
			tcell.StyleDefault.Foreground(msg.SenderColor()).Bold(true))
		row++
	}
	h := msg.Height()
	total := h + boolInt(lay.metaRow)
	barColor := ColorComposerBar
	if !lay.own {
		barColor = msg.SenderColor()
	}
	for y := range total {
		screen.SetContent(lay.barX, row+y, '┃', nil, tcell.StyleDefault.Foreground(barColor))
	}
	msg.Draw(mauview.NewProxyScreen(screen, lay.x, row, lay.inner, h))

	metaY := row + h - 1
	if lay.metaRow {
		metaY = row + h
		if msg.IsSelected {
			widget.WriteLinePadded(screen, mauview.AlignLeft, "", lay.x, metaY, lay.inner, tcell.StyleDefault.Background(tcell.ColorDarkGreen))
		}
	}
	metaStyle := dim
	if msg.IsSelected {
		metaStyle = metaStyle.Background(tcell.ColorDarkGreen)
	}
	end := lay.x + lay.inner
	if lay.own {
		tick, color := view.tickFor(msg, lay)
		tickStyle := metaStyle.Foreground(color)
		widget.WriteLinePadded(screen, mauview.AlignLeft, tick, end-tickWidth, metaY, tickWidth, tickStyle)
		end -= tickWidth + 1
	}
	meta := runewidth.Truncate(lay.meta, max(end-lay.x, 0), "")
	metaW := runewidth.StringWidth(meta)
	widget.WriteLine(screen, mauview.AlignLeft, meta, end-metaW, metaY, metaW, metaStyle)
	return row, row + total - 1
}

// drawDateRule draws "──── October 20, 2026 ────" across the pane.
func drawDateRule(screen mauview.Screen, label string, y, width int) {
	style := tcell.StyleDefault.Foreground(ColorStatusText)
	for x := range width {
		screen.SetContent(x, y, '─', nil, style)
	}
	label = " " + strings.TrimSpace(label) + " "
	lw := runewidth.StringWidth(label)
	widget.WriteLine(screen, mauview.AlignLeft, label, max((width-lw)/2, 0), y, width, style)
}

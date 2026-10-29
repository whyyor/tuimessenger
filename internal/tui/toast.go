package tui

import (
	"html"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"go.mau.fi/mauview"

	"go.mau.fi/gomuks/tui/widget"
)

// Toasts replace gomuks' notices in the timeline: opencode's layout (top
// right, a coloured ┃ each side, click to dismiss) in our theme: a Nerd Font
// icon per kind, Kanagawa colours, transparent background.
type toastKind int

const (
	toastInfo toastKind = iota
	toastSuccess
	toastWarning
	toastError
)

const (
	toastDuration      = 5 * time.Second
	toastErrorDuration = 7 * time.Second
	// toastMinShow is how long a toast stays before a newer one replaces it.
	toastMinShow  = time.Second
	toastMaxWidth = 60
	toastMaxLines = 6
)

var toastColors = map[toastKind]tcell.Color{
	toastInfo:    ColorPillBlue,
	toastSuccess: ColorPillGreen,
	toastWarning: ColorPillYellow,
	toastError:   tcell.NewHexColor(0xe46876),
}

var toastIcons = map[toastKind]string{
	toastInfo:    "\uf05a",
	toastSuccess: "\uf00c",
	toastWarning: "\uf071",
	toastError:   "\uf057",
}

type toast struct {
	kind  toastKind
	text  string
	shown time.Time
}

type toaster struct {
	mu      sync.Mutex
	current *toast
	queue   []*toast
	timer   *time.Timer
	render  func()
	// The last drawn box, for clicks.
	x, y, w, h int
}

var toasts = &toaster{}

// now and after are swapped in tests.
var (
	toastNow   = time.Now
	toastAfter = time.AfterFunc
)

// Show displays text now, or queues it behind a toast still being read.
func (t *toaster) Show(kind toastKind, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	t.mu.Lock()
	next := &toast{kind: kind, text: text}
	switch {
	case t.current != nil && t.current.text == text && t.current.kind == kind:
		t.mu.Unlock()
		return
	case t.current != nil && (len(t.queue) > 0 || toastNow().Sub(t.current.shown) < toastMinShow):
		t.queue = append(t.queue, next)
	default:
		t.showLocked(next)
	}
	render := t.render
	t.mu.Unlock()
	if render != nil {
		render()
	}
}

func (t *toaster) showLocked(next *toast) {
	if t.timer != nil {
		t.timer.Stop()
	}
	t.current = next
	if next == nil {
		t.timer = nil
		return
	}
	next.shown = toastNow()
	d := toastDuration
	if next.kind == toastError {
		d = toastErrorDuration
	}
	t.timer = toastAfter(d, t.expire(next))
}

func (t *toaster) expire(which *toast) func() {
	return func() {
		t.mu.Lock()
		if t.current != which {
			t.mu.Unlock()
			return
		}
		t.dismissLocked()
		render := t.render
		t.mu.Unlock()
		if render != nil {
			render()
		}
	}
}

func (t *toaster) dismissLocked() {
	var next *toast
	if len(t.queue) > 0 {
		next, t.queue = t.queue[0], t.queue[1:]
	}
	t.showLocked(next)
}

// Dismiss closes the current toast and shows the next queued one.
func (t *toaster) Dismiss() {
	t.mu.Lock()
	t.dismissLocked()
	t.mu.Unlock()
	RequestRepaint()
}

// Hit reports whether a screen cell is on the visible toast.
func (t *toaster) Hit(x, y int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.current != nil && x >= t.x && x < t.x+t.w && y >= t.y && y < t.y+t.h
}

// wrapText breaks text into lines at most width cells wide, by word.
func wrapText(text string, width int) []string {
	var lines []string
	for _, para := range strings.Split(text, "\n") {
		line := ""
		for _, word := range strings.Fields(para) {
			for runewidth.StringWidth(word) > width {
				cut := runewidth.Truncate(word, width, "")
				if line != "" {
					lines, line = append(lines, line), ""
				}
				lines = append(lines, cut)
				word = word[len(cut):]
			}
			switch {
			case line == "":
				line = word
			case runewidth.StringWidth(line)+1+runewidth.StringWidth(word) <= width:
				line += " " + word
			default:
				lines, line = append(lines, line), word
			}
		}
		lines = append(lines, line)
	}
	return lines
}

// Draw paints the toast at the top right, on the terminal background.
func (t *toaster) Draw(screen mauview.Screen) {
	t.mu.Lock()
	defer t.mu.Unlock()
	cur := t.current
	if cur == nil {
		t.w, t.h = 0, 0
		return
	}
	width, height := screen.Size()
	// ┃, two cells of padding, the icon and a space, the text, two cells,
	// x, two cells, ┃.
	textMax := min(toastMaxWidth, width-6) - 11
	if textMax < 8 {
		return
	}
	lines := wrapText(cur.text, textMax)
	if len(lines) > toastMaxLines {
		lines = lines[:toastMaxLines]
		lines[len(lines)-1] = runewidth.Truncate(lines[len(lines)-1]+" …", textMax, "…")
	}
	textW := 0
	for _, l := range lines {
		textW = max(textW, runewidth.StringWidth(l))
	}
	w := textW + 11
	h := len(lines) + 2
	if len(t.queue) > 0 {
		h += 2
	}
	h = min(h, height-1)
	x, y := width-w-2, 1
	t.x, t.y, t.w, t.h = x, y, w, h

	color := toastColors[cur.kind]
	bar := tcell.StyleDefault.Foreground(color)
	muted := tcell.StyleDefault.Foreground(ColorStatusText)
	for row := y; row < y+h; row++ {
		widget.WriteLinePadded(screen, mauview.AlignLeft, "", x, row, w, tcell.StyleDefault)
		screen.SetContent(x, row, '┃', nil, bar)
		screen.SetContent(x+w-1, row, '┃', nil, bar)
	}
	widget.WriteLine(screen, mauview.AlignLeft, toastIcons[cur.kind], x+3, y+1, 1, bar)
	for i, l := range lines {
		if y+1+i >= y+h {
			break
		}
		widget.WriteLine(screen, mauview.AlignLeft, l, x+5, y+1+i, textW, tcell.StyleDefault)
	}
	screen.SetContent(x+w-4, y+1, 'x', nil, muted)
	if n := len(t.queue); n > 0 && y+h-2 > y+len(lines) {
		widget.WriteLine(screen, mauview.AlignLeft, "+"+strconv.Itoa(n)+" more", x+5, y+h-2, textW, muted)
	}
}

// classifyNotice picks the toast colour from how gomuks words its notices.
func classifyNotice(text string) toastKind {
	lower := strings.ToLower(text)
	hasPrefix := func(prefixes ...string) bool {
		for _, p := range prefixes {
			if strings.HasPrefix(lower, p) {
				return true
			}
		}
		return false
	}
	switch {
	case hasPrefix("failed", "couldn't", "can't", "cannot", "no such", "invalid") || strings.Contains(lower, "failed to"):
		return toastError
	case hasPrefix("nothing", "no ", "that message has no", "room list not loaded", "use two slashes") ||
		strings.Contains(lower, "not found"):
		return toastWarning
	}
	if strings.HasSuffix(text, "…") || strings.HasSuffix(text, "...") {
		return toastInfo
	}
	for _, p := range []string{"opening", "playing", "fetching", "syncing", "uploading", "alias:"} {
		if strings.HasPrefix(lower, p) {
			return toastInfo
		}
	}
	return toastSuccess
}

var htmlTag = regexp.MustCompile(`<[^>]*>`)

// noticeText turns a backend notice's HTML into plain text.
func noticeText(htmlText string) string {
	htmlText = strings.NewReplacer("<br>", "\n", "<br/>", "\n", "<br />", "\n", "</p>", "\n").Replace(htmlText)
	return strings.TrimSpace(html.UnescapeString(htmlTag.ReplaceAllString(htmlText, "")))
}

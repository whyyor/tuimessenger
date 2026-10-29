package tui

import (
	"strconv"
	"sync/atomic"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"go.mau.fi/mauview"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/tui/debug"
	"go.mau.fi/gomuks/tui/lib/emoji"
	"go.mau.fi/gomuks/tui/widget"
)

// quickReactions match the 1–6 keys in visual mode.
var quickReactions = []string{"👍", "❤️", "😂", "😮", "😢", "🙏"}

const pickerMaxResults = 8

// ReactPicker is the + popup (react) and Ctrl+e (insert into the composer):
// a quick row of six, or a shortcode search.
type ReactPicker struct {
	room    *RoomView
	eventID id.EventID
	own     atomic.Pointer[map[string]id.EventID]
	quick   []string
	insert  bool

	sel       int
	searching bool
	query     string
	results   []emoji.Match
	scroll    int

	// Browse grid (empty query): tab 0 is recent, then the categories.
	cat, gridSel, gridScroll int

	// Last drawn box and quick-slot spans, in content-screen cells.
	x, y, w, h int
	slots      [][2]int
}

func (view *RoomView) OpenReactPicker(eventID id.EventID) {
	picker := &ReactPicker{room: view, eventID: eventID, quick: quickReactions, cat: 1}
	view.picker = picker
	go func() {
		defer debug.Recover()
		own := view.OwnReactions(eventID)
		picker.own.Store(&own)
		view.parent.parent.Render()
	}()
}

// OpenEmojiPicker is Ctrl+e: recent emoji, / to search, picks go into the composer.
func (view *RoomView) OpenEmojiPicker() {
	view.picker = &ReactPicker{room: view, insert: true, cat: 1,
		quick: recentEmoji.use(view.configDir()).Top(len(quickReactions))}
	RequestRepaint()
}

func (view *RoomView) configDir() string {
	if view.config == nil {
		return ""
	}
	return view.config.Dir
}

func (view *RoomView) closePicker() {
	view.picker = nil
	RequestRepaint()
}

func (p *ReactPicker) isOwn(key string) bool {
	own := p.own.Load()
	if own == nil {
		return false
	}
	for ownKey := range *own {
		if sameReaction(ownKey, key) {
			return true
		}
	}
	return false
}

func (p *ReactPicker) pick(key string) {
	view := p.room
	view.closePicker()
	recentEmoji.use(view.configDir()).Add(key)
	if p.insert {
		view.insertAtCursor(key)
		view.input.Focus()
		return
	}
	go view.ToggleReaction(p.eventID, key)
	view.StopSelecting()
	view.input.Focus()
}

func (p *ReactPicker) setQuery(q string) {
	p.query = q
	p.results = emoji.Search(q, 50)
	p.sel, p.scroll = 0, 0
}

func (p *ReactPicker) OnKeyEvent(event mauview.KeyEvent) bool {
	if p.searching {
		return p.onSearchKey(event)
	}
	if len(p.quick) == 0 {
		p.quick = quickReactions
	}
	n := len(p.quick)
	switch event.Key() {
	case tcell.KeyEscape:
		p.room.closePicker()
	case tcell.KeyLeft, tcell.KeyBacktab:
		p.sel = (p.sel + n - 1) % n
	case tcell.KeyRight, tcell.KeyTab:
		p.sel = (p.sel + 1) % n
	case tcell.KeyEnter:
		p.pick(p.quick[p.sel])
	case tcell.KeyRune:
		r := event.Rune()
		switch {
		case r >= '1' && r <= '0'+rune(n):
			p.pick(p.quick[r-'1'])
		case r == 'h':
			p.sel = (p.sel + n - 1) % n
		case r == 'l':
			p.sel = (p.sel + 1) % n
		case r == 'q':
			p.room.closePicker()
		case r == '/' || r == ':':
			p.searching = true
			p.setQuery("")
		case r >= 'a' && r <= 'z':
			p.searching = true
			p.setQuery(string(r))
		}
	}
	return true
}

func (p *ReactPicker) onSearchKey(event mauview.KeyEvent) bool {
	if p.query == "" && p.onGridKey(event) {
		return true
	}
	switch event.Key() {
	case tcell.KeyEscape:
		p.room.closePicker()
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if p.query == "" {
			p.searching = false
		} else {
			r := []rune(p.query)
			p.setQuery(string(r[:len(r)-1]))
		}
	case tcell.KeyUp, tcell.KeyBacktab:
		if p.sel > 0 {
			p.sel--
		}
	case tcell.KeyDown, tcell.KeyTab:
		if p.sel < len(p.results)-1 {
			p.sel++
		}
	case tcell.KeyEnter:
		if p.sel < len(p.results) {
			p.pick(p.results[p.sel].Emoji)
		}
	case tcell.KeyRune:
		p.setQuery(p.query + string(event.Rune()))
	}
	if p.sel < p.scroll {
		p.scroll = p.sel
	} else if p.sel >= p.scroll+pickerMaxResults {
		p.scroll = p.sel - pickerMaxResults + 1
	}
	return true
}

// OnMouseEvent picks a clicked quick slot; a click outside closes.
func (p *ReactPicker) OnMouseEvent(x, y int) {
	if x < p.x || x >= p.x+p.w || y < p.y || y >= p.y+p.h {
		p.room.closePicker()
		return
	}
	if p.gridMode() {
		p.onGridClick(x, y)
		return
	}
	if !p.searching && y == p.y+1 {
		for i, span := range p.slots {
			if x-p.x >= span[0] && x-p.x < span[1] {
				p.pick(p.quick[i])
				return
			}
		}
	}
}

// Draw places the box above the selected rows (top..bottom), else below.
func (p *ReactPicker) Draw(screen mauview.Screen, anchorX, top, bottom int) {
	width, height := screen.Size()
	var rows [][]pickerCell
	inner := 0
	switch {
	case p.gridMode():
		rows, inner = p.gridLayoutRows(), gridInner
	case p.searching:
		rows = p.searchRows()
		for _, row := range rows {
			inner = max(inner, rowWidth(row))
		}
		inner = max(inner, gridInner)
	default:
		rows = [][]pickerCell{p.quickRow()}
		inner = rowWidth(rows[0])
	}
	p.w, p.h = min(inner+2, width), len(rows)+2
	p.x = max(0, min(anchorX, width-p.w))
	switch {
	case top >= 0 && top-p.h >= 0:
		p.y = top - p.h
	case bottom >= 0 && bottom+1+p.h <= height:
		p.y = bottom + 1
	default:
		p.y = max(0, height-p.h)
	}
	title := " \uf118 React "
	if p.insert {
		title = " \uf118 Emoji "
	}
	drawThinBox(screen, p.x, p.y, p.w, p.h, title)
	for i, row := range rows {
		cx := p.x + 1
		for _, cell := range row {
			widget.WriteLine(screen, mauview.AlignLeft, cell.text, cx, p.y+1+i, p.x+p.w-1-cx, cell.style)
			cx += runewidth.StringWidth(cell.text)
		}
	}
	if p.gridMode() {
		p.drawGrid(screen)
	}
}

type pickerCell struct {
	text  string
	style tcell.Style
}

func rowWidth(row []pickerCell) int {
	w := 0
	for _, c := range row {
		w += runewidth.StringWidth(c.text)
	}
	return w
}

func (p *ReactPicker) quickRow() []pickerCell {
	dim := tcell.StyleDefault.Foreground(ColorStatusText)
	row := []pickerCell{{" ", tcell.StyleDefault}}
	p.slots = p.slots[:0]
	if len(p.quick) == 0 {
		p.quick = quickReactions
	}
	x := 1
	for i, key := range p.quick {
		style, numStyle := tcell.StyleDefault, dim
		if i == p.sel {
			style = tcell.StyleDefault.Background(ColorHighlight).Foreground(ColorOnHighlight).Bold(true)
			numStyle = tcell.StyleDefault.Background(ColorHighlight).Foreground(ColorOnHighlight)
		}
		if p.isOwn(key) {
			style = style.Underline(true)
		}
		slot := []pickerCell{{strconv.Itoa(i+1) + " ", numStyle}, {key, style}, {"  ", tcell.StyleDefault}}
		start := x
		x += rowWidth(slot)
		p.slots = append(p.slots, [2]int{start, x - 2})
		row = append(row, slot...)
	}
	return append(row, pickerCell{"/ search ", dim})
}

func (p *ReactPicker) searchRows() [][]pickerCell {
	dim := tcell.StyleDefault.Foreground(ColorStatusText)
	accent := tcell.StyleDefault.Foreground(ColorComposerBar).Bold(true)
	rows := [][]pickerCell{{{" :", accent}, {p.query, tcell.StyleDefault}, {"█", accent}}}
	if len(p.results) == 0 {
		return append(rows, []pickerCell{{"   No matching emoji", dim}})
	}
	end := min(len(p.results), p.scroll+pickerMaxResults)
	for i := p.scroll; i < end; i++ {
		m := p.results[i]
		marker, markerStyle, codeStyle, style := "   ", accent, dim, tcell.StyleDefault
		if i == p.sel {
			markerStyle = tcell.StyleDefault.Background(ColorHighlight).Foreground(ColorOnHighlight)
			marker, codeStyle, style = " › ", markerStyle.Bold(true), markerStyle
		}
		if p.isOwn(m.Emoji) {
			style = style.Underline(true)
		}
		row := []pickerCell{{marker, markerStyle}, {m.Emoji, style}, {" :" + m.Shortcode + ":", codeStyle}}
		if m.Keyword != "" {
			// Why it matched, for searches by meaning: "awkward".
			row = append(row, pickerCell{"  " + m.Keyword, dim})
		}
		rows = append(rows, row)
	}
	return rows
}

// drawThinBox clears an area and frames it with single-line borders.
func drawThinBox(screen mauview.Screen, x, y, w, h int, title string) {
	border := tcell.StyleDefault.Foreground(ColorBorder)
	for row := y; row < y+h; row++ {
		widget.WriteLinePadded(screen, mauview.AlignLeft, "", x, row, w, tcell.StyleDefault)
	}
	for cx := x + 1; cx < x+w-1; cx++ {
		screen.SetContent(cx, y, '─', nil, border)
		screen.SetContent(cx, y+h-1, '─', nil, border)
	}
	for row := y + 1; row < y+h-1; row++ {
		screen.SetContent(x, row, '│', nil, border)
		screen.SetContent(x+w-1, row, '│', nil, border)
	}
	screen.SetContent(x, y, '╭', nil, border)
	screen.SetContent(x+w-1, y, '╮', nil, border)
	screen.SetContent(x, y+h-1, '╰', nil, border)
	screen.SetContent(x+w-1, y+h-1, '╯', nil, border)
	if title != "" && w > 4 {
		widget.WriteLine(screen, mauview.AlignLeft, title, x+2, y, w-4, tcell.StyleDefault.Foreground(ColorPillYellow))
	}
}

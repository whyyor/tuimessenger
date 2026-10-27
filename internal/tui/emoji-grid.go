package tui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"go.mau.fi/mauview"

	"go.mau.fi/gomuks/tui/lib/emoji"
	"go.mau.fi/gomuks/tui/widget"
)

// The browse grid: / with an empty query shows category tabs and emoji.
const (
	gridCols = 10
	gridRows = 6
	// gridInner is the box's inner width: a pad cell plus three per column.
	gridInner = 1 + gridCols*3 + 1
)

var categoryIcons = map[string]string{
	"Smileys & Emotion": "😀", "People & Body": "👋", "Animals & Nature": "🐶",
	"Food & Drink": "🍔", "Travel & Places": "🚗", "Activities": "⚽",
	"Objects": "💡", "Symbols": "🔣", "Flags": "🏁",
}

func (p *ReactPicker) gridMode() bool {
	return p.searching && p.query == ""
}

// tabIcons: recent first, then the categories.
func tabIcons() []string {
	icons := []string{"🕘"}
	for _, c := range emoji.Categories() {
		icon := categoryIcons[c.Name]
		if icon == "" && len(c.Emojis) > 0 {
			icon = c.Emojis[0].Emoji
		}
		icons = append(icons, icon)
	}
	return icons
}

func (p *ReactPicker) gridItems() []string {
	if p.cat <= 0 {
		return recentEmoji.Top(recentEmojiMax)
	}
	cats := emoji.Categories()
	if p.cat > len(cats) {
		return nil
	}
	items := make([]string, len(cats[p.cat-1].Emojis))
	for i, info := range cats[p.cat-1].Emojis {
		items[i] = info.Emoji
	}
	return items
}

func (p *ReactPicker) switchTab(delta int) {
	n := len(emoji.Categories()) + 1
	p.cat = ((p.cat+delta)%n + n) % n
	p.gridSel, p.gridScroll = 0, 0
}

// onGridKey handles movement in the grid; false lets typing fall through.
func (p *ReactPicker) onGridKey(event mauview.KeyEvent) bool {
	items := p.gridItems()
	n := len(items)
	switch event.Key() {
	case tcell.KeyTab:
		p.switchTab(1)
	case tcell.KeyBacktab:
		p.switchTab(-1)
	case tcell.KeyLeft:
		p.gridSel = max(p.gridSel-1, 0)
	case tcell.KeyRight:
		p.gridSel = min(p.gridSel+1, max(n-1, 0))
	case tcell.KeyUp:
		if p.gridSel >= gridCols {
			p.gridSel -= gridCols
		}
	case tcell.KeyDown:
		if p.gridSel+gridCols < n {
			p.gridSel += gridCols
		} else if p.gridSel/gridCols < (n-1)/gridCols {
			p.gridSel = n - 1
		}
	case tcell.KeyPgUp:
		p.gridSel = max(p.gridSel-gridCols*gridRows, p.gridSel%gridCols)
	case tcell.KeyPgDn:
		if n > 0 {
			p.gridSel = min(p.gridSel+gridCols*gridRows, n-1)
		}
	case tcell.KeyEnter:
		if p.gridSel < n {
			p.pick(items[p.gridSel])
		}
		return true
	default:
		return false
	}
	row := p.gridSel / gridCols
	if row < p.gridScroll {
		p.gridScroll = row
	} else if row >= p.gridScroll+gridRows {
		p.gridScroll = row - gridRows + 1
	}
	return true
}

// gridLayoutRows are the rows Draw frames: the grid itself is drawn on top.
func (p *ReactPicker) gridLayoutRows() [][]pickerCell {
	dim := tcell.StyleDefault.Foreground(ColorStatusText)
	accent := tcell.StyleDefault.Foreground(ColorComposerBar).Bold(true)
	rows := [][]pickerCell{{{" :", accent}, {"█", accent}, {" type to search · tab ↹ group", dim}}}
	for i := 0; i < gridRows+1; i++ {
		rows = append(rows, nil)
	}
	return append(rows, p.nameRow())
}

// nameRow teaches the name: ":grimacing:  awkward, cringe, eek".
func (p *ReactPicker) nameRow() []pickerCell {
	items := p.gridItems()
	if p.gridSel >= len(items) {
		return nil
	}
	info := emoji.Lookup(items[p.gridSel])
	if info == nil {
		return nil
	}
	row := []pickerCell{{" :" + info.Shortcode + ":", tcell.StyleDefault.Bold(true)}}
	if len(info.Keywords) > 0 {
		row = append(row, pickerCell{"  " + strings.Join(info.Keywords[:min(len(info.Keywords), 4)], ", "),
			tcell.StyleDefault.Foreground(ColorStatusText)})
	}
	return row
}

// drawEmojiCell writes one emoji in exactly two cells, in a way tcell (which
// sizes a cell by its first rune) and the terminal agree on.
func drawEmojiCell(screen mauview.Screen, x, y int, e string, style tcell.Style) {
	r := []rune(e)
	if len(r) == 0 {
		return
	}
	switch {
	case runewidth.RuneWidth(r[0]) == 2:
		// One cluster, joiners and modifiers kept: 🧑‍🚒 stays one picture.
		screen.SetContent(x, y, r[0], r[1:], style)
		screen.SetContent(x+1, y, r[0], nil, style)
	case len(r) >= 2 && isRegionalIndicator(r[0]) && isRegionalIndicator(r[1]):
		// Flags: 1+1 cells that the terminal joins into one 2-wide flag.
		screen.SetContent(x, y, r[0], nil, style)
		screen.SetContent(x+1, y, r[1], nil, style)
	default:
		// A narrow base plus U+FE0F: drawn bare, as the bubbles do.
		screen.SetContent(x, y, r[0], nil, style)
		screen.SetContent(x+1, y, ' ', nil, style)
	}
}

func isRegionalIndicator(r rune) bool {
	return r >= 0x1F1E6 && r <= 0x1F1FF
}

// slotX is where column i's emoji starts; tabs share the grid's columns.
func (p *ReactPicker) slotX(i int) int {
	return p.x + 2 + 3*i
}

// drawGrid overlays the tabs and the visible grid rows on the framed box.
func (p *ReactPicker) drawGrid(screen mauview.Screen) {
	hl := tcell.StyleDefault.Background(ColorHighlight).Foreground(ColorOnHighlight)
	pad := func(x, y int) {
		widget.WriteLinePadded(screen, mauview.AlignLeft, "", x-1, y, 4, hl)
	}
	tabsY := p.y + 2
	for i, icon := range tabIcons() {
		if i >= gridCols {
			break
		}
		style := tcell.StyleDefault
		if i == p.cat {
			pad(p.slotX(i), tabsY)
			style = hl
		}
		drawEmojiCell(screen, p.slotX(i), tabsY, icon, style)
	}
	items := p.gridItems()
	for row := 0; row < gridRows; row++ {
		for col := 0; col < gridCols; col++ {
			idx := (p.gridScroll+row)*gridCols + col
			if idx >= len(items) {
				return
			}
			style := tcell.StyleDefault
			if idx == p.gridSel {
				pad(p.slotX(col), tabsY+1+row)
				style = hl
			}
			drawEmojiCell(screen, p.slotX(col), tabsY+1+row, items[idx], style)
		}
	}
}

// onGridClick switches tabs or picks a grid emoji; true when handled.
func (p *ReactPicker) onGridClick(x, y int) bool {
	col := (x - (p.x + 1)) / 3
	if x < p.x+1 || col < 0 || col >= gridCols {
		return false
	}
	tabsY := p.y + 2
	switch {
	case y == tabsY && col < len(tabIcons()):
		p.switchTab(col - p.cat)
		return true
	case y > tabsY && y <= tabsY+gridRows:
		idx := (p.gridScroll+y-tabsY-1)*gridCols + col
		if items := p.gridItems(); idx < len(items) {
			p.pick(items[idx])
			return true
		}
	}
	return false
}

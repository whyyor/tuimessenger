package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"go.mau.fi/mauview"
	"maunium.net/go/mautrix/bridgev2"
	"rsc.io/qr"

	"github.com/whyyor/tuimessenger/internal/tui/widget"
)

// The Networks panel follows the palette (see the setup screen): borderless panel, accent pill, rows on one grid.
const (
	netPanelWidth = 72
	netTextX      = 5
	netLabelWidth = 12
	qrQuietZone   = 2 // modules of light border; phones need some contrast around the code
)

type netRowKind int

const (
	netRowText netRowKind = iota
	netRowChoice
	netRowField
	netRowQR
)

type netRow struct {
	kind     netRowKind
	text     string
	detail   string
	icon     string
	color    tcell.Color
	style    tcell.Style
	selected bool
	index    int
	qrLine   int
}

func (p *NetworksPanel) titleLocked() string {
	switch p.state {
	case netList:
		return " \uf1e6 Networks "
	case netKeys:
		return " Telegram API keys "
	default:
		return " " + networkName(p.network.Name) + " "
	}
}

func (p *NetworksPanel) hintsLocked() string {
	switch p.state {
	case netList:
		return "↑↓ choose  enter open  esc close"
	case netActions, netFlows:
		return "↑↓ choose  enter go  esc back"
	case netConfirmLogout:
		return "y log out  n keep"
	case netKeys:
		return "tab next  enter save  esc back"
	}
	if p.login != nil && len(p.fields) > 0 {
		hint := "tab next  enter continue  esc cancel"
		if len(p.fields[p.focus].options) > 0 {
			hint = "←→ choose  " + hint
		}
		return hint
	}
	return "esc cancel"
}

func (p *NetworksPanel) rowsLocked(width int) ([]netRow, *qr.Code) {
	dim := tcell.StyleDefault.Foreground(ColorStatusText)
	var rows []netRow
	blank := func() { rows = append(rows, netRow{kind: netRowText}) }
	text := func(s string, style tcell.Style) {
		for _, line := range wrapText(s, width-netTextX-2) {
			rows = append(rows, netRow{kind: netRowText, text: line, style: style})
		}
	}
	choice := func(i int, title, detail, icon string, color tcell.Color) {
		rows = append(rows, netRow{kind: netRowChoice, text: title, detail: detail, icon: icon, color: color, selected: i == p.selected})
	}
	var code *qr.Code
	switch p.state {
	case netList:
		for i, n := range p.networks {
			icon, color := BridgeIconColor(n.Name)
			status, _ := networkStatus(n)
			choice(i, networkName(n.Name), status, icon, color)
		}
		if len(p.networks) == 0 && !p.busy {
			text("No networks", dim)
		}
	case netActions:
		status, _ := networkStatus(p.network)
		text(status, dim)
		blank()
		choice(0, "Log in again", "e.g. after being logged out", "", 0)
		choice(1, "Log out", "", "", 0)
	case netConfirmLogout:
		text("Log out of "+networkName(p.network.Name)+"? You stop getting its messages here; its chats stay, and logging in again brings them back to life.", tcell.StyleDefault)
	case netFlows:
		text("How do you want to log in?", dim)
		blank()
		for i, f := range p.network.Flows {
			choice(i, f.Name, "", "", 0)
		}
		if p.selected < len(p.network.Flows) {
			blank()
			text(p.network.Flows[p.selected].Description, dim)
		}
	case netKeys:
		text("Telegram needs your own app keys (Telegram's rule for unofficial apps):", tcell.StyleDefault)
		text("1. Open my.telegram.org and log in with your phone number.", dim)
		text("2. API development tools → create an app (any name and short name).", dim)
		text("3. Copy api_id and api_hash here.", dim)
		blank()
		for i := range p.fields {
			rows = append(rows, netRow{kind: netRowField, index: i})
		}
	case netStep:
		if p.login != nil {
			step := p.login.Step
			if step.Instructions != "" {
				text(step.Instructions, dim)
				blank()
			}
			if dw := step.DisplayAndWaitParams; step.Type == bridgev2.LoginStepTypeDisplayAndWait && dw != nil {
				switch dw.Type {
				case bridgev2.LoginDisplayTypeQR:
					if c, err := qr.Encode(dw.Data, qr.L); err == nil {
						code = c
						for line := 0; line < (c.Size+2*qrQuietZone+1)/2; line++ {
							rows = append(rows, netRow{kind: netRowQR, qrLine: line})
						}
					} else {
						text("Couldn't draw the QR code: "+err.Error(), tcell.StyleDefault.Foreground(ColorMonokaiRed))
					}
				case bridgev2.LoginDisplayTypeCode, bridgev2.LoginDisplayTypeEmoji:
					rows = append(rows, netRow{kind: netRowText, text: "  " + dw.Data + "  ",
						style: tcell.StyleDefault.Background(ColorComposerBar).Foreground(ColorOnSetupPill).Bold(true)})
				}
				blank()
			}
			for i := range p.fields {
				rows = append(rows, netRow{kind: netRowField, index: i})
			}
			if len(p.fields) > 0 && p.fields[p.focus].hint != "" {
				blank()
				text(p.fields[p.focus].hint, dim)
			}
		}
	}
	if p.errText != "" {
		blank()
		text(p.errText, tcell.StyleDefault.Foreground(ColorMonokaiRed))
	} else if p.status != "" {
		blank()
		text(p.status, dim)
	}
	return rows, code
}

func (p *NetworksPanel) Draw(screen mauview.Screen) {
	p.mu.Lock()
	defer p.mu.Unlock()
	sw, sh := screen.Size()
	if sw < 30 || sh < 10 {
		return
	}
	w := min(sw-2, netPanelWidth)
	rows, code := p.rowsLocked(w)
	listH := min(len(rows), sh-5)
	h := listH + 5
	x, y := (sw-w)/2, max(0, (sh-h)/2)
	p.x, p.y, p.w, p.h = x, y, w, h

	panel := &panelScreen{Screen: screen, bg: ColorPanel}
	for py := y; py < y+h; py++ {
		for px := x; px < x+w; px++ {
			panel.SetContent(px, py, ' ', nil, tcell.StyleDefault)
		}
	}
	title := p.titleLocked()
	if tw := runewidth.StringWidth(title); tw <= w {
		widget.WriteLine(screen, mauview.AlignLeft, title, x+(w-tw)/2, y, tw,
			tcell.StyleDefault.Background(ColorSetupPill).Foreground(ColorOnSetupPill))
	}
	for i := 0; i < listH; i++ {
		p.drawRowLocked(panel, rows[i], code, x, y+2+i, w)
	}
	widget.WriteLine(panel, mauview.AlignRight, p.hintsLocked(), x+1, y+2+listH+1, w-3,
		tcell.StyleDefault.Foreground(ColorStatusText))
	toasts.Draw(screen)
}

func (p *NetworksPanel) drawRowLocked(screen mauview.Screen, row netRow, code *qr.Code, x, y, w int) {
	dim := tcell.StyleDefault.Foreground(ColorStatusText)
	selected := row.selected || (row.kind == netRowField && row.index == p.focus)
	base := tcell.StyleDefault
	if selected {
		base, dim = base.Background(ColorBarBackground), dim.Background(ColorBarBackground)
		widget.WriteLinePadded(screen, mauview.AlignLeft, "", x+1, y, w-2, base)
		screen.SetContent(x+1, y, '▌', nil, base.Foreground(ColorSetupPill))
	}
	switch row.kind {
	case netRowText:
		widget.WriteLine(screen, mauview.AlignLeft, row.text, x+netTextX, y, w-netTextX-2, row.style)
	case netRowChoice:
		titleX := x + netTextX
		if row.icon != "" {
			color := row.color
			if selected {
				color = vivid(color)
			}
			widget.WriteLine(screen, mauview.AlignLeft, row.icon, x+3, y, 2, base.Foreground(color).Bold(false))
		}
		detailW := min(runewidth.StringWidth(row.detail), w/2)
		if detailW > 0 {
			widget.WriteLine(screen, mauview.AlignLeft, runewidth.Truncate(row.detail, detailW, "…"), x+w-detailW-2, y, detailW, dim)
		}
		widget.WriteLine(screen, mauview.AlignLeft, row.text, titleX, y, w-netTextX-detailW-4, base.Bold(selected))
	case netRowField:
		if row.index >= len(p.fields) {
			return
		}
		f := p.fields[row.index]
		labelW := p.labelWidthLocked()
		widget.WriteLine(screen, mauview.AlignLeft, f.label, x+netTextX, y, labelW, base.Bold(selected))
		inputX := x + netTextX + labelW + 1
		if len(f.options) > 0 {
			widget.WriteLine(screen, mauview.AlignLeft, "‹ "+f.options[f.option]+" ›", inputX, y, w-(inputX-x)-2, base)
			return
		}
		bg := tcell.ColorDefault
		if selected {
			bg = ColorBarBackground
		}
		f.input.SetBackgroundColor(bg)
		f.input.Draw(mauview.NewProxyScreen(screen, inputX, y, w-(inputX-x)-2, 1))
	case netRowQR:
		drawQRLine(screen, code, row.qrLine, x+(w-(code.Size+2*qrQuietZone))/2, y)
	}
}

// labelWidthLocked fits the form's longest label, between netLabelWidth and twice that.
func (p *NetworksPanel) labelWidthLocked() int {
	width := netLabelWidth
	for _, f := range p.fields {
		width = max(width, runewidth.StringWidth(f.label)+1)
	}
	return min(width, 2*netLabelWidth)
}

// drawQRLine draws two module rows of the code as one terminal row of half blocks, dark on light.
func drawQRLine(screen mauview.Screen, code *qr.Code, line, x, y int) {
	light, dark := tcell.NewHexColor(0xffffff), tcell.NewHexColor(0x000000) // exact, not the theme's "white"
	black := func(mx, my int) bool {
		mx, my = mx-qrQuietZone, my-qrQuietZone
		return mx >= 0 && my >= 0 && mx < code.Size && my < code.Size && code.Black(mx, my)
	}
	size := code.Size + 2*qrQuietZone
	for mx := 0; mx < size; mx++ {
		top, bottom := light, light
		if black(mx, line*2) {
			top = dark
		}
		if line*2+1 >= size {
			bottom = tcell.ColorDefault
		} else if black(mx, line*2+1) {
			bottom = dark
		}
		screen.SetContent(x+mx, y, '▀', nil, tcell.StyleDefault.Foreground(top).Background(bottom))
	}
}

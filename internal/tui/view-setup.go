package tui

import (
	"context"
	"strings"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"go.mau.fi/mauview"

	"github.com/whyyor/tuimessenger/internal/hicli/jsoncmd"
	"github.com/whyyor/tuimessenger/internal/tui/debug"
	"github.com/whyyor/tuimessenger/internal/tui/widget"
)

type setupMode int

const (
	setupComplete setupMode = iota
	setupSelfHosted
	setupHostBackend
)

type setupModeInfo struct {
	icon, name, hint string
	available        bool
}

// setupIcon is the terminal glyph the palette's pill uses too.
const setupIcon = "\uf120"

var setupModes = []setupModeInfo{
	setupComplete:    {"\uf108", "Complete setup", "backend and TUI on this machine", true},
	setupSelfHosted:  {"\uf0c2", "TUI with self-hosted backend", "connect to your backend, e.g. a Pi", true},
	setupHostBackend: {"\uf233", "Host backend", "only the backend here · coming soon", false},
}

// SetupDefaults are this machine's backend address and username; Complete setup uses them as they are.
type SetupDefaults struct {
	Server   string
	Username string
}

const (
	setupPanelWidth = 72 // the palette's width
	setupLabelWidth = 10
)

type setupField struct {
	label string
	input *mauview.InputField
}

type setupRow struct {
	kind  setupRowKind
	text  string
	style tcell.Style
	index int // the mode or field the row shows
}

// SetupView is the first screen when the TUI isn't logged in: pick how tuimessenger runs, then log in.
type SetupView struct {
	defaults SetupDefaults

	choosing bool
	selected setupMode
	mode     setupMode
	fields   []setupField
	focus    int

	mu      sync.Mutex
	busy    bool
	status  string
	errText string

	check      func(ctx context.Context, base string) error
	login      func(ctx context.Context, base, username, password string) error
	onLoggedIn func()
	render     func()
	quit       func()
}

func (ui *App) NewSetupView() *SetupView {
	view := newSetupView(ui.Setup)
	view.login = func(ctx context.Context, base, username, password string) error {
		return ui.gmx.API.Login(ctx, &jsoncmd.LoginParams{HomeserverURL: base, Username: username, Password: password})
	}
	view.onLoggedIn = ui.leaveSetup
	view.render = ui.Render
	view.quit = ui.Stop
	return view
}

func newSetupView(defaults SetupDefaults) *SetupView {
	return &SetupView{
		defaults: defaults,
		choosing: true,
		check:    checkBackend,
		render:   func() {},
		quit:     func() {},
	}
}

// nextMode moves the selection by delta, skipping modes that aren't available yet.
func nextMode(from setupMode, delta int) setupMode {
	n := len(setupModes)
	for i, at := 0, int(from); i < n; i++ {
		at = ((at+delta)%n + n) % n
		if setupModes[at].available {
			return setupMode(at)
		}
	}
	return from
}

func newSetupInput(text, placeholder string, password bool) *mauview.InputField {
	input := mauview.NewInputField().SetText(text).SetPlaceholder(placeholder).
		SetTextColor(tcell.ColorDefault).SetPlaceholderTextColor(ColorStatusText)
	if password {
		input.SetMaskCharacter('*')
	}
	return input
}

func (view *SetupView) choose(mode setupMode) {
	if !setupModes[mode].available {
		return
	}
	view.mode, view.choosing, view.focus = mode, false, 0
	password := setupField{"Password", newSetupInput("", "your tuimessenger password", true)}
	if mode == setupComplete {
		view.fields = []setupField{password}
	} else {
		view.fields = []setupField{
			{"Address", newSetupInput("", "http://pi:8008", false)},
			{"Username", newSetupInput(view.defaults.Username, "me", false)},
			password,
		}
	}
	view.fields[0].input.Focus()
	view.setResult("", "")
}

func (view *SetupView) back() {
	view.choosing, view.fields = true, nil
	view.setResult("", "")
}

func (view *SetupView) moveFocus(delta int) {
	view.fields[view.focus].input.Blur()
	view.focus = (view.focus + delta + len(view.fields)) % len(view.fields)
	view.fields[view.focus].input.Focus()
}

func (view *SetupView) isBusy() bool {
	view.mu.Lock()
	defer view.mu.Unlock()
	return view.busy
}

func (view *SetupView) setResult(status, errText string) {
	view.mu.Lock()
	view.status, view.errText = status, errText
	view.mu.Unlock()
}

// submit reads the form and, when it's complete, checks the backend and logs in in the background.
func (view *SetupView) submit() {
	if view.isBusy() {
		return
	}
	base, username := view.defaults.Server, view.defaults.Username
	password := view.fields[len(view.fields)-1].input.GetText()
	if view.mode == setupSelfHosted {
		var err error
		if base, err = normalizeBackendURL(view.fields[0].input.GetText()); err != nil {
			view.fail(err.Error())
			return
		}
		username = strings.TrimSpace(view.fields[1].input.GetText())
		if username == "" {
			view.fail("Enter your username.")
			return
		}
	}
	if password == "" {
		view.fail("Enter your password.")
		return
	}
	view.mu.Lock()
	view.busy, view.status, view.errText = true, "Checking the backend at "+base+"…", ""
	view.mu.Unlock()
	go view.run(view.mode, base, username, password)
}

func (view *SetupView) run(mode setupMode, base, username, password string) {
	defer debug.Recover()
	ctx := context.Background()
	err := view.check(ctx, base)
	if err == nil {
		view.setResult("Logging in…", "")
		view.render()
		err = view.login(ctx, base, username, password)
	}
	if err != nil {
		debug.Print("Setup failed:", err)
		text := setupErrorText(mode, base, err)
		toasts.Show(toastError, text)
		view.finish("", text)
		return
	}
	view.onLoggedIn()
	view.finish("Logged in. Loading your chats…", "")
}

// finish ends a login attempt; the result and busy change together so Enter can't start another in between.
func (view *SetupView) finish(status, errText string) {
	view.mu.Lock()
	view.busy, view.status, view.errText = false, status, errText
	view.mu.Unlock()
	view.render()
}

func (view *SetupView) errorText() string {
	view.mu.Lock()
	defer view.mu.Unlock()
	return view.errText
}

// fail shows an error in the form and as a toast, so it's seen even when the form is scrolled out of view.
func (view *SetupView) fail(text string) {
	view.setResult("", text)
	toasts.Show(toastError, text)
	view.render()
}

func (view *SetupView) OnKeyEvent(event mauview.KeyEvent) bool {
	if event.Key() == tcell.KeyCtrlC {
		view.quit()
		return true
	}
	if view.choosing {
		return view.chooserKey(event)
	}
	switch event.Key() {
	case tcell.KeyEscape:
		if !view.isBusy() {
			view.back()
		}
	case tcell.KeyEnter:
		if view.focus < len(view.fields)-1 {
			view.moveFocus(1)
		} else {
			view.submit()
		}
	case tcell.KeyTab, tcell.KeyDown:
		view.moveFocus(1)
	case tcell.KeyBacktab, tcell.KeyUp:
		view.moveFocus(-1)
	default:
		if view.isBusy() {
			return true
		}
		return view.fields[view.focus].input.OnKeyEvent(event)
	}
	return true
}

func (view *SetupView) chooserKey(event mauview.KeyEvent) bool {
	switch event.Key() {
	case tcell.KeyUp, tcell.KeyBacktab:
		view.selected = nextMode(view.selected, -1)
	case tcell.KeyDown, tcell.KeyTab:
		view.selected = nextMode(view.selected, 1)
	case tcell.KeyEnter:
		view.choose(view.selected)
	case tcell.KeyRune:
		switch r := event.Rune(); {
		case r == 'k':
			view.selected = nextMode(view.selected, -1)
		case r == 'j':
			view.selected = nextMode(view.selected, 1)
		case r == 'q':
			view.quit()
		case r >= '1' && r < '1'+rune(len(setupModes)):
			view.choose(setupMode(r - '1'))
		}
	default:
		return false
	}
	return true
}

func (view *SetupView) OnPasteEvent(event mauview.PasteEvent) bool {
	if view.choosing || view.isBusy() {
		return false
	}
	return view.fields[view.focus].input.OnPasteEvent(event)
}

func (view *SetupView) OnMouseEvent(mauview.MouseEvent) bool { return false }

// The setup panel follows the palette and Ctrl+k: a borderless panel, a centred pill title in the accent colour,
// rows on one grid (▌ col 1, icon col 3, text col 5), and dim key hints in the bottom row.
var (
	ColorSetupPill   = ColorComposerBar
	ColorOnSetupPill = tcell.NewHexColor(0x000000)
)

const (
	setupTextX  = 5 // titles, labels and text, like the palette's titles
	setupInputX = setupTextX + setupLabelWidth
)

type setupRowKind int

const (
	rowBlank setupRowKind = iota
	rowText
	rowMode
	rowField
)

// rows lays out the panel's content between the title pill and the hint row.
func (view *SetupView) rows(width int) []setupRow {
	dim := tcell.StyleDefault.Foreground(ColorStatusText)
	var rows []setupRow
	blank := func() { rows = append(rows, setupRow{kind: rowBlank}) }
	text := func(s string, style tcell.Style) {
		for _, line := range wrapText(s, width-setupTextX-2) {
			rows = append(rows, setupRow{kind: rowText, text: line, style: style})
		}
	}
	if view.choosing {
		for i := range setupModes {
			rows = append(rows, setupRow{kind: rowMode, index: i})
		}
		return rows
	}
	if view.mode == setupComplete {
		text("Log in to the backend on this machine ("+view.defaults.Server+") as "+view.defaults.Username+".", dim)
	} else {
		text("Your backend's address, and the username and password you set on it.", dim)
	}
	blank()
	for i := range view.fields {
		rows = append(rows, setupRow{kind: rowField, index: i})
	}
	view.mu.Lock()
	status, errText := view.status, view.errText
	view.mu.Unlock()
	if errText != "" {
		blank()
		text(errText, tcell.StyleDefault.Foreground(ColorMonokaiRed))
	} else if status != "" {
		blank()
		text(status, dim)
	}
	return rows
}

func (view *SetupView) hints() string {
	if view.choosing {
		return "↑↓ choose  enter continue  ctrl+c quit"
	}
	return "tab next field  enter log in  esc back"
}

func (view *SetupView) Draw(screen mauview.Screen) {
	sw, sh := screen.Size()
	if sw < 24 || sh < 9 {
		return
	}
	w := min(sw-4, setupPanelWidth)
	rows := view.rows(w)
	listH := min(len(rows), sh-5)
	h := listH + 5 // pill, blank, rows, blank, hints, padding
	x, y := (sw-w)/2, max(0, (sh-h)/2)

	panel := &panelScreen{Screen: screen, bg: ColorPanel}
	for py := y; py < y+h; py++ {
		for px := x; px < x+w; px++ {
			panel.SetContent(px, py, ' ', nil, tcell.StyleDefault)
		}
	}
	title := " " + setupIcon + " Set up tuimessenger "
	if !view.choosing {
		title = " " + setupModes[view.mode].icon + " " + setupModes[view.mode].name + " "
	}
	if tw := runewidth.StringWidth(title); tw <= w {
		widget.WriteLine(screen, mauview.AlignLeft, title, x+(w-tw)/2, y,
			tw, tcell.StyleDefault.Background(ColorSetupPill).Foreground(ColorOnSetupPill))
	}
	for i := 0; i < listH; i++ {
		view.drawRow(panel, rows[i], x, y+2+i, w)
	}
	widget.WriteLine(panel, mauview.AlignRight, view.hints(), x+1, y+2+listH+1, w-3,
		tcell.StyleDefault.Foreground(ColorStatusText))
	toasts.Draw(screen)
}

// drawRow draws one row on the palette's grid; the selected mode or focused field gets the selection bar and ▌.
func (view *SetupView) drawRow(screen mauview.Screen, row setupRow, x, y, w int) {
	dim := tcell.StyleDefault.Foreground(ColorStatusText)
	selected := (row.kind == rowMode && setupMode(row.index) == view.selected) ||
		(row.kind == rowField && row.index == view.focus)
	base := tcell.StyleDefault
	if selected {
		base, dim = base.Background(ColorBarBackground), dim.Background(ColorBarBackground)
		widget.WriteLinePadded(screen, mauview.AlignLeft, "", x+1, y, w-2, base)
		screen.SetContent(x+1, y, '▌', nil, base.Foreground(ColorSetupPill))
	}
	switch row.kind {
	case rowText:
		widget.WriteLine(screen, mauview.AlignLeft, row.text, x+setupTextX, y, w-setupTextX-2, row.style)
	case rowMode:
		mode := setupModes[row.index]
		titleStyle, iconColor := base.Bold(selected), ColorSetupPill
		if !mode.available {
			titleStyle, iconColor = dim, ColorStatusText
		} else if selected {
			iconColor = vivid(iconColor)
		}
		widget.WriteLine(screen, mauview.AlignLeft, mode.icon, x+3, y, 2, base.Foreground(iconColor).Bold(false))
		detailW := runewidth.StringWidth(mode.hint)
		widget.WriteLine(screen, mauview.AlignLeft, mode.hint, x+w-detailW-2, y, detailW, dim)
		widget.WriteLine(screen, mauview.AlignLeft, mode.name, x+setupTextX, y, w-setupTextX-detailW-4, titleStyle)
	case rowField:
		field := view.fields[row.index]
		widget.WriteLine(screen, mauview.AlignLeft, field.label, x+setupTextX, y, setupLabelWidth, base.Bold(selected))
		bg := tcell.ColorDefault // the panel shows through
		if selected {
			bg = ColorBarBackground
		}
		field.input.SetBackgroundColor(bg)
		field.input.Draw(mauview.NewProxyScreen(screen, x+setupInputX, y, w-setupInputX-2, 1))
	}
}

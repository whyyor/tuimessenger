package tui

import (
	"context"
	"errors"
	"fmt"
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

// LocalBackend runs the backend inside this process, for Complete setup when none is running.
type LocalBackend interface {
	// Start runs the backend if it isn't running yet and reports whether it has a password (clients can log in).
	Start(ctx context.Context) (hasPassword bool, err error)
	Running() bool
	SetPassword(ctx context.Context, password string) error
}

// SetupView is the first screen when the TUI isn't logged in: pick how tuimessenger runs, then log in.
type SetupView struct {
	defaults SetupDefaults
	local    LocalBackend // nil: Complete setup only connects to a backend that's already running

	// mu guards the state below: Draw and keys run on the UI thread, backend checks and logins in goroutines.
	// Callbacks that draw (render, toasts) must run without it, as drawing takes it.
	mu          sync.Mutex
	choosing    bool
	selected    setupMode
	mode        setupMode
	newPassword bool // Complete setup on a backend without a password: choose one first
	fields      []setupField
	focus       int
	busy        bool
	status      string
	errText     string

	check      func(ctx context.Context, base string) error
	login      func(ctx context.Context, base, username, password string) error
	onLoggedIn func(mode setupMode)
	render     func()
	quit       func()
}

func (ui *App) NewSetupView() *SetupView {
	view := newSetupView(ui.Setup)
	view.local = ui.Local
	view.login = func(ctx context.Context, base, username, password string) error {
		return ui.gmx.API.Login(ctx, &jsoncmd.LoginParams{HomeserverURL: base, Username: username, Password: password})
	}
	view.onLoggedIn = ui.finishSetup
	view.render = ui.Render
	view.quit = ui.Stop
	return view
}

func newSetupView(defaults SetupDefaults) *SetupView {
	return &SetupView{
		defaults:   defaults,
		choosing:   true,
		check:      checkBackend,
		onLoggedIn: func(setupMode) {},
		render:     func() {},
		quit:       func() {},
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

// effects are what a locked step asks for once the lock is released.
type effects struct {
	toast string
	quit  bool
}

func (view *SetupView) apply(fx effects) {
	if fx.toast != "" {
		toasts.Show(toastError, fx.toast)
	}
	if fx.quit {
		view.quit()
	}
	view.render()
}

func (view *SetupView) setFieldsLocked(fields ...setupField) {
	view.fields, view.focus = fields, 0
	if len(fields) > 0 {
		fields[0].input.Focus()
	}
}

func (view *SetupView) passwordFieldsLocked() {
	if view.newPassword {
		view.setFieldsLocked(
			setupField{"Password", newSetupInput("", "at least 8 characters", true)},
			setupField{"Again", newSetupInput("", "the same password again", true)},
		)
	} else {
		view.setFieldsLocked(setupField{"Password", newSetupInput("", "your tuimessenger password", true)})
	}
}

func (view *SetupView) chooseLocked(mode setupMode) {
	if !setupModes[mode].available {
		return
	}
	view.mode, view.choosing, view.newPassword = mode, false, false
	view.status, view.errText = "", ""
	if mode == setupSelfHosted {
		view.setFieldsLocked(
			setupField{"Address", newSetupInput("", "http://pi:8008", false)},
			setupField{"Username", newSetupInput(view.defaults.Username, "me", false)},
			setupField{"Password", newSetupInput("", "your tuimessenger password", true)},
		)
		return
	}
	// Complete setup: find (or start) the backend first; whether it has a password decides the form.
	view.setFieldsLocked()
	view.busy, view.status = true, "Looking for the backend on this machine…"
	go view.prepare()
}

// ensureBackend makes sure this machine's backend answers, starting it in this process if need be.
func (view *SetupView) ensureBackend(ctx context.Context) (hasPassword bool, err error) {
	if view.local != nil && view.local.Running() {
		return view.local.Start(ctx)
	}
	err = view.check(ctx, view.defaults.Server)
	if err == nil {
		return true, nil // a backend running on its own was given a password when it first started
	} else if !errors.Is(err, errBackendUnreachable) || view.local == nil {
		return false, err
	}
	view.mu.Lock()
	view.status = "Starting the backend on this machine…"
	view.mu.Unlock()
	view.render()
	if hasPassword, err = view.local.Start(ctx); err != nil {
		return false, fmt.Errorf("%w: %w", errLocalStart, err)
	}
	return hasPassword, nil
}

func (view *SetupView) prepare() {
	defer debug.Recover()
	hasPassword, err := view.ensureBackend(context.Background())
	view.mu.Lock()
	view.busy, view.status, view.errText = false, "", ""
	view.newPassword = err == nil && !hasPassword
	view.passwordFieldsLocked()
	var fx effects
	if err != nil {
		view.errText = setupErrorText(setupComplete, view.defaults.Server, err)
		fx.toast = view.errText
	}
	view.mu.Unlock()
	view.apply(fx)
}

func (view *SetupView) backLocked() {
	view.choosing, view.newPassword = true, false
	view.setFieldsLocked()
	view.status, view.errText = "", ""
}

func (view *SetupView) moveFocusLocked(delta int) {
	if len(view.fields) == 0 {
		return
	}
	view.fields[view.focus].input.Blur()
	view.focus = (view.focus + delta + len(view.fields)) % len(view.fields)
	view.fields[view.focus].input.Focus()
}

func (view *SetupView) isBusy() bool {
	view.mu.Lock()
	defer view.mu.Unlock()
	return view.busy
}

func (view *SetupView) errorText() string {
	view.mu.Lock()
	defer view.mu.Unlock()
	return view.errText
}

func (view *SetupView) failLocked(text string) effects {
	view.status, view.errText = "", text
	return effects{toast: text}
}

// submitLocked reads the form and, when it's complete, checks the backend and logs in in the background.
func (view *SetupView) submitLocked() effects {
	if view.busy || len(view.fields) == 0 {
		return effects{}
	}
	base, username := view.defaults.Server, view.defaults.Username
	password := view.fields[0].input.GetText()
	switch {
	case view.mode == setupSelfHosted:
		var err error
		if base, err = normalizeBackendURL(view.fields[0].input.GetText()); err != nil {
			return view.failLocked(err.Error())
		}
		if username = strings.TrimSpace(view.fields[1].input.GetText()); username == "" {
			return view.failLocked("Enter your username.")
		}
		password = view.fields[2].input.GetText()
	case view.newPassword:
		if len(password) < minPasswordLength {
			return view.failLocked(fmt.Sprintf("The password needs at least %d characters.", minPasswordLength))
		} else if password != view.fields[1].input.GetText() {
			return view.failLocked("The two passwords don't match.")
		}
	}
	if password == "" {
		return view.failLocked("Enter your password.")
	}
	view.busy, view.status, view.errText = true, "Checking the backend at "+base+"…", ""
	go view.run(view.mode, view.newPassword, base, username, password)
	return effects{}
}

// minPasswordLength matches what the backend accepts.
const minPasswordLength = 8

func (view *SetupView) run(mode setupMode, choosePassword bool, base, username, password string) {
	defer debug.Recover()
	ctx := context.Background()
	var err error
	if mode == setupComplete {
		var hasPassword bool
		if hasPassword, err = view.ensureBackend(ctx); err == nil && !hasPassword && !choosePassword {
			// The backend only just started here and has no password yet: ask for a new one instead.
			view.mu.Lock()
			view.busy, view.status, view.newPassword = false, "", true
			view.passwordFieldsLocked()
			view.mu.Unlock()
			view.render()
			return
		} else if err == nil && choosePassword {
			err = view.local.SetPassword(ctx, password)
		}
	} else {
		err = view.check(ctx, base)
	}
	if err == nil {
		view.mu.Lock()
		view.status = "Logging in…"
		view.mu.Unlock()
		view.render()
		err = view.login(ctx, base, username, password)
	}
	var fx effects
	view.mu.Lock()
	if err != nil {
		debug.Print("Setup failed:", err)
		fx = view.failLocked(setupErrorText(mode, base, err))
		view.busy = false
		view.mu.Unlock()
		view.apply(fx)
		return
	}
	view.mu.Unlock()
	view.onLoggedIn(mode)
	view.mu.Lock()
	view.busy, view.status, view.errText = false, "Logged in. Loading your chats…", ""
	view.mu.Unlock()
	view.render()
}

func (view *SetupView) OnKeyEvent(event mauview.KeyEvent) bool {
	if event.Key() == tcell.KeyCtrlC {
		view.quit()
		return true
	}
	view.mu.Lock()
	handled, fx := view.keyLocked(event)
	view.mu.Unlock()
	if fx != (effects{}) {
		view.apply(fx)
	}
	return handled
}

func (view *SetupView) keyLocked(event mauview.KeyEvent) (bool, effects) {
	if view.choosing {
		return view.chooserKeyLocked(event)
	}
	switch event.Key() {
	case tcell.KeyEscape:
		if !view.busy {
			view.backLocked()
		}
	case tcell.KeyEnter:
		if view.focus < len(view.fields)-1 {
			view.moveFocusLocked(1)
		} else {
			return true, view.submitLocked()
		}
	case tcell.KeyTab, tcell.KeyDown:
		view.moveFocusLocked(1)
	case tcell.KeyBacktab, tcell.KeyUp:
		view.moveFocusLocked(-1)
	default:
		if view.busy || len(view.fields) == 0 {
			return true, effects{}
		}
		return view.fields[view.focus].input.OnKeyEvent(event), effects{}
	}
	return true, effects{}
}

func (view *SetupView) chooserKeyLocked(event mauview.KeyEvent) (bool, effects) {
	switch event.Key() {
	case tcell.KeyUp, tcell.KeyBacktab:
		view.selected = nextMode(view.selected, -1)
	case tcell.KeyDown, tcell.KeyTab:
		view.selected = nextMode(view.selected, 1)
	case tcell.KeyEnter:
		view.chooseLocked(view.selected)
	case tcell.KeyRune:
		switch r := event.Rune(); {
		case r == 'k':
			view.selected = nextMode(view.selected, -1)
		case r == 'j':
			view.selected = nextMode(view.selected, 1)
		case r == 'q':
			return true, effects{quit: true}
		case r >= '1' && r < '1'+rune(len(setupModes)):
			view.chooseLocked(setupMode(r - '1'))
		}
	default:
		return false, effects{}
	}
	return true, effects{}
}

func (view *SetupView) OnPasteEvent(event mauview.PasteEvent) bool {
	view.mu.Lock()
	defer view.mu.Unlock()
	if view.choosing || view.busy || len(view.fields) == 0 {
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
	if view.mode == setupComplete && view.newPassword {
		text("First run: choose the password you'll log in with, here and from your other devices.", dim)
	} else if view.mode == setupComplete {
		text("Log in to the backend on this machine ("+view.defaults.Server+") as "+view.defaults.Username+".", dim)
	} else {
		text("Your backend's address, and the username and password you set on it.", dim)
	}
	blank()
	for i := range view.fields {
		rows = append(rows, setupRow{kind: rowField, index: i})
	}
	status, errText := view.status, view.errText
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
	view.mu.Lock()
	defer view.mu.Unlock()
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

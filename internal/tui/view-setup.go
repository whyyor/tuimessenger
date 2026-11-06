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
)

type setupMode int

const (
	setupComplete setupMode = iota
	setupSelfHosted
	setupHostBackend
)

type setupModeInfo struct {
	name, hint string
	available  bool
}

var setupModes = []setupModeInfo{
	setupComplete:    {"Complete setup", "backend and TUI on this machine", true},
	setupSelfHosted:  {"TUI with self-hosted backend", "connect to your backend elsewhere (e.g. a Pi)", true},
	setupHostBackend: {"Host backend", "run only the backend here, for other devices (coming soon)", false},
}

// SetupDefaults are this machine's backend address and username; Complete setup uses them as they are.
type SetupDefaults struct {
	Server   string
	Username string
}

const (
	setupBoxWidth   = 68
	setupLabelWidth = 10
)

type setupField struct {
	label string
	input *mauview.InputField
}

type setupRow struct {
	text  string
	style tcell.Style
	field int // index into fields, or -1
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
		SetTextColor(tcell.ColorWhite).SetBackgroundColor(ColorBarBackground).SetPlaceholderTextColor(ColorStatusText)
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

// rows lays out the box's content for an inner width.
func (view *SetupView) rows(width int) []setupRow {
	plain := tcell.StyleDefault
	dim := plain.Foreground(ColorStatusText)
	var rows []setupRow
	add := func(text string, style tcell.Style) { rows = append(rows, setupRow{mauview.Escape(text), style, -1}) }
	wrapped := func(text string, style tcell.Style) {
		for _, line := range wrapText(text, width-4) {
			add(line, style)
		}
	}
	add("", plain)
	if view.choosing {
		for i, mode := range setupModes {
			marker, style := "  ", plain
			if setupMode(i) == view.selected {
				marker, style = "▸ ", plain.Bold(true).Foreground(ColorSelectionBackground)
			}
			if !mode.available {
				style = dim
			}
			add(marker+mode.name, style)
			add("    "+mode.hint, dim)
			add("", plain)
		}
		add("↑/↓ choose · Enter continue · Ctrl+C quit", dim)
		return rows
	}
	if view.mode == setupComplete {
		wrapped("Log in to the backend on this machine ("+view.defaults.Server+") as "+view.defaults.Username+".", dim)
	} else {
		wrapped("Your backend's address, and the username and password you set on it.", dim)
	}
	add("", plain)
	for i := range view.fields {
		rows = append(rows, setupRow{field: i})
		add("", plain)
	}
	view.mu.Lock()
	status, errText := view.status, view.errText
	view.mu.Unlock()
	if errText != "" {
		wrapped(errText, plain.Foreground(ColorMonokaiRed))
		add("", plain)
	} else if status != "" {
		wrapped(status, dim)
		add("", plain)
	}
	add("Enter log in · Tab next field · Esc back · Ctrl+C quit", dim)
	return rows
}

func (view *SetupView) Draw(screen mauview.Screen) {
	sw, sh := screen.Size()
	width := min(setupBoxWidth, sw)
	rows := view.rows(width - 2)
	height := min(len(rows)+2, sh)
	x, y := (sw-width)/2, (sh-height)/2
	title := "Set up tuimessenger"
	if !view.choosing {
		title = setupModes[view.mode].name
	}
	drawSetupBox(screen, x, y, width, height, title)
	for i, row := range rows {
		if i >= height-2 {
			break
		}
		line := mauview.NewProxyScreen(screen, x+2, y+1+i, width-4, 1)
		if row.field < 0 {
			mauview.PrintWithStyle(line, row.text, 0, 0, width-4, mauview.AlignLeft, row.style)
			continue
		}
		field := view.fields[row.field]
		labelStyle := tcell.StyleDefault
		if row.field == view.focus {
			labelStyle = labelStyle.Bold(true)
		}
		mauview.PrintWithStyle(line, field.label, 0, 0, setupLabelWidth, mauview.AlignLeft, labelStyle)
		field.input.Draw(mauview.NewProxyScreen(line, setupLabelWidth, 0, width-4-setupLabelWidth, 1))
	}
	toasts.Draw(screen)
}

// drawSetupBox draws a rounded border with a title, the same look as the other boxes.
func drawSetupBox(screen mauview.Screen, x, y, width, height int, title string) {
	if width < 2 || height < 2 {
		return
	}
	border := tcell.StyleDefault.Foreground(ColorBorder)
	for i := x + 1; i < x+width-1; i++ {
		screen.SetContent(i, y, '─', nil, border)
		screen.SetContent(i, y+height-1, '─', nil, border)
	}
	for j := y + 1; j < y+height-1; j++ {
		screen.SetContent(x, j, '│', nil, border)
		screen.SetContent(x+width-1, j, '│', nil, border)
	}
	screen.SetContent(x, y, '╭', nil, border)
	screen.SetContent(x+width-1, y, '╮', nil, border)
	screen.SetContent(x, y+height-1, '╰', nil, border)
	screen.SetContent(x+width-1, y+height-1, '╯', nil, border)
	title = " " + mauview.Escape(runewidth.Truncate(title, width-6, "…")) + " "
	mauview.PrintWithStyle(screen, title, x+2, y, width-4, mauview.AlignLeft, tcell.StyleDefault.Bold(true))
}

package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/mauview"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"

	"github.com/whyyor/tuimessenger/internal/login"
	"github.com/whyyor/tuimessenger/internal/rpc/client"
	"github.com/whyyor/tuimessenger/internal/tui/debug"
)

// netState is the screen the Networks panel shows.
type netState int

const (
	netList          netState = iota // every network and its account
	netActions                       // log in again / log out, for a network that has an account
	netConfirmLogout                 // "log out of …?"
	netFlows                         // pick a way to log in
	netStep                          // a step of a login in progress
	netKeys                          // Telegram's app keys
)

// netRequestTimeout caps a request that isn't waiting on you (starting a login, a form, logging out).
const netRequestTimeout = 2 * time.Minute

// loginField is one input of a form: a text field, or a choice cycled with ←/→ when options is set.
type loginField struct {
	id, label, hint string
	input           *mauview.InputField
	options         []string
	option          int
}

func (f *loginField) value() string {
	if len(f.options) > 0 {
		return f.options[f.option]
	}
	return f.input.GetText()
}

// NetworksPanel lists the chat networks and logs them in and out, step by step (Ctrl+p → Networks).
type NetworksPanel struct {
	parent *MainView
	api    client.NetworkLogins
	redraw func()
	hide   func()

	// mu guards everything below: drawing and keys run on the UI thread, requests in goroutines.
	mu       sync.Mutex
	state    netState
	networks []client.Network
	selected int
	network  client.Network
	login    *client.LoginStep
	fields   []loginField
	focus    int
	busy     bool
	status   string
	errText  string
	browser  bool // a browser window is open for a cookie step
	gen      int  // bumped when a login is left, so its pending answers are ignored
	cancel   context.CancelFunc
	closed   bool

	x, y, w, h int // last drawn geometry, for clicks outside
}

// OpenNetworks shows the Networks panel, if the backend can log in to networks.
func (view *MainView) OpenNetworks() {
	api, ok := view.matrix.Backend.(client.NetworkLogins)
	if !ok {
		toasts.Show(toastWarning, "This backend can't log in to networks from here")
		return
	}
	p := &NetworksPanel{parent: view, api: api, busy: true, status: "Loading networks…",
		redraw: view.parent.Render, hide: func() { view.HideModal(); RequestRepaint() }}
	view.ShowModal(p)
	view.parent.Render()
	go p.reload("")
}

func (p *NetworksPanel) render() { p.redraw() }

// reload fetches the networks and goes back to the list, keeping the selection on focusName if given.
func (p *NetworksPanel) reload(focusName string) {
	defer debug.Recover()
	ctx, cancel := context.WithTimeout(context.Background(), netRequestTimeout)
	defer cancel()
	networks, err := p.api.Networks(ctx)
	p.mu.Lock()
	p.busy, p.status = false, ""
	if err != nil {
		p.errText = "Couldn't load networks: " + apiErrorText(err)
	} else {
		p.networks = networks
	}
	p.state, p.selected = netList, 0
	for i, n := range p.networks {
		if n.Name == focusName {
			p.selected = i
		}
	}
	p.mu.Unlock()
	if err != nil {
		toasts.Show(toastError, "Couldn't load networks: "+apiErrorText(err))
	}
	p.render()
}

// apiErrorText is the backend's message for an error, without the HTTP wrapping.
func apiErrorText(err error) string {
	var httpErr mautrix.HTTPError
	if errors.As(err, &httpErr) && httpErr.RespError != nil && httpErr.RespError.Err != "" {
		return httpErr.RespError.Err
	}
	var respErr mautrix.RespError
	if errors.As(err, &respErr) && respErr.Err != "" {
		return respErr.Err
	}
	return err.Error()
}

// networkStatus is the short state shown next to a network.
func networkStatus(n client.Network) (text string, ok bool) {
	switch {
	case n.Setup == client.SetupTelegramKeys:
		return "needs your API keys", false
	case !n.Running:
		return "not running", false
	case len(n.Logins) == 0:
		return "not logged in", false
	}
	l := n.Logins[0]
	name := l.Name
	if name == "" {
		name = "logged in"
	}
	switch l.State {
	case "CONNECTED":
		return name, true
	case "BAD_CREDENTIALS", "LOGGED_OUT":
		return name + " · logged out, log in again", false
	case "":
		return name + " · connecting", true
	default:
		return name + " · " + strings.ToLower(strings.ReplaceAll(l.State, "_", " ")), false
	}
}

func (p *NetworksPanel) close() {
	p.mu.Lock()
	p.closed = true
	p.leaveLoginLocked()
	p.mu.Unlock()
	p.hide()
	p.render()
}

// leaveLoginLocked abandons the login in progress: the backend cancels it, and late answers are ignored.
func (p *NetworksPanel) leaveLoginLocked() {
	p.gen++
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	if p.login != nil {
		loginID := p.login.LoginID
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = p.api.CancelLogin(ctx, loginID)
		}()
	}
	p.login, p.fields, p.browser, p.busy, p.status = nil, nil, false, false, ""
}

func (p *NetworksPanel) backToListLocked() {
	p.leaveLoginLocked()
	p.state, p.errText = netList, ""
	for i, n := range p.networks {
		if n.Name == p.network.Name {
			p.selected = i
		}
	}
}

// choose acts on the selected row of the list screens.
func (p *NetworksPanel) chooseLocked() {
	p.errText = ""
	switch p.state {
	case netList:
		if p.selected >= len(p.networks) {
			return
		}
		p.network = p.networks[p.selected]
		switch {
		case p.network.Setup == client.SetupTelegramKeys:
			p.state, p.focus = netKeys, 0
			p.fields = []loginField{
				{id: "api_id", label: "api_id", input: newSetupInput("", "a number, e.g. 1234567", false)},
				{id: "api_hash", label: "api_hash", input: newSetupInput("", "32 letters and digits", false)},
			}
			p.fields[0].input.Focus()
		case !p.network.Running:
			p.errText = networkName(p.network.Name) + " isn't running on the backend"
		case len(p.network.Logins) > 0:
			p.state, p.selected = netActions, 0
		default:
			p.startFlowsLocked()
		}
	case netActions:
		if p.selected == 0 {
			p.startFlowsLocked()
		} else {
			p.state = netConfirmLogout
		}
	case netFlows:
		if p.selected < len(p.network.Flows) {
			p.startLoginLocked(p.network.Flows[p.selected].ID)
		}
	}
}

// startFlowsLocked asks how to log in, or starts right away when there's only one way.
func (p *NetworksPanel) startFlowsLocked() {
	switch len(p.network.Flows) {
	case 0:
		p.errText = networkName(p.network.Name) + " has no way to log in from here"
	case 1:
		p.startLoginLocked(p.network.Flows[0].ID)
	default:
		p.state, p.selected = netFlows, 0
	}
}

func (p *NetworksPanel) startLoginLocked(flowID string) {
	p.leaveLoginLocked()
	p.state, p.busy, p.status = netStep, true, "Starting…"
	gen, network := p.gen, p.network.Name
	go func() {
		defer debug.Recover()
		ctx, cancel := context.WithTimeout(context.Background(), netRequestTimeout)
		defer cancel()
		resp, err := p.api.StartLogin(ctx, network, flowID)
		p.handleStep(gen, resp, err)
	}()
}

// handleStep shows what the backend answered, if the panel is still on that login.
func (p *NetworksPanel) handleStep(gen int, resp *client.LoginStep, err error) {
	p.mu.Lock()
	if gen != p.gen || p.closed {
		p.mu.Unlock()
		return
	}
	p.busy, p.status, p.browser = false, "", false
	var toast, success string
	switch {
	case err != nil && errors.Is(err, mautrix.MInvalidParam) && p.login != nil:
		// A mistake in the form: the login goes on, fix it and press Enter again.
		p.errText = apiErrorText(err)
		toast = p.errText
	case err != nil:
		p.login = nil
		p.errText = "Login failed: " + apiErrorText(err)
		toast = p.errText
	case resp.Step.Type == bridgev2.LoginStepTypeComplete:
		success = "Logged in to " + networkName(p.network.Name)
		p.login = nil
	default:
		p.login, p.errText = resp, ""
		p.prepareStepLocked()
	}
	network := p.network.Name
	p.mu.Unlock()
	if toast != "" {
		toasts.Show(toastError, toast)
	}
	if success != "" {
		toasts.Show(toastSuccess, success)
		go p.reload(network)
		return
	}
	p.render()
}

// prepareStepLocked sets up the current step: a form, a wait for a QR scan, or the browser.
func (p *NetworksPanel) prepareStepLocked() {
	step := p.login.Step
	p.fields, p.focus = nil, 0
	switch step.Type {
	case bridgev2.LoginStepTypeUserInput:
		for _, f := range step.UserInputParams.Fields {
			field := loginField{id: f.ID, label: f.Name, hint: f.Description}
			if f.Type == bridgev2.LoginInputFieldTypeSelect && len(f.Options) > 0 {
				field.options = f.Options
				for i, o := range f.Options {
					if o == f.DefaultValue {
						field.option = i
					}
				}
			} else {
				secret := f.Type == bridgev2.LoginInputFieldTypePassword || f.Type == bridgev2.LoginInputFieldTypeToken
				field.input = newSetupInput(f.DefaultValue, inputPlaceholder(f), secret)
			}
			p.fields = append(p.fields, field)
		}
		p.focusFieldLocked(0)
	case bridgev2.LoginStepTypeDisplayAndWait:
		p.busy, p.status = true, "Waiting for you…"
		p.waitLocked()
	case bridgev2.LoginStepTypeCookies:
		if browser := login.FindBrowser(); browser != "" {
			p.browser, p.busy = true, true
			p.status = "A browser window opened: log in to " + networkName(p.network.Name) + " there. It closes by itself when done."
			p.collectCookiesLocked(browser)
		} else {
			p.pasteCookieFieldsLocked()
		}
	}
}

func inputPlaceholder(f bridgev2.LoginInputDataField) string {
	switch f.Type {
	case bridgev2.LoginInputFieldTypePhoneNumber:
		return "+1 555 010 0001"
	case bridgev2.LoginInputFieldTypeEmail:
		return "you@example.com"
	case bridgev2.LoginInputFieldType2FACode:
		return "the code you received"
	}
	return ""
}

func (p *NetworksPanel) pasteCookieFieldsLocked() {
	p.fields = nil
	for _, f := range p.login.Step.CookiesParams.Fields {
		label, hint := f.ID, ""
		for _, src := range f.Sources {
			if src.Type == bridgev2.LoginCookieTypeCookie {
				label, hint = src.Name, "cookie on "+src.CookieDomain
				break
			}
		}
		if !f.Required {
			hint = strings.TrimPrefix(hint+" (optional)", " ")
		}
		p.fields = append(p.fields, loginField{id: f.ID, label: label, hint: hint, input: newSetupInput("", "paste the value", true)})
	}
	p.focusFieldLocked(0)
}

func (p *NetworksPanel) focusFieldLocked(i int) {
	if len(p.fields) == 0 {
		return
	}
	if p.fields[p.focus].input != nil {
		p.fields[p.focus].input.Blur()
	}
	p.focus = (i + len(p.fields)) % len(p.fields)
	if p.fields[p.focus].input != nil {
		p.fields[p.focus].input.Focus()
	}
}

// waitLocked waits in the background until the network moves on (e.g. the QR code was scanned, or a new one is due).
func (p *NetworksPanel) waitLocked() {
	gen, loginID, step := p.gen, p.login.LoginID, p.login.Step
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go func() {
		defer debug.Recover()
		resp, err := p.api.SubmitLogin(ctx, loginID, step, nil)
		if ctx.Err() != nil {
			return
		}
		p.handleStep(gen, resp, err)
	}()
}

// collectCookiesLocked opens a browser on this computer and hands what it collects to the backend.
func (p *NetworksPanel) collectCookiesLocked(browser string) {
	gen, loginID, step := p.gen, p.login.LoginID, p.login.Step
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	p.cancel = cancel
	go func() {
		defer debug.Recover()
		cookies, err := login.CollectCookies(ctx, browser, step.CookiesParams, io.Discard)
		if ctx.Err() != nil && err != nil {
			return
		}
		if err != nil {
			p.mu.Lock()
			if gen == p.gen && !p.closed {
				p.browser, p.busy, p.status = false, false, ""
				p.errText = "The browser didn't finish (" + err.Error() + "). Paste the values instead:"
				p.pasteCookieFieldsLocked()
			}
			p.mu.Unlock()
			p.render()
			return
		}
		resp, err := p.api.SubmitLogin(ctx, loginID, step, cookies)
		p.handleStep(gen, resp, err)
	}()
}

// submitLocked sends the form of the current screen.
func (p *NetworksPanel) submitLocked() {
	if p.busy || len(p.fields) == 0 {
		return
	}
	values := make(map[string]string, len(p.fields))
	for _, f := range p.fields {
		values[f.id] = strings.TrimSpace(f.value())
	}
	p.errText, p.busy = "", true
	if p.state == netKeys {
		p.status = "Saving…"
		go p.saveTelegramKeys(values["api_id"], values["api_hash"])
		return
	}
	p.status = "Checking…"
	gen, loginID, step := p.gen, p.login.LoginID, p.login.Step
	go func() {
		defer debug.Recover()
		ctx, cancel := context.WithTimeout(context.Background(), netRequestTimeout)
		defer cancel()
		resp, err := p.api.SubmitLogin(ctx, loginID, step, values)
		p.handleStep(gen, resp, err)
	}()
}

func (p *NetworksPanel) saveTelegramKeys(rawID, hash string) {
	defer debug.Recover()
	apiID, err := strconv.Atoi(rawID)
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), netRequestTimeout)
		err = p.api.SetTelegramKeys(ctx, apiID, hash)
		cancel()
	} else {
		err = errors.New("api_id is a number")
	}
	p.mu.Lock()
	p.busy, p.status = false, ""
	if err != nil {
		p.errText = apiErrorText(err)
	}
	p.mu.Unlock()
	if err != nil {
		toasts.Show(toastError, "Couldn't save the Telegram keys: "+apiErrorText(err))
		p.render()
		return
	}
	toasts.Show(toastSuccess, "Telegram keys saved. Restart tuimessenger (or its backend) to start Telegram, then log in here.")
	p.reload("telegram")
}

func (p *NetworksPanel) logout() {
	defer debug.Recover()
	p.mu.Lock()
	network := p.network
	p.busy, p.status = true, "Logging out…"
	p.mu.Unlock()
	p.render()
	ctx, cancel := context.WithTimeout(context.Background(), netRequestTimeout)
	defer cancel()
	err := p.api.LogoutNetwork(ctx, network.Name, network.Logins[0].ID)
	if err != nil {
		p.mu.Lock()
		p.busy, p.status, p.errText = false, "", "Couldn't log out: "+apiErrorText(err)
		p.state = netActions
		p.mu.Unlock()
		toasts.Show(toastError, "Couldn't log out of "+networkName(network.Name)+": "+apiErrorText(err))
		p.render()
		return
	}
	toasts.Show(toastSuccess, fmt.Sprintf("Logged out of %s", networkName(network.Name)))
	p.reload(network.Name)
}

// rowCount is how many choices the current list screen has.
func (p *NetworksPanel) rowCountLocked() int {
	switch p.state {
	case netList:
		return len(p.networks)
	case netActions:
		return 2
	case netFlows:
		return len(p.network.Flows)
	}
	return 0
}

func (p *NetworksPanel) OnKeyEvent(event mauview.KeyEvent) bool {
	if event.Key() == tcell.KeyCtrlC {
		p.close()
		return true
	}
	p.mu.Lock()
	closeIt, logout := false, false
	switch p.state {
	case netList, netActions, netFlows:
		switch event.Key() {
		case tcell.KeyEscape:
			if p.state == netList {
				closeIt = true
			} else {
				p.backToListLocked()
			}
		case tcell.KeyEnter:
			if !p.busy {
				p.chooseLocked()
			}
		case tcell.KeyDown, tcell.KeyTab:
			p.moveLocked(1)
		case tcell.KeyUp, tcell.KeyBacktab:
			p.moveLocked(-1)
		case tcell.KeyRune:
			switch event.Rune() {
			case 'j':
				p.moveLocked(1)
			case 'k':
				p.moveLocked(-1)
			case 'q':
				closeIt = p.state == netList
			}
		}
	case netConfirmLogout:
		switch {
		case event.Key() == tcell.KeyEnter || (event.Key() == tcell.KeyRune && (event.Rune() == 'y' || event.Rune() == 'Y')):
			logout = !p.busy
		case event.Key() == tcell.KeyEscape || (event.Key() == tcell.KeyRune && (event.Rune() == 'n' || event.Rune() == 'N')):
			p.state, p.selected = netActions, 1
		}
	case netStep, netKeys:
		p.formKeyLocked(event)
	}
	p.mu.Unlock()
	if closeIt {
		p.close()
		return true
	}
	if logout {
		go p.logout()
	}
	p.render()
	return true
}

func (p *NetworksPanel) moveLocked(delta int) {
	if n := p.rowCountLocked(); n > 0 {
		p.selected = (p.selected + delta + n) % n
	}
}

func (p *NetworksPanel) formKeyLocked(event mauview.KeyEvent) {
	switch event.Key() {
	case tcell.KeyEscape:
		p.backToListLocked()
		return
	case tcell.KeyEnter:
		if p.focus < len(p.fields)-1 {
			p.focusFieldLocked(p.focus + 1)
		} else {
			p.submitLocked()
		}
		return
	case tcell.KeyTab, tcell.KeyDown:
		p.focusFieldLocked(p.focus + 1)
		return
	case tcell.KeyBacktab, tcell.KeyUp:
		p.focusFieldLocked(p.focus - 1)
		return
	}
	if p.busy || len(p.fields) == 0 {
		return
	}
	f := &p.fields[p.focus]
	if len(f.options) > 0 {
		switch event.Key() {
		case tcell.KeyLeft:
			f.option = (f.option - 1 + len(f.options)) % len(f.options)
		case tcell.KeyRight:
			f.option = (f.option + 1) % len(f.options)
		}
		return
	}
	f.input.OnKeyEvent(event)
}

func (p *NetworksPanel) OnPasteEvent(event mauview.PasteEvent) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if (p.state == netStep || p.state == netKeys) && !p.busy && len(p.fields) > 0 && p.fields[p.focus].input != nil {
		p.fields[p.focus].input.OnPasteEvent(event)
	}
	return true
}

func (p *NetworksPanel) OnMouseEvent(event mauview.MouseEvent) bool {
	if event.Buttons() != tcell.Button1 || event.HasMotion() {
		return true
	}
	x, y := event.Position()
	p.mu.Lock()
	outside := x < p.x || x >= p.x+p.w || y < p.y || y >= p.y+p.h
	p.mu.Unlock()
	if outside {
		p.close()
	}
	return true
}

func (p *NetworksPanel) Focus() {}
func (p *NetworksPanel) Blur()  {}

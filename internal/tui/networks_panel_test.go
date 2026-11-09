package tui

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"rsc.io/qr"

	"github.com/whyyor/tuimessenger/internal/rpc/client"
)

// fakeNetworks is a backend with WhatsApp (two ways to log in), Telegram without keys, and Slack logged in.
type fakeNetworks struct {
	mu        sync.Mutex
	submits   []map[string]string
	answers   []func() (*client.LoginStep, error)
	cancelled []string
	loggedOut []string
	keys      string
}

func (f *fakeNetworks) Networks(context.Context) ([]client.Network, error) {
	return []client.Network{
		{Name: "whatsapp", Running: true, Flows: []bridgev2.LoginFlow{{ID: "qr", Name: "QR"}, {ID: "phone", Name: "Pairing code"}}},
		{Name: "telegram", Setup: client.SetupTelegramKeys},
		{Name: "slack", Running: true, State: "CONNECTED", Flows: []bridgev2.LoginFlow{{ID: "email", Name: "Email"}},
			Logins: []client.NetworkLogin{{ID: "T1-U1", Name: "Acme - me@acme.test", State: "CONNECTED"}}},
	}, nil
}
func (f *fakeNetworks) SetNetworkSetting(context.Context, string, string, bool) error { return nil }
func (f *fakeNetworks) StartLogin(_ context.Context, network, flowID string) (*client.LoginStep, error) {
	return &client.LoginStep{LoginID: "L1", Step: &bridgev2.LoginStep{Type: bridgev2.LoginStepTypeUserInput, StepID: "phone",
		UserInputParams: &bridgev2.LoginUserInputParams{Fields: []bridgev2.LoginInputDataField{
			{Type: bridgev2.LoginInputFieldTypePhoneNumber, ID: "phone", Name: "Phone number"}}}}}, nil
}
func (f *fakeNetworks) SubmitLogin(_ context.Context, _ string, _ *bridgev2.LoginStep, input map[string]string) (*client.LoginStep, error) {
	f.mu.Lock()
	f.submits = append(f.submits, input)
	answer := f.answers[0]
	f.answers = f.answers[1:]
	f.mu.Unlock()
	return answer()
}
func (f *fakeNetworks) CancelLogin(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancelled = append(f.cancelled, id)
	return nil
}
func (f *fakeNetworks) LogoutNetwork(_ context.Context, network, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loggedOut = append(f.loggedOut, network+"/"+id)
	return nil
}
func (f *fakeNetworks) SetTelegramKeys(_ context.Context, id int, hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.keys = hash
	return nil
}

func newTestPanel(api *fakeNetworks) *NetworksPanel {
	p := &NetworksPanel{api: api, redraw: func() {}, hide: func() {}}
	p.reload("")
	return p
}

func eventually(t *testing.T, what string, cond func(p *NetworksPanel) bool, p *NetworksPanel) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); ; {
		p.mu.Lock()
		ok := cond(p)
		p.mu.Unlock()
		if ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for: " + what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func keyPress(k tcell.Key) *tcell.EventKey { return tcell.NewEventKey(k, 0, tcell.ModNone) }
func runePress(r rune) *tcell.EventKey     { return tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone) }

func TestNetworkStatus(t *testing.T) {
	api := &fakeNetworks{}
	networks, _ := api.Networks(context.Background())
	for i, want := range []string{"not logged in", "needs your API keys", "Acme - me@acme.test"} {
		if got, _ := networkStatus(networks[i]); got != want {
			t.Errorf("%s: %q, want %q", networks[i].Name, got, want)
		}
	}
	bad := client.Network{Running: true, Logins: []client.NetworkLogin{{Name: "+91", State: "BAD_CREDENTIALS"}}}
	if got, ok := networkStatus(bad); ok || !strings.Contains(got, "log in again") {
		t.Errorf("logged out by the network: %q", got)
	}
}

func TestNetworksLoginFlow(t *testing.T) {
	invalid := mautrix.HTTPError{Response: &http.Response{StatusCode: 400},
		RespError: &mautrix.RespError{ErrCode: "M_INVALID_PARAM", Err: "Phone number: phone number must start with +"}}
	qrStep := &client.LoginStep{LoginID: "L1", Step: &bridgev2.LoginStep{Type: bridgev2.LoginStepTypeDisplayAndWait, StepID: "qr",
		DisplayAndWaitParams: &bridgev2.LoginDisplayAndWaitParams{Type: bridgev2.LoginDisplayTypeQR, Data: "2@abcdef,ghijkl"}}}
	done := &client.LoginStep{LoginID: "L1", Step: &bridgev2.LoginStep{Type: bridgev2.LoginStepTypeComplete, StepID: "done",
		CompleteParams: &bridgev2.LoginCompleteParams{UserLoginID: "919"}}}
	release := make(chan struct{})
	api := &fakeNetworks{answers: []func() (*client.LoginStep, error){
		func() (*client.LoginStep, error) { return nil, invalid },
		func() (*client.LoginStep, error) { return qrStep, nil },
		func() (*client.LoginStep, error) { <-release; return done, nil }, // the QR scan
	}}
	p := newTestPanel(api)
	if p.state != netList || len(p.networks) != 3 {
		t.Fatalf("list: state %v, %d networks", p.state, len(p.networks))
	}

	// WhatsApp has two ways: pick the second.
	p.OnKeyEvent(keyPress(tcell.KeyEnter))
	if p.state != netFlows {
		t.Fatalf("want the ways to log in, got state %v", p.state)
	}
	p.OnKeyEvent(keyPress(tcell.KeyDown))
	p.OnKeyEvent(keyPress(tcell.KeyEnter))
	eventually(t, "the phone form", func(p *NetworksPanel) bool { return len(p.fields) == 1 && !p.busy }, p)

	// A form mistake: shown, and the login goes on.
	for _, r := range "98765" {
		p.OnKeyEvent(runePress(r))
	}
	p.OnKeyEvent(keyPress(tcell.KeyEnter))
	eventually(t, "the mistake", func(p *NetworksPanel) bool { return strings.Contains(p.errText, "must start with +") }, p)
	if p.login == nil || p.state != netStep {
		t.Fatal("a form mistake must not end the login")
	}

	// Fixed: next comes a QR code, waited on in the background, drawn on screen.
	p.OnKeyEvent(keyPress(tcell.KeyEnter))
	eventually(t, "the QR step", func(p *NetworksPanel) bool {
		return p.login != nil && p.login.Step.StepID == "qr" && p.busy
	}, p)
	p.mu.Lock()
	rows, code := p.rowsLocked(netPanelWidth)
	p.mu.Unlock()
	qrRows := 0
	for _, r := range rows {
		if r.kind == netRowQR {
			qrRows++
		}
	}
	if code == nil || qrRows != (code.Size+2*qrQuietZone+1)/2 {
		t.Errorf("QR: code %v, %d rows", code != nil, qrRows)
	}
	close(release)
	eventually(t, "back on the list after logging in", func(p *NetworksPanel) bool { return p.state == netList && p.login == nil }, p)
	if got := api.submits[0]["phone"]; got != "98765" {
		t.Errorf("first submit: %v", api.submits)
	}
}

func TestNetworksCancelLogoutAndKeys(t *testing.T) {
	api := &fakeNetworks{answers: []func() (*client.LoginStep, error){}}
	p := newTestPanel(api)

	// Esc during a login cancels it on the backend.
	p.OnKeyEvent(keyPress(tcell.KeyEnter))
	p.OnKeyEvent(keyPress(tcell.KeyEnter))
	eventually(t, "the form", func(p *NetworksPanel) bool { return p.login != nil && !p.busy }, p)
	p.OnKeyEvent(keyPress(tcell.KeyEscape))
	eventually(t, "the cancel", func(*NetworksPanel) bool { api.mu.Lock(); defer api.mu.Unlock(); return len(api.cancelled) == 1 }, p)
	if p.state != netList {
		t.Errorf("Esc goes back to the list, state %v", p.state)
	}

	// Slack is logged in: Log out, confirmed with y.
	p.OnKeyEvent(keyPress(tcell.KeyDown))
	p.OnKeyEvent(keyPress(tcell.KeyDown))
	p.OnKeyEvent(keyPress(tcell.KeyEnter))
	if p.state != netActions {
		t.Fatalf("a logged-in network offers actions, state %v", p.state)
	}
	p.OnKeyEvent(keyPress(tcell.KeyDown))
	p.OnKeyEvent(keyPress(tcell.KeyEnter))
	if p.state != netConfirmLogout {
		t.Fatalf("logging out asks first, state %v", p.state)
	}
	p.OnKeyEvent(runePress('y'))
	eventually(t, "the logout", func(*NetworksPanel) bool { api.mu.Lock(); defer api.mu.Unlock(); return len(api.loggedOut) == 1 }, p)
	if api.loggedOut[0] != "slack/T1-U1" {
		t.Errorf("logged out %v", api.loggedOut)
	}

	// Telegram asks for its keys.
	eventually(t, "the list again", func(p *NetworksPanel) bool { return p.state == netList && !p.busy }, p)
	p.mu.Lock()
	p.selected = 1
	p.mu.Unlock()
	p.OnKeyEvent(keyPress(tcell.KeyEnter))
	if p.state != netKeys || len(p.fields) != 2 {
		t.Fatalf("Telegram keys form: state %v, %d fields", p.state, len(p.fields))
	}
	for _, r := range "123" {
		p.OnKeyEvent(runePress(r))
	}
	p.OnKeyEvent(keyPress(tcell.KeyEnter))
	for _, r := range "0123456789abcdef0123456789abcdef" {
		p.OnKeyEvent(runePress(r))
	}
	p.OnKeyEvent(keyPress(tcell.KeyEnter))
	eventually(t, "the keys saved", func(*NetworksPanel) bool { api.mu.Lock(); defer api.mu.Unlock(); return api.keys != "" }, p)
}

func TestQRDrawsDarkOnLight(t *testing.T) {
	code, err := qr.Encode("2@abcdef", qr.L)
	if err != nil {
		t.Fatal(err)
	}
	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(80, 40)
	for line := 0; line < (code.Size+2*qrQuietZone+1)/2; line++ {
		drawQRLine(sim, code, line, 0, line)
	}
	// The top-left corner is quiet zone (light), and the finder pattern's corner just inside it is dark.
	_, _, st, _ := sim.GetContent(0, 0)
	if fg, _, _ := st.Decompose(); fg != tcell.NewHexColor(0xffffff) {
		t.Errorf("quiet zone should be light, got %v", fg)
	}
	_, _, st, _ = sim.GetContent(qrQuietZone, qrQuietZone/2)
	if fg, _, _ := st.Decompose(); fg != tcell.NewHexColor(0x000000) {
		t.Errorf("finder pattern should be dark, got %v", fg)
	}
}

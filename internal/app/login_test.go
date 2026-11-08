package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
)

// fakeLogin is a scripted network login: each submit or wait returns the next scripted step (or error).
type fakeLogin struct {
	next      []*bridgev2.LoginStep
	errs      []error
	inputs    []map[string]string
	cancelled int
}

func (f *fakeLogin) Start(context.Context) (*bridgev2.LoginStep, error) { return f.pop() }
func (f *fakeLogin) Cancel()                                            { f.cancelled++ }
func (f *fakeLogin) Wait(context.Context) (*bridgev2.LoginStep, error)  { return f.pop() }
func (f *fakeLogin) SubmitUserInput(_ context.Context, in map[string]string) (*bridgev2.LoginStep, error) {
	f.inputs = append(f.inputs, in)
	return f.pop()
}
func (f *fakeLogin) SubmitCookies(_ context.Context, in map[string]string) (*bridgev2.LoginStep, error) {
	f.inputs = append(f.inputs, in)
	return f.pop()
}

func (f *fakeLogin) pop() (*bridgev2.LoginStep, error) {
	step, err := f.next[0], f.errs[0]
	f.next, f.errs = f.next[1:], f.errs[1:]
	return step, err
}

func (f *fakeLogin) then(step *bridgev2.LoginStep, err error) *fakeLogin {
	f.next, f.errs = append(f.next, step), append(f.errs, err)
	return f
}

var (
	phoneStep = &bridgev2.LoginStep{Type: bridgev2.LoginStepTypeUserInput, StepID: "phone",
		UserInputParams: &bridgev2.LoginUserInputParams{Fields: []bridgev2.LoginInputDataField{
			{Type: bridgev2.LoginInputFieldTypePhoneNumber, ID: "phone", Name: "Phone number"},
			{Type: bridgev2.LoginInputFieldTypePassword, ID: "2fa", Name: "Password", Optional: true},
		}}}
	qrStep = &bridgev2.LoginStep{Type: bridgev2.LoginStepTypeDisplayAndWait, StepID: "qr",
		DisplayAndWaitParams: &bridgev2.LoginDisplayAndWaitParams{Type: bridgev2.LoginDisplayTypeQR, Data: "2@abc"}}
	doneStep = &bridgev2.LoginStep{Type: bridgev2.LoginStepTypeComplete, StepID: "done",
		CompleteParams: &bridgev2.LoginCompleteParams{UserLoginID: "919000000000"}}
)

// started registers a login whose first step is first, as StartLogin would.
func started(t *testing.T, a *App, proc *fakeLogin, first *bridgev2.LoginStep) string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	s := &loginSession{id: "L" + first.StepID, network: "whatsapp", proc: proc, ctx: ctx, cancel: cancel}
	a.loginsLock.Lock()
	if a.logins == nil {
		a.logins = map[string]*loginSession{}
	}
	a.logins[s.id] = s
	a.loginsLock.Unlock()
	if _, err := a.afterStep(s, first, nil); err != nil {
		t.Fatal(err)
	}
	return s.id
}

func TestLoginFormValidation(t *testing.T) {
	a := &App{}
	ctx := context.Background()
	proc := (&fakeLogin{}).then(doneStep, nil)
	id := started(t, a, proc, phoneStep)

	// A mistake is the user's to fix: the login goes on and nothing reaches the network.
	if _, err := a.SubmitLogin(ctx, id, "phone", bridgev2.LoginStepTypeUserInput, map[string]string{}); !errors.Is(err, mautrix.MInvalidParam) {
		t.Errorf("missing phone: %v", err)
	}
	if _, err := a.SubmitLogin(ctx, id, "phone", bridgev2.LoginStepTypeUserInput, map[string]string{"phone": "98765"}); !errors.Is(err, mautrix.MInvalidParam) || !strings.Contains(err.Error(), "+") {
		t.Errorf("no country code: %v", err)
	}
	if len(proc.inputs) != 0 || a.loginSession(id) == nil {
		t.Fatal("invalid input must not reach the network or end the login")
	}
	// A stale step is refused.
	if _, err := a.SubmitLogin(ctx, id, "other", bridgev2.LoginStepTypeUserInput, nil); !errors.Is(err, mautrix.RespError{ErrCode: "M_BAD_STATE"}) {
		t.Errorf("stale step: %v", err)
	}
	// Valid: cleaned up, optional field left empty, and done.
	resp, err := a.SubmitLogin(ctx, id, "phone", bridgev2.LoginStepTypeUserInput, map[string]string{"phone": "+1 555 010-0001"})
	if err != nil || resp.Step.Type != bridgev2.LoginStepTypeComplete {
		t.Fatalf("valid: %+v %v", resp, err)
	}
	if got := proc.inputs[0]; got["phone"] != "+15550100001" || got["2fa"] != "" {
		t.Errorf("network got %v", got)
	}
	if a.loginSession(id) != nil || proc.cancelled != 0 {
		t.Errorf("a finished login is forgotten without cancelling it (cancelled %d)", proc.cancelled)
	}
}

func TestLoginWaitAndErrors(t *testing.T) {
	a := &App{}
	ctx := context.Background()

	// QR: waiting can bring a fresh code, then completion.
	proc := (&fakeLogin{}).then(qrStep, nil).then(doneStep, nil)
	id := started(t, a, proc, qrStep)
	if resp, err := a.SubmitLogin(ctx, id, "qr", bridgev2.LoginStepTypeDisplayAndWait, nil); err != nil || resp.Step.StepID != "qr" {
		t.Fatalf("refreshed QR: %+v %v", resp, err)
	}
	if resp, err := a.SubmitLogin(ctx, id, "qr", bridgev2.LoginStepTypeDisplayAndWait, nil); err != nil || resp.Step.Type != bridgev2.LoginStepTypeComplete {
		t.Fatalf("scanned: %+v %v", resp, err)
	}

	// A network error ends the login, keeps the network's own words, and cancels the process once.
	proc = (&fakeLogin{}).then(nil, errors.New("invalid code"))
	id = started(t, a, proc, phoneStep)
	_, err := a.SubmitLogin(ctx, id, "phone", bridgev2.LoginStepTypeUserInput, map[string]string{"phone": "+15550100002"})
	if err == nil || !strings.Contains(err.Error(), "invalid code") || strings.Contains(err.Error(), "Internal") {
		t.Errorf("network error should reach the user as is: %v", err)
	}
	if a.loginSession(id) != nil || proc.cancelled != 1 {
		t.Errorf("failed login: still there=%v, cancelled %d", a.loginSession(id) != nil, proc.cancelled)
	}
	if _, err = a.SubmitLogin(ctx, id, "phone", bridgev2.LoginStepTypeUserInput, nil); !errors.Is(err, mautrix.MNotFound) {
		t.Errorf("after the end: %v", err)
	}

	// Cancelling: once, and the login is gone.
	proc = &fakeLogin{}
	id = started(t, a, proc, qrStep)
	if err = a.CancelLogin(ctx, id); err != nil || proc.cancelled != 1 {
		t.Errorf("cancel: %v, cancelled %d", err, proc.cancelled)
	}
	if err = a.CancelLogin(ctx, id); !errors.Is(err, mautrix.MNotFound) || proc.cancelled != 1 {
		t.Errorf("cancel twice: %v, cancelled %d", err, proc.cancelled)
	}

	// A step type we can't show ends the login with a clear message.
	proc = (&fakeLogin{}).then(&bridgev2.LoginStep{Type: bridgev2.LoginStepTypeWebAuthn, StepID: "passkey"}, nil)
	id = started(t, a, proc, qrStep)
	if _, err = a.SubmitLogin(ctx, id, "qr", bridgev2.LoginStepTypeDisplayAndWait, nil); err == nil || !strings.Contains(err.Error(), "isn't supported") {
		t.Errorf("passkey step: %v", err)
	}
}

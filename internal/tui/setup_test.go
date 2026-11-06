package tui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/mauview"
)

func TestNormalizeBackendURL(t *testing.T) {
	for in, want := range map[string]string{
		"pi:8008":                  "http://pi:8008",
		" http://pi:8008/ ":        "http://pi:8008",
		"https://pi.tail.ts.net":   "https://pi.tail.ts.net",
		"http://100.64.0.1:8008//": "http://100.64.0.1:8008",
	} {
		if got, err := normalizeBackendURL(in); err != nil || got != want {
			t.Errorf("%q: got %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "   ", "ftp://pi", "http://"} {
		if got, err := normalizeBackendURL(bad); err == nil {
			t.Errorf("%q should be refused, got %q", bad, got)
		}
	}
}

func TestCheckBackend(t *testing.T) {
	serve := func(status int, body string) string {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/_matrix/client/versions" {
				http.NotFound(w, r)
				return
			}
			w.WriteHeader(status)
			fmt.Fprint(w, body)
		}))
		t.Cleanup(srv.Close)
		return srv.URL
	}
	ctx := context.Background()
	if err := checkBackend(ctx, serve(200, `{"versions":["v1.11"],"unstable_features":{"com.tuimessenger.no_e2ee":true}}`)); err != nil {
		t.Errorf("tuimessenger backend refused: %v", err)
	}
	if err := checkBackend(ctx, serve(200, `{"versions":["v1.11"],"unstable_features":{}}`)); !errors.Is(err, errNotTuimessenger) {
		t.Errorf("plain Matrix server: %v", err)
	}
	if err := checkBackend(ctx, serve(404, `not here`)); !errors.Is(err, errNotTuimessenger) {
		t.Errorf("other web server: %v", err)
	}
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	if err := checkBackend(ctx, closed.URL); !errors.Is(err, errBackendUnreachable) {
		t.Errorf("nothing listening: %v", err)
	}
}

func TestSetupErrorText(t *testing.T) {
	unreachable := fmt.Errorf("%w: connection refused", errBackendUnreachable)
	for _, c := range []struct {
		mode setupMode
		err  error
		want string
	}{
		{setupComplete, unreachable, "No tuimessenger backend is running on this machine"},
		{setupSelfHosted, unreachable, "Can't reach http://b"},
		{setupSelfHosted, errNotTuimessenger, "isn't a tuimessenger backend"},
		{setupComplete, errors.New("M_FORBIDDEN (HTTP 403): Invalid username or password"), "Wrong username or password."},
		{setupComplete, errors.New("boom"), "Login failed: boom"},
	} {
		if got := setupErrorText(c.mode, "http://b", c.err); !strings.Contains(got, c.want) {
			t.Errorf("%v: got %q, want it to contain %q", c.err, got, c.want)
		}
	}
}

func TestSetupModeChooser(t *testing.T) {
	if got := nextMode(setupComplete, 1); got != setupSelfHosted {
		t.Errorf("down from Complete: %v", got)
	}
	if got := nextMode(setupSelfHosted, 1); got != setupComplete {
		t.Errorf("down from Self-hosted should skip Host backend (not available yet) and wrap: %v", got)
	}
	if got := nextMode(setupComplete, -1); got != setupSelfHosted {
		t.Errorf("up from Complete should wrap past Host backend: %v", got)
	}
	view := newSetupView(SetupDefaults{Server: "http://127.0.0.1:8008", Username: "me"})
	view.OnKeyEvent(key('3'))
	if !view.choosing {
		t.Error("Host backend isn't available yet and must not open")
	}
	view.OnKeyEvent(key('2'))
	if view.choosing || view.mode != setupSelfHosted || len(view.fields) != 3 || view.fields[1].input.GetText() != "me" {
		t.Errorf("self-hosted form: choosing=%v mode=%v fields=%d", view.choosing, view.mode, len(view.fields))
	}
	view.OnKeyEvent(tcell.NewEventKey(tcell.KeyEscape, 0, 0))
	if !view.choosing || view.fields != nil {
		t.Error("Esc should go back to the mode list")
	}
}

func key(r rune) mauview.KeyEvent { return tcell.NewEventKey(tcell.KeyRune, r, 0) }

func typeText(view *SetupView, text string) {
	for _, r := range text {
		view.OnKeyEvent(key(r))
	}
}

func waitIdle(t *testing.T, view *SetupView) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); view.isBusy(); {
		if time.Now().After(deadline) {
			t.Fatal("setup still busy")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestSetupLogin(t *testing.T) {
	enter := tcell.NewEventKey(tcell.KeyEnter, 0, 0)
	type call struct{ base, user, pass string }
	newView := func(checkErr, loginErr error) (*SetupView, *[]call, *bool) {
		var calls []call
		loggedIn := false
		view := newSetupView(SetupDefaults{Server: "http://127.0.0.1:8008", Username: "me"})
		view.check = func(context.Context, string) error { return checkErr }
		view.login = func(_ context.Context, base, user, pass string) error {
			calls = append(calls, call{base, user, pass})
			return loginErr
		}
		view.onLoggedIn = func() { loggedIn = true }
		return view, &calls, &loggedIn
	}

	// Complete setup: only the password; server and username come from this machine.
	view, calls, loggedIn := newView(nil, nil)
	view.OnKeyEvent(key('1'))
	view.OnKeyEvent(enter)
	if view.errorText() != "Enter your password." || len(*calls) != 0 {
		t.Errorf("empty password: %q, %d calls", view.errorText(), len(*calls))
	}
	typeText(view, "secret")
	view.OnKeyEvent(enter)
	waitIdle(t, view)
	if len(*calls) != 1 || (*calls)[0] != (call{"http://127.0.0.1:8008", "me", "secret"}) || !*loggedIn {
		t.Errorf("complete setup: calls=%+v loggedIn=%v err=%q", *calls, *loggedIn, view.errorText())
	}

	// Self-hosted: Enter walks the fields, the address is normalised, and a wrong password is explained.
	view, calls, loggedIn = newView(nil, errors.New("M_FORBIDDEN (HTTP 403): Invalid username or password"))
	view.OnKeyEvent(key('2'))
	typeText(view, "pi:8008")
	view.OnKeyEvent(enter)
	view.OnKeyEvent(enter) // username keeps "me"
	typeText(view, "nope")
	view.OnKeyEvent(enter)
	waitIdle(t, view)
	if len(*calls) != 1 || (*calls)[0] != (call{"http://pi:8008", "me", "nope"}) || *loggedIn ||
		view.errorText() != "Wrong username or password." {
		t.Errorf("self-hosted: calls=%+v loggedIn=%v err=%q", *calls, *loggedIn, view.errorText())
	}

	// No backend: login isn't tried and the message says how to start it.
	view, calls, loggedIn = newView(fmt.Errorf("%w: refused", errBackendUnreachable), nil)
	view.OnKeyEvent(key('1'))
	typeText(view, "secret")
	view.OnKeyEvent(enter)
	waitIdle(t, view)
	if len(*calls) != 0 || *loggedIn || !strings.Contains(view.errorText(), "Start it with `tuimessenger`") {
		t.Errorf("no backend: calls=%+v loggedIn=%v err=%q", *calls, *loggedIn, view.errorText())
	}
}

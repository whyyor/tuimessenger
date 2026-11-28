package login

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"maunium.net/go/mautrix/bridgev2"
)

func TestDomainMatches(t *testing.T) {
	for _, tc := range []struct {
		cookie, want string
		ok           bool
	}{
		{".instagram.com", "instagram.com", true},
		{"www.instagram.com", "instagram.com", true},
		{"instagram.com", "instagram.com", true},
		{"evilinstagram.com", "instagram.com", false},
		{"instagram.com.evil.net", "instagram.com", false},
	} {
		if got := domainMatches(tc.cookie, tc.want); got != tc.ok {
			t.Errorf("domainMatches(%q, %q) = %v", tc.cookie, tc.want, got)
		}
	}
}

func TestPickCookies(t *testing.T) {
	fields := []bridgev2.LoginCookieField{
		{ID: "sessionid", Required: true, Sources: []bridgev2.LoginCookieFieldSource{{Type: bridgev2.LoginCookieTypeCookie, Name: "sessionid", CookieDomain: "instagram.com"}}},
		{ID: "mid", Sources: []bridgev2.LoginCookieFieldSource{{Type: bridgev2.LoginCookieTypeCookie, Name: "mid", CookieDomain: "instagram.com"}}},
	}
	cookies := []*network.Cookie{
		{Name: "sessionid", Value: "from-other-site", Domain: ".example.com"},
		{Name: "sessionid", Value: "s3cret", Domain: ".instagram.com"},
	}
	values := pickCookies(fields, cookies)
	if values["sessionid"] != "s3cret" || values["mid"] != "" {
		t.Errorf("picked %v", values)
	}
	if !hasRequired(fields, values) || hasRequired(fields, map[string]string{}) {
		t.Error("hasRequired is wrong")
	}
}

// TestCollectCookies drives a real (headless) browser through a fake login site.
func TestCollectCookies(t *testing.T) {
	browser := FindBrowser()
	if browser == "" || testing.Short() {
		t.Skip("no Chromium-based browser installed")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: "s3cret", Path: "/", HttpOnly: true})
		w.Header().Set("Content-Type", "text/html")
		// The "login" finishes a moment later, like a real site after you press the button.
		_, _ = io.WriteString(w, `<script>
			localStorage.setItem("token", "xoxc-123");
			setTimeout(() => location.href = "/done", 1500);
		</script>`)
	})
	mux.HandleFunc("/done", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "logged in")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	headless = true
	defer func() { headless = false }()
	params := &bridgev2.LoginCookiesParams{
		URL: srv.URL + "/login",
		Fields: []bridgev2.LoginCookieField{
			{ID: "sessionid", Required: true, Sources: []bridgev2.LoginCookieFieldSource{{Type: bridgev2.LoginCookieTypeCookie, Name: "sessionid", CookieDomain: "127.0.0.1"}}},
			{ID: "token", Required: true, Sources: []bridgev2.LoginCookieFieldSource{{Type: bridgev2.LoginCookieTypeLocalStorage, Name: "token"}}},
			{ID: "optional", Sources: []bridgev2.LoginCookieFieldSource{{Type: bridgev2.LoginCookieTypeCookie, Name: "optional"}}},
		},
		WaitForURLPattern: "/done$",
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	start := time.Now()
	values, err := CollectCookies(ctx, browser, params, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if values["sessionid"] != "s3cret" || values["token"] != "xoxc-123" {
		t.Errorf("collected %v (the HttpOnly cookie and the local storage value should both be there)", values)
	}
	if time.Since(start) < time.Second {
		t.Error("it should wait for the logged-in page, not finish as soon as the cookies exist")
	}
}

// On Linux browsers are commands on PATH, named differently per distro (Arch's Chrome is google-chrome-stable).
func TestFindBrowserOnPath(t *testing.T) {
	oldPaths := browserPaths
	t.Cleanup(func() { browserPaths = oldPaths })
	browserPaths = nil // as on Linux: no /Applications
	t.Setenv("TUIMESSENGER_BROWSER", "")
	dir := t.TempDir()
	chrome := filepath.Join(dir, "google-chrome-stable")
	if err := os.WriteFile(chrome, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if got := FindBrowser(); got != chrome {
		t.Errorf("got %q, want %q", got, chrome)
	}
	t.Setenv("PATH", t.TempDir())
	if got := FindBrowser(); got != "" {
		t.Errorf("no browser on PATH: got %q", got)
	}
	t.Setenv("TUIMESSENGER_BROWSER", "/opt/my/chrome")
	if got := FindBrowser(); got != "/opt/my/chrome" {
		t.Errorf("TUIMESSENGER_BROWSER should win, got %q", got)
	}
}

func TestCookieHelp(t *testing.T) {
	params := &bridgev2.LoginCookiesParams{URL: "https://www.instagram.com/accounts/login/", Fields: []bridgev2.LoginCookieField{
		{ID: "sessionid", Required: true, Sources: []bridgev2.LoginCookieFieldSource{
			{Type: bridgev2.LoginCookieTypeCookie, Name: "sessionid", CookieDomain: ".instagram.com"}}}}}
	help := CookieHelp("Instagram", params)
	for _, want := range []string{"Instagram login page", params.URL, "Cookies → instagram.com", "TUIMESSENGER_BROWSER"} {
		if !strings.Contains(help, want) {
			t.Errorf("help %q is missing %q", help, want)
		}
	}
}

// probeSite is a fake login whose page stores what a bot check sees first, navigator.webdriver, in a cookie.
func probeSite(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: "s3cret", Path: "/", HttpOnly: true})
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<script>
			document.cookie = "webdriver=" + navigator.webdriver + "; path=/";
			setTimeout(() => location.href = "/done", 1500);
		</script>`)
	})
	mux.HandleFunc("/done", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "logged in")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func cookieField(name string) bridgev2.LoginCookieField {
	return bridgev2.LoginCookieField{ID: name, Required: true, Sources: []bridgev2.LoginCookieFieldSource{
		{Type: bridgev2.LoginCookieTypeCookie, Name: name, CookieDomain: "127.0.0.1"}}}
}

func headlessBrowser(t *testing.T) string {
	browser := FindBrowser()
	if browser == "" || testing.Short() {
		t.Skip("no Chromium-based browser installed")
	}
	headless = true
	t.Cleanup(func() { headless = false })
	return browser
}

// The login browser doesn't tell pages it's automated (Meta's captchas never let such a browser in), in either mode.
// Cookie-only logins (Meta's) don't attach to the page at all, and still read HttpOnly cookies and wait for the page.
func TestLoginBrowserLooksManual(t *testing.T) {
	browser := headlessBrowser(t)
	srv := probeSite(t)
	storageField := bridgev2.LoginCookieField{ID: "x", Sources: []bridgev2.LoginCookieFieldSource{{Type: bridgev2.LoginCookieTypeLocalStorage, Name: "x"}}}
	for _, mode := range []struct {
		name   string
		fields []bridgev2.LoginCookieField
	}{{"hands-off", nil}, {"attached", []bridgev2.LoginCookieField{storageField}}} {
		params := &bridgev2.LoginCookiesParams{URL: srv.URL + "/login", WaitForURLPattern: "/done$",
			Fields: append([]bridgev2.LoginCookieField{cookieField("sessionid"), cookieField("webdriver")}, mode.fields...)}
		if got := needsPage(params); got != (mode.name == "attached") {
			t.Errorf("%s: needsPage = %v", mode.name, got)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		start := time.Now()
		values, err := CollectCookies(ctx, browser, params, io.Discard)
		cancel()
		if err != nil {
			t.Fatalf("%s: %v", mode.name, err)
		}
		if values["webdriver"] != "false" || values["sessionid"] != "s3cret" {
			t.Errorf("%s: %v (navigator.webdriver should be false, and the HttpOnly cookie read)", mode.name, values)
		}
		if time.Since(start) < time.Second {
			t.Errorf("%s: it should wait for the logged-in page", mode.name)
		}
	}
}

// Closing the window counts as done once the cookies are there, even before the final page (as in attached mode).
func TestHandsOffWindowClosed(t *testing.T) {
	browser := headlessBrowser(t)
	srv := probeSite(t)
	params := &bridgev2.LoginCookiesParams{URL: srv.URL + "/login", WaitForURLPattern: "/never$",
		Fields: []bridgev2.LoginCookieField{cookieField("sessionid")}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	// You close the window after logging in: once a check has seen the cookie. On Linux that quits the browser.
	afterPoll = func(ctx context.Context, values map[string]string) {
		if values["sessionid"] == "" {
			return
		}
		tabs, _ := chromedp.CallBrowser(ctx, target.GetTargets, target.GetTargetsParams{})
		for _, tab := range tabs.TargetInfos {
			if tab.Type == "page" {
				_, _ = chromedp.CallBrowser(ctx, target.CloseTarget, target.CloseTargetParams{TargetID: tab.TargetID})
			}
		}
	}
	t.Cleanup(func() { afterPoll = nil })
	start := time.Now()
	values, err := CollectCookies(ctx, browser, params, io.Discard)
	if err != nil || values["sessionid"] != "s3cret" {
		t.Errorf("closing the window after logging in: %v, %v", values, err)
	}
	if waited := time.Since(start); waited > 20*time.Second {
		t.Errorf("it took %v: closing the window should end the login, not the timeout", waited)
	}
}

// A login that ends unfinished says what was missing and where the tabs were.
func TestUnfinishedLoginSaysWhy(t *testing.T) {
	browser := headlessBrowser(t)
	srv := probeSite(t)
	params := &bridgev2.LoginCookiesParams{URL: srv.URL + "/login", WaitForURLPattern: "/never$",
		Fields: []bridgev2.LoginCookieField{cookieField("sessionid"), cookieField("c_user")}}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	_, err := CollectCookies(ctx, browser, params, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "missing c_user;") || !strings.Contains(err.Error(), srv.URL+"/done") {
		t.Errorf("error: %v", err)
	}
}

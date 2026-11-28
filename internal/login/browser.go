package login

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"maunium.net/go/mautrix/bridgev2"
)

// Browser login: open a real browser window, let you log in normally (captchas, 2FA and all), then read the
// cookies the network needs through the Chrome DevTools protocol, which also sees HttpOnly cookies.

const browserLoginTimeout = 10 * time.Minute

// browserPaths are Chromium-based browsers (they speak the DevTools protocol), most common first.
var browserPaths = []string{
	"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	"/Applications/Chromium.app/Contents/MacOS/Chromium",
	"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
	"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
	"/Applications/Helium.app/Contents/MacOS/Helium",
}

// browserCommands are the same browsers' commands on Linux (and other Unixes), looked up on PATH; distros name them
// differently (Arch's Google Chrome is google-chrome-stable, Debian's Chromium is chromium-browser).
var browserCommands = []string{
	"google-chrome-stable", "google-chrome", "chromium", "chromium-browser",
	"brave", "brave-browser", "brave-browser-stable", "microsoft-edge-stable", "microsoft-edge",
	"vivaldi-stable", "vivaldi", "helium-browser",
}

// FindBrowser returns a Chromium-based browser to log in with, or "" if there is none (e.g. on a headless Pi).
// TUIMESSENGER_BROWSER picks one explicitly.
func FindBrowser() string {
	if path := os.Getenv("TUIMESSENGER_BROWSER"); path != "" {
		return path
	}
	for _, path := range browserPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	for _, name := range browserCommands {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

// CookieHelp tells someone without a supported browser how to log in by hand: the page, and where its cookies are.
func CookieHelp(network string, params *bridgev2.LoginCookiesParams) string {
	domain := ""
	for _, f := range params.Fields {
		for _, src := range f.Sources {
			if src.Type == bridgev2.LoginCookieTypeCookie && src.CookieDomain != "" && domain == "" {
				domain = strings.TrimPrefix(src.CookieDomain, ".")
			}
		}
	}
	where := "Cookies"
	if domain != "" {
		where += " → " + domain
	}
	return fmt.Sprintf("No Chrome, Chromium, Brave or Edge found, so the %s login page opened in your default browser "+
		"(%s). Log in there, then copy each value below from its developer tools (F12 → Storage or Application → %s). "+
		"To log in automatically instead, install Chromium or set TUIMESSENGER_BROWSER.", network, params.URL, where)
}

// headless hides the window; only tests use it, a real login needs you to see the page.
var headless = false

// afterPoll, when set (tests only), runs after each successful check with what it found, e.g. to close the window.
var afterPoll func(ctx context.Context, values map[string]string)

// CollectCookies opens params.URL in a fresh browser profile and returns the requested values once you've logged in.
func CollectCookies(ctx context.Context, browser string, params *bridgev2.LoginCookiesParams, out io.Writer) (map[string]string, error) {
	var urlPattern *regexp.Regexp
	if params.WaitForURLPattern != "" {
		var err error
		if urlPattern, err = regexp.Compile(params.WaitForURLPattern); err != nil {
			return nil, fmt.Errorf("network gave a bad URL pattern: %w", err)
		}
	}
	// A throwaway profile: nothing from your normal browser is touched, and the session is deleted afterwards.
	profile, err := os.MkdirTemp("", "tuimessenger-login-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(profile)

	// No user agent from params: Meta asks for a Linux Chrome one, which contradicts everything else the real browser
	// shows, and its login answers that with captchas that never let you in. Instagram's login doesn't ask for one.
	opts := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(browser),
		chromedp.UserDataDir(profile),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Flag("headless", headless),
		chromedp.WindowSize(1100, 900),
		// Any remote-controlled Chrome tells pages navigator.webdriver = true, the first thing bot checks look at;
		// Meta then shows captchas that never let you in. This keeps it false, as in a browser you opened yourself.
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
	}
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
	defer cancelAlloc()
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()
	browserCtx, cancelTimeout := context.WithTimeout(browserCtx, browserLoginTimeout)
	defer cancelTimeout()

	var w watcher = handsOff{}
	if needsPage(params) {
		w = attached{}
	}
	if err = w.open(browserCtx, params); err != nil {
		return nil, err
	}
	fmt.Fprintln(out, "Log in in the browser window that opened; it closes by itself once you're logged in.")

	var found map[string]string
	var tabs []string
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for done := false; !done; {
		st, err := w.poll(browserCtx, params, urlPattern)
		if err == nil {
			found, tabs = st.values, st.urls
			if hasRequired(params.Fields, found) && st.atURL {
				return found, nil
			}
			if afterPoll != nil {
				afterPoll(browserCtx, found)
			}
			if !st.open {
				break // the window was closed (on macOS the browser itself keeps running)
			}
		}
		select {
		case <-browserCtx.Done():
			done = true
		case <-ticker.C:
		}
	}
	// Closing the window after logging in still counts, even if the final page wasn't reached.
	if hasRequired(params.Fields, found) {
		return found, nil
	}
	// Say what was missing, so a login that never finishes can be told apart from one on an unexpected page.
	where := fmt.Sprintf("missing %s; tabs: %s", strings.Join(missingFields(params.Fields, found), ", "), strings.Join(tabs, " "))
	switch {
	case errors.Is(browserCtx.Err(), context.DeadlineExceeded):
		return nil, fmt.Errorf("gave up waiting for the login after 10 minutes (%s)", where)
	case ctx.Err() != nil:
		return nil, fmt.Errorf("login cancelled (%s)", where)
	}
	return nil, fmt.Errorf("the browser was closed before logging in (%s)", where)
}

func missingFields(fields []bridgev2.LoginCookieField, values map[string]string) []string {
	var missing []string
	for _, field := range fields {
		if field.Required && values[field.ID] == "" {
			missing = append(missing, field.ID)
		}
	}
	return missing
}

// watcher opens the login page and reports, every second, what the browser has so far.
type watcher interface {
	open(ctx context.Context, params *bridgev2.LoginCookiesParams) error
	poll(ctx context.Context, params *bridgev2.LoginCookiesParams, urlPattern *regexp.Regexp) (pollState, error)
}

type pollState struct {
	values map[string]string
	atURL  bool // a tab is on the page the network waits for (or it waits for none)
	open   bool // a tab is still open
	urls   []string
}

// needsPage says whether a value can only be read from inside the page (local storage, a script), not from cookies.
func needsPage(params *bridgev2.LoginCookiesParams) bool {
	if params.ExtractJS != "" {
		return true
	}
	for _, field := range params.Fields {
		for _, src := range field.Sources {
			if src.Type != bridgev2.LoginCookieTypeCookie {
				return true
			}
		}
	}
	return false
}

// handsOff never attaches to the page: it only asks the browser for its cookies and tabs. Attaching turns on
// debugging inside the page, which bot checks (Meta's) detect, and then their captchas never let you in.
type handsOff struct{}

func (handsOff) open(ctx context.Context, params *bridgev2.LoginCookiesParams) error {
	if len(params.InitialCookies) > 0 {
		cookies := make([]*network.CookieParam, 0, len(params.InitialCookies))
		for _, c := range params.InitialCookies {
			path := c.Path
			if path == "" {
				path = "/"
			}
			cookies = append(cookies, &network.CookieParam{
				Name: c.Name, Value: c.Value, Domain: c.Domain, Path: path, Secure: c.Secure, HTTPOnly: c.HTTPOnly,
			})
		}
		if _, err := chromedp.CallBrowser(ctx, storage.SetCookies, storage.SetCookiesParams{Cookies: cookies}); err != nil {
			return fmt.Errorf("failed to continue the earlier login: %w", err)
		}
	}
	blank, err := chromedp.CallBrowser(ctx, target.GetTargets, target.GetTargetsParams{})
	if err != nil {
		return fmt.Errorf("failed to open the browser: %w", err)
	}
	created, err := chromedp.CallBrowser(ctx, target.CreateTarget, target.CreateTargetParams{URL: params.URL})
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", params.URL, err)
	}
	for _, t := range blank.TargetInfos {
		if t.Type == "page" && t.TargetID != created.TargetID {
			_, _ = chromedp.CallBrowser(ctx, target.CloseTarget, target.CloseTargetParams{TargetID: t.TargetID})
		}
	}
	return nil
}

func (handsOff) poll(ctx context.Context, params *bridgev2.LoginCookiesParams, urlPattern *regexp.Regexp) (pollState, error) {
	tabs, err := chromedp.CallBrowser(ctx, target.GetTargets, target.GetTargetsParams{})
	if err != nil {
		return pollState{}, err
	}
	st := pollState{atURL: urlPattern == nil}
	for _, t := range tabs.TargetInfos {
		if t.Type == "page" {
			st.open = true
			st.urls = append(st.urls, t.URL)
			st.atURL = st.atURL || urlPattern.MatchString(t.URL)
		}
	}
	// storage.GetCookies returns every cookie in the browser, HttpOnly ones included.
	res, err := chromedp.CallBrowser(ctx, storage.GetCookies, storage.GetCookiesParams{})
	if err != nil {
		return pollState{}, err
	}
	st.values = pickCookies(params.Fields, res.Cookies)
	return st, nil
}

// attached drives the page itself, for networks whose values live in local storage or come from a script (Slack).
type attached struct{}

func (attached) open(ctx context.Context, params *bridgev2.LoginCookiesParams) error {
	if _, err := chromedp.Call(ctx, network.Enable, network.EnableParams{}); err != nil {
		return fmt.Errorf("failed to open the browser: %w", err)
	}
	for _, c := range params.InitialCookies {
		path := c.Path
		if path == "" {
			path = "/"
		}
		cookie := network.SetCookieParams{
			Name: c.Name, Value: c.Value, Domain: c.Domain, Path: path, Secure: &c.Secure, HTTPOnly: &c.HTTPOnly,
		}
		if _, err := chromedp.Call(ctx, network.SetCookie, cookie); err != nil {
			return fmt.Errorf("failed to continue the earlier login: %w", err)
		}
	}
	if err := chromedp.Do(ctx, chromedp.Navigate(params.URL)); err != nil {
		return fmt.Errorf("failed to open %s: %w", params.URL, err)
	}
	return nil
}

func (attached) poll(ctx context.Context, params *bridgev2.LoginCookiesParams, urlPattern *regexp.Regexp) (pollState, error) {
	values, err := readFields(ctx, params)
	if err != nil {
		return pollState{}, err
	}
	// Closing its tab ends the browser context, so the loop notices that on its own.
	location, _ := chromedp.Run(ctx, chromedp.Location())
	return pollState{values: values, atURL: urlPattern == nil || urlPattern.MatchString(location), open: true, urls: []string{location}}, nil
}

// readFields collects every value it can find right now: cookies, local storage, and the network's own script.
func readFields(ctx context.Context, params *bridgev2.LoginCookiesParams) (map[string]string, error) {
	// storage.GetCookies returns every cookie in the browser, HttpOnly ones included.
	res, err := chromedp.Call(ctx, storage.GetCookies, storage.GetCookiesParams{})
	if err != nil {
		return nil, err
	}
	values := pickCookies(params.Fields, res.Cookies)
	for _, field := range params.Fields {
		for _, src := range field.Sources {
			if src.Type != bridgev2.LoginCookieTypeLocalStorage || values[field.ID] != "" {
				continue
			}
			script := fmt.Sprintf("localStorage.getItem(%q) || ''", src.Name)
			if value, err := chromedp.Run(ctx, chromedp.Evaluate[string](script)); err == nil && value != "" {
				values[field.ID] = value
			}
		}
	}
	if params.ExtractJS != "" && !hasRequired(params.Fields, values) {
		// The script resolves once it finds its fields; give it a moment each round instead of waiting forever.
		jsCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		extracted, err := chromedp.Run(jsCtx, chromedp.Evaluate[map[string]string](params.ExtractJS, chromedp.EvalAwaitPromise))
		cancel()
		if err == nil {
			for key, value := range extracted {
				if value != "" {
					values[key] = value
				}
			}
		}
	}
	return values, nil
}

// pickCookies matches the browser's cookies to the fields the network asked for.
func pickCookies(fields []bridgev2.LoginCookieField, cookies []*network.Cookie) map[string]string {
	values := make(map[string]string)
	for _, field := range fields {
		for _, src := range field.Sources {
			if src.Type != bridgev2.LoginCookieTypeCookie {
				continue
			}
			for _, c := range cookies {
				if c.Name == src.Name && c.Value != "" && domainMatches(c.Domain, src.CookieDomain) {
					values[field.ID] = c.Value
				}
			}
		}
	}
	return values
}

// domainMatches says whether a cookie for cookieDomain (".www.instagram.com") belongs to want ("instagram.com").
func domainMatches(cookieDomain, want string) bool {
	cookieDomain = strings.TrimPrefix(cookieDomain, ".")
	return want == "" || cookieDomain == want || strings.HasSuffix(cookieDomain, "."+want)
}

func hasRequired(fields []bridgev2.LoginCookieField, values map[string]string) bool {
	for _, field := range fields {
		if field.Required && values[field.ID] == "" {
			return false
		}
	}
	return true
}

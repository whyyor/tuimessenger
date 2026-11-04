package login

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/storage"
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
	"/usr/bin/google-chrome",
	"/usr/bin/chromium",
	"/usr/bin/chromium-browser",
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
	return ""
}

// headless hides the window; only tests use it, a real login needs you to see the page.
var headless = false

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

	opts := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(browser),
		chromedp.UserDataDir(profile),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Flag("headless", headless),
		chromedp.WindowSize(1100, 900),
	}
	if params.UserAgent != "" {
		opts = append(opts, chromedp.UserAgent(params.UserAgent))
	}
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
	defer cancelAlloc()
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()
	browserCtx, cancelTimeout := context.WithTimeout(browserCtx, browserLoginTimeout)
	defer cancelTimeout()

	if _, err = chromedp.Call(browserCtx, network.Enable, network.EnableParams{}); err != nil {
		return nil, fmt.Errorf("failed to open the browser: %w", err)
	}
	for _, c := range params.InitialCookies {
		path := c.Path
		if path == "" {
			path = "/"
		}
		cookie := network.SetCookieParams{
			Name: c.Name, Value: c.Value, Domain: c.Domain, Path: path, Secure: &c.Secure, HTTPOnly: &c.HTTPOnly,
		}
		if _, err = chromedp.Call(browserCtx, network.SetCookie, cookie); err != nil {
			return nil, fmt.Errorf("failed to continue the earlier login: %w", err)
		}
	}
	if err = chromedp.Do(browserCtx, chromedp.Navigate(params.URL)); err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", params.URL, err)
	}
	fmt.Fprintln(out, "Log in in the browser window that opened; it closes by itself once you're logged in.")

	var found map[string]string
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if values, err := readFields(browserCtx, params); err == nil {
			found = values
			if hasRequired(params.Fields, found) && urlMatches(browserCtx, urlPattern) {
				return found, nil
			}
		}
		select {
		case <-browserCtx.Done():
			// Closing the window after logging in still counts, even if the final page wasn't reached.
			if hasRequired(params.Fields, found) {
				return found, nil
			}
			if errors.Is(browserCtx.Err(), context.DeadlineExceeded) {
				return nil, errors.New("gave up waiting for the login after 10 minutes")
			}
			return nil, errors.New("the browser was closed before logging in")
		case <-ticker.C:
		}
	}
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

func urlMatches(ctx context.Context, pattern *regexp.Regexp) bool {
	if pattern == nil {
		return true
	}
	location, err := chromedp.Run(ctx, chromedp.Location())
	return err == nil && pattern.MatchString(location)
}

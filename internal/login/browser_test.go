package login

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"
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

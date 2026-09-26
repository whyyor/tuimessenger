package emoji

import (
	"strings"
	"testing"

	"go.mau.fi/util/variationselector"
)

func TestReplace(t *testing.T) {
	cases := []struct{ in, want string }{
		{"hello :fire: world", "hello " + variationselector.Add("🔥") + " world"},
		{":joy:", variationselector.Add("😂")},
		{"no emoji here", "no emoji here"},
		{":notarealshortcode: stays", ":notarealshortcode: stays"},
		{"https://example.com stays", "https://example.com stays"},
		{"a :: b", "a :: b"},
		{":fire::fire:", variationselector.Add("🔥") + variationselector.Add("🔥")},
		{"ratio 1:2 and 3:4", "ratio 1:2 and 3:4"},
	}
	for _, c := range cases {
		if got := Replace(c.in); got != c.want {
			t.Errorf("Replace(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestComplete(t *testing.T) {
	matches := Complete("fir", 10)
	if len(matches) == 0 {
		t.Fatal("expected matches for 'fir'")
	}
	found := false
	for _, m := range matches {
		if !strings.HasPrefix(m.Shortcode, "fir") {
			t.Errorf("match %q does not have prefix", m.Shortcode)
		}
		if m.Shortcode == "fire" {
			found = true
			if m.Emoji != "🔥" {
				t.Errorf("fire = %q", m.Emoji)
			}
		}
	}
	if !found {
		t.Error("'fire' missing from completions of 'fir'")
	}
}

func TestGet(t *testing.T) {
	if Get("fire") != "🔥" {
		t.Errorf("Get(fire) = %q", Get("fire"))
	}
	if Get("definitely_not_real") != "" {
		t.Error("expected empty for unknown shortcode")
	}
}

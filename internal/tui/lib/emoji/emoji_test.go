package emoji

import (
	"slices"
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

func TestSearchByMeaning(t *testing.T) {
	first := func(q string) Match {
		res := Search(q, 10)
		if len(res) == 0 {
			t.Fatalf("no results for %q", q)
		}
		return res[0]
	}
	if m := first("awk"); m.Emoji != "😬" || m.Keyword != "awk" {
		t.Errorf("awk should find 😬 and say why: %+v", m)
	}
	if m := first("thx"); m.Emoji != "🙏" {
		t.Errorf("thx: %+v", m)
	}
	if m := first("fire"); m.Emoji != "🔥" || m.Keyword != "" {
		t.Errorf("an exact shortcode beats keywords: %+v", m)
	}
	if m := first("fi"); len(m.Shortcode) > 4 {
		t.Errorf("the shortest prefix match should lead, got %q", m.Shortcode)
	}
	found := false
	for _, m := range Search("cringe", 10) {
		found = found || m.Emoji == "😬"
	}
	if !found {
		t.Error("the slang list should map cringe to 😬")
	}
}

func TestCategoriesAndLookup(t *testing.T) {
	cats := Categories()
	if len(cats) != 9 || cats[0].Name != "Smileys & Emotion" || cats[len(cats)-1].Name != "Flags" {
		t.Fatalf("want 9 categories in the standard order, got %d", len(cats))
	}
	for _, c := range cats {
		if c.Name == "Component" {
			t.Error("skin tone swatches should not be a tab")
		}
	}
	if info := Lookup("❤"); info == nil || info.Shortcode != "heart" {
		t.Errorf("lookup should ignore variation selectors: %+v", info)
	}
	if info := Lookup("😬"); info == nil || !slices.Contains(info.Keywords, "awkward") {
		t.Errorf("😬 keywords: %+v", info)
	}
}

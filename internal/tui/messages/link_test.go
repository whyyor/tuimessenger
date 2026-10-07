package messages

import "testing"

func TestFirstURL(t *testing.T) {
	cases := map[string]string{
		// Instagram bridge body: markdown link, the second copy has the query.
		"[https://www.instagram.com/reel/AbCdEfGhIjK/](https://www.instagram.com/reel/AbCdEfGhIjK/?id=394)": "https://www.instagram.com/reel/AbCdEfGhIjK/",
		"look at this https://example.com/a?b=c nice":                                                       "https://example.com/a?b=c",
		"line one\nhttps://example.com/x\nline three":                                                       "https://example.com/x",
		"no link here": "",
	}
	for body, want := range cases {
		if got := FirstURL(body); got != want {
			t.Errorf("FirstURL(%q) = %q, want %q", body, got, want)
		}
	}
}

func TestShortLink(t *testing.T) {
	got := shortLink("https://www.instagram.com/reel/AbCdEfGhIjK/?id=1000000000000000000_10000000000")
	if want := "instagram.com/reel/AbCdEfGhIjK"; got != want {
		t.Errorf("shortLink = %q, want %q", got, want)
	}
}

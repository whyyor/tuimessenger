package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/lithammer/fuzzysearch/fuzzy"
)

func TestMatchTier(t *testing.T) {
	cases := []struct {
		name, query string
		want        int
	}{
		{"telegram saved messages", "tel", 0},
		{"airtel finance", "fin", 1},
		{"nadia, sabrina lopez", "sab", 1},
		{"airtel", "tel", 2},
		{"optimum nutrition india", "mum", 2},
		{"rosalie", "sa", 2}, // ro-sa-lie: "sa" really is a substring
		{"oscar", "sa", 3}, // s then a, but not adjacent
		{"mariana", "ni", 3},
	}
	for _, c := range cases {
		if got := matchTier(c.name, c.query); got != c.want {
			t.Errorf("matchTier(%q, %q) = %d, want %d", c.name, c.query, got, c.want)
		}
	}
}

// ranking must prefer strong matches, and among equally strong ones the newest.
func TestSortRoomMatches(t *testing.T) {
	names := []string{
		"Airtel",                  // substring        -> tier 2
		"Telegram",                // prefix, old      -> tier 0
		"The Legends",             // t..e..l scattered -> tier 3
		"Telegram Saved Messages", // prefix, new      -> tier 0
		"Airtel Finance",          // substring        -> tier 2
	}
	days := []int{1, 300, 2, 0, 5} // days ago
	now := time.Now()
	lower := make([]string, len(names))
	recency := make([]time.Time, len(names))
	for i, n := range names {
		lower[i] = strings.ToLower(n)
		recency[i] = now.AddDate(0, 0, -days[i])
	}

	matches := fuzzy.RankFindFold("tel", names)
	sortRoomMatches(matches, "tel", lower, recency)

	got := make([]string, len(matches))
	for i, m := range matches {
		got[i] = m.Target
	}
	want := []string{
		"Telegram Saved Messages", // tier 0, newest
		"Telegram",                // tier 0, 300 days old
		"Airtel",                  // tier 2, 1 day
		"Airtel Finance",          // tier 2, 5 days
		"The Legends",             // tier 3
	}
	if len(got) != len(want) {
		t.Fatalf("got %d matches %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d: got %q, want %q (full order %v)", i, got[i], want[i], got)
		}
	}
}

// a stale exact-prefix room must not outrank a recent one, which was the
// original bug: Levenshtein ordering made short old names win.
func TestSortRoomMatchesRecencyWithinTier(t *testing.T) {
	names := []string{"Sattu", "Sanya"}
	lower := []string{"sattu", "sanya"}
	now := time.Now()
	recency := []time.Time{now.AddDate(-2, 0, 0), now} // Sattu 2 years old

	matches := fuzzy.RankFindFold("sa", names)
	sortRoomMatches(matches, "sa", lower, recency)

	if matches[0].Target != "Sanya" {
		t.Errorf("expected recent Sanya first, got %q", matches[0].Target)
	}
}

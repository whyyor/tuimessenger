package tui

import (
	"sort"
	"strings"
	"time"

	"github.com/lithammer/fuzzysearch/fuzzy"
)

// matchTier grades how strongly a case-folded name matches a case-folded query.
// Lower is better. fuzzysearch only reports *whether* a scattered subsequence
// matched, so prefix and substring hits have to be graded here.
func matchTier(lowerName, lowerQuery string) int {
	switch {
	case strings.HasPrefix(lowerName, lowerQuery):
		return 0
	case hasWordPrefix(lowerName, lowerQuery):
		return 1
	case strings.Contains(lowerName, lowerQuery):
		return 2
	default:
		return 3
	}
}

func hasWordPrefix(name, query string) bool {
	for i := 1; i < len(name); i++ {
		if isWordSep(name[i-1]) && strings.HasPrefix(name[i:], query) {
			return true
		}
	}
	return false
}

func isWordSep(c byte) bool {
	switch c {
	case ' ', '\t', ',', '.', '-', '_', '/', ':', '(', '[', '+', '&', '\'', '"', '|':
		return true
	}
	return false
}

// sortRoomMatches orders matches by match quality, then most-recent-first.
// fuzzy.Ranks.Less sorts purely on Levenshtein distance against the whole name,
// which degenerates into "shortest name wins" and ignores recency completely.
func sortRoomMatches(matches fuzzy.Ranks, query string, lowerNames []string, recency []time.Time) {
	lowerQuery := strings.ToLower(query)
	type scored struct {
		rank fuzzy.Rank
		tier int
	}
	scoredMatches := make([]scored, len(matches))
	for i, match := range matches {
		scoredMatches[i] = scored{match, matchTier(lowerNames[match.OriginalIndex], lowerQuery)}
	}
	sort.SliceStable(scoredMatches, func(a, b int) bool {
		x, y := scoredMatches[a], scoredMatches[b]
		if x.tier != y.tier {
			return x.tier < y.tier
		}
		return recency[x.rank.OriginalIndex].After(recency[y.rank.OriginalIndex])
	})
	for i, s := range scoredMatches {
		matches[i] = s.rank
	}
}

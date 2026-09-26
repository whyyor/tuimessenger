// Package emoji resolves :shortcode: tokens to unicode emoji, using the same
// dataset the web client's picker ships (data.json is a copy of
// web/src/util/emoji/data.json).
package emoji

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strings"
	"sync"

	"go.mau.fi/util/variationselector"
)

//go:embed data.json
var rawData []byte

type entry struct {
	Unicode    string   `json:"u"`
	Name       string   `json:"n"`
	Shortcodes []string `json:"s"`
}

var (
	once        sync.Once
	byShortcode map[string]string
	shortcodes  []string
)

func load() {
	once.Do(func() {
		var data struct {
			Emojis []entry `json:"e"`
		}
		if err := json.Unmarshal(rawData, &data); err != nil {
			byShortcode = map[string]string{}
			return
		}
		byShortcode = make(map[string]string, len(data.Emojis)*2)
		for _, e := range data.Emojis {
			for _, code := range append([]string{e.Name}, e.Shortcodes...) {
				code = strings.ToLower(code)
				if code == "" {
					continue
				}
				if _, exists := byShortcode[code]; !exists {
					byShortcode[code] = e.Unicode
				}
			}
		}
		shortcodes = make([]string, 0, len(byShortcode))
		for code := range byShortcode {
			shortcodes = append(shortcodes, code)
		}
		sort.Strings(shortcodes)
	})
}

// Get returns the emoji for a shortcode (without colons), or "".
func Get(shortcode string) string {
	load()
	return byShortcode[strings.ToLower(shortcode)]
}

// Match is one completion candidate.
type Match struct {
	Shortcode string
	Emoji     string
}

// Complete returns up to limit shortcodes starting with the given prefix.
func Complete(prefix string, limit int) []Match {
	load()
	prefix = strings.ToLower(prefix)
	var matches []Match
	idx := sort.SearchStrings(shortcodes, prefix)
	for ; idx < len(shortcodes) && len(matches) < limit; idx++ {
		if !strings.HasPrefix(shortcodes[idx], prefix) {
			break
		}
		matches = append(matches, Match{shortcodes[idx], byShortcode[shortcodes[idx]]})
	}
	return matches
}

func isShortcodeChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
		c == '_' || c == '+' || c == '-'
}

// IsValidToken reports whether s could be the start of a shortcode: non-empty
// and made only of shortcode characters.
func IsValidToken(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isShortcodeChar(s[i]) {
			return false
		}
	}
	return true
}

// Replace substitutes every :shortcode: in text with its emoji, with the
// variation selectors Matrix expects. Unknown shortcodes are left alone.
func Replace(text string) string {
	load()
	var out strings.Builder
	for {
		start := strings.IndexByte(text, ':')
		if start == -1 {
			break
		}
		end := start + 1
		for end < len(text) && isShortcodeChar(text[end]) {
			end++
		}
		if end < len(text) && text[end] == ':' && end > start+1 {
			if emoji := byShortcode[strings.ToLower(text[start+1:end])]; emoji != "" {
				out.WriteString(text[:start])
				out.WriteString(variationselector.Add(emoji))
				text = text[end+1:]
				continue
			}
		}
		out.WriteString(text[:end])
		text = text[end:]
	}
	out.WriteString(text)
	return out.String()
}

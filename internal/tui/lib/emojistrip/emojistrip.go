package emojistrip

import (
	"strings"
	"unicode"

	"github.com/rivo/uniseg"
)

// combining enclosing keycap; leads sequences like 1️⃣ whose first rune is plain ASCII
const keycap = 0x20E3

var pictographic = &unicode.RangeTable{
	R16: []unicode.Range16{
		{0x203C, 0x2049, 0x0D}, // ‼ ⁉
		{0x2122, 0x2122, 1},    // ™
		{0x2139, 0x2139, 1},    // ℹ
		{0x2194, 0x21AA, 1},    // arrows
		{0x231A, 0x231B, 1},    // ⌚ ⌛
		{0x2328, 0x2328, 1},    // ⌨
		{0x23CF, 0x23FA, 1},    // media controls, ⏰
		{0x24C2, 0x24C2, 1},    // Ⓜ
		{0x25AA, 0x25FE, 1},    // ▪ ◻ ◾
		{0x2600, 0x27BF, 1},    // misc symbols + dingbats
		{0x2934, 0x2935, 1},    // ⤴ ⤵
		{0x2B00, 0x2BFF, 1},    // ⬅ ⭐ ⭕
		{0x3030, 0x3030, 1},    // 〰
		{0x303D, 0x303D, 1},    // 〽
		{0x3297, 0x3299, 2},    // ㊗ ㊙
		{0xFE0F, 0xFE0F, 1},    // stray variation selector
	},
	R32: []unicode.Range32{
		{0x1F000, 0x1F2FF, 1}, // mahjong, cards, regional indicators (flags)
		{0x1F300, 0x1F5FF, 1}, // misc pictographs
		{0x1F600, 0x1F64F, 1}, // emoticons
		{0x1F650, 0x1F67F, 1}, // ornamental dingbats
		{0x1F680, 0x1F6FF, 1}, // transport
		{0x1F700, 0x1F7FF, 1}, // alchemical, geometric ext
		{0x1F800, 0x1F8FF, 1}, // supplemental arrows-C
		{0x1F900, 0x1F9FF, 1}, // supplemental pictographs
		{0x1FA00, 0x1FAFF, 1}, // chess, symbols ext-A
		{0x1FB00, 0x1FBFF, 1}, // legacy computing
		{0xE0020, 0xE007F, 1}, // tag chars in flag sequences
	},
}

// Strip drops emoji grapheme clusters and collapses the whitespace they leave behind.
func Strip(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	graphemes := uniseg.NewGraphemes(s)
	for graphemes.Next() {
		if isEmoji(graphemes.Runes()) {
			continue
		}
		b.WriteString(graphemes.Str())
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func isEmoji(runes []rune) bool {
	if len(runes) == 0 {
		return false
	}
	for _, r := range runes {
		if r == keycap {
			return true
		}
	}
	return unicode.Is(pictographic, runes[0])
}

package emojistrip

import "testing"

func TestStrip(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Alice", "Alice"},
		{"🎉 Rahul", "Rahul"},
		{"Rahul 🎉 Sharma", "Rahul Sharma"},
		{"Rahul🎉", "Rahul"},
		{"👨‍👩‍👧 Family Chat", "Family Chat"},
		{"Anna 👩🏽‍💻", "Anna"},
		{"🇮🇳 India Room", "India Room"},
		{"1️⃣ First", "First"},
		{"❤️ Love", "Love"},
		{"🎉🎉🎉", ""},
		{"", ""},
		{"Team #1", "Team #1"},
		{"José Ω 漢字", "José Ω 漢字"},
		{"a  b", "a b"},
	}
	for _, c := range cases {
		if got := Strip(c.in); got != c.want {
			t.Errorf("Strip(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

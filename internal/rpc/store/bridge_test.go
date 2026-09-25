package store

import (
	"testing"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/database"
)

func metaWithHeroes(heroes ...string) *database.Room {
	ids := make([]id.UserID, len(heroes))
	for i, h := range heroes {
		ids[i] = id.UserID(h)
	}
	return &database.Room{LazyLoadSummary: &mautrix.LazyLoadSummary{Heroes: ids}}
}

func TestBridgeIDFromMeta(t *testing.T) {
	cases := []struct {
		name string
		meta *database.Room
		want string
	}{
		{"whatsapp phone", metaWithHeroes("@whatsapp_15550100001:beeper.local"), "whatsapp"},
		{"whatsapp lid", metaWithHeroes("@whatsapp_lid-100000000000001:beeper.local"), "whatsapp"},
		{"telegram", metaWithHeroes("@telegram_100000001:beeper.local"), "telegram"},
		{"slackgo", metaWithHeroes("@slackgo_t0000000000-uslackbot:beeper.local"), "slackgo"},
		{"instagramgo", metaWithHeroes("@instagramgo_100000000000002:beeper.local"), "instagramgo"},
		{"whatsapp bot room", metaWithHeroes("@whatsappbot:beeper.local"), "whatsapp"},
		{"telegram bot room", metaWithHeroes("@telegrambot:beeper.local"), "telegram"},
		{"slackgo bot room", metaWithHeroes("@slackgobot:beeper.local"), "slackgo"},
		{"native beeper staff", metaWithHeroes("@brad:beeper.com"), ""},
		{"native help", metaWithHeroes("@help:beeper.com"), ""},
		{"empty heroes", metaWithHeroes(), ""},
		{"nil summary", &database.Room{}, ""},
		{"nil meta", nil, ""},
	}
	for _, c := range cases {
		if got := bridgeIDFromMeta(c.meta); got != c.want {
			t.Errorf("%s: bridgeIDFromMeta() = %q, want %q", c.name, got, c.want)
		}
	}
}

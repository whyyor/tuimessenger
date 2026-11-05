package tui

import (
	"testing"

	"maunium.net/go/mautrix/id"
)

func TestMutedRooms(t *testing.T) {
	rules := `{"global":{"room":[
		{"rule_id":"!muted:beeper.local","enabled":true,"default":false,"actions":[]},
		{"rule_id":"!loud:beeper.local","enabled":true,"actions":["notify",{"set_tweak":"sound","value":"default"}]},
		{"rule_id":"!disabled:beeper.local","enabled":false,"actions":[]},
		{"rule_id":"!dontnotify:beeper.local","enabled":true,"actions":["dont_notify"]}
	]}}`
	got := mutedRooms([]byte(rules))
	want := map[string]bool{"!muted:beeper.local": true, "!dontnotify:beeper.local": true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for room := range want {
		if !got[id.RoomID(room)] {
			t.Errorf("%s should be muted", room)
		}
	}
	if mutedRooms([]byte("not json")) != nil {
		t.Error("bad JSON should give no muted rooms")
	}
}

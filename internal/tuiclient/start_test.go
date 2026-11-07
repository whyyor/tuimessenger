package tuiclient

import "testing"

func TestCanStartHere(t *testing.T) {
	const local = "http://127.0.0.1:8008"
	for _, c := range []struct {
		mode      string
		installed bool
		base      string
		want      bool
	}{
		{"complete", false, local, true},            // backend inside the TUI
		{"complete", true, local + "/", true},       // the service
		{"", true, local, true},                     // set up before modes, with the service
		{"", false, local, false},                   // nothing to start: say so instead of hanging
		{"self-hosted", true, local, false},         // never for a self-hosted setup
		{"complete", true, "http://pi:8008", false}, // a backend elsewhere
	} {
		if got := canStartHere(c.mode, c.installed, c.base, local); got != c.want {
			t.Errorf("mode %q, service %v, %s: got %v", c.mode, c.installed, c.base, got)
		}
	}
}

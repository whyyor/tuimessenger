package tui

import "testing"

func TestNetworkName(t *testing.T) {
	for in, want := range map[string]string{"whatsapp": "WhatsApp", "slack": "Slack", "linkedin": "LinkedIn", "newnet": "Newnet", "": ""} {
		if got := networkName(in); got != want {
			t.Errorf("networkName(%q) = %q, want %q", in, got, want)
		}
	}
}

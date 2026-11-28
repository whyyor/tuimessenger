package bridges

import (
	"testing"

	facebook "go.mau.fi/mautrix-meta/pkg/connector"
	instagram "go.mau.fi/mautrix-meta/pkg/igconnector"
	whatsapp "go.mau.fi/mautrix-whatsapp/pkg/connector"

	"github.com/whyyor/tuimessenger/internal/config"
)

func TestLinkedDeviceOS(t *testing.T) {
	for goos, want := range map[string]string{"darwin": "Mac OS", "linux": "Linux", "windows": "Windows", "freebsd": "Linux"} {
		if got := linkedDeviceOS(goos); got != want {
			t.Errorf("%s: %q, want %q", goos, got, want)
		}
	}
}

func TestNetworkDefaults(t *testing.T) {
	wa, err := New("whatsapp", &config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	waCfg := wa.Connector.(*whatsapp.WhatsAppConnector).Config
	if waCfg.OSName == "tuimessenger" || waCfg.OSName == "" || waCfg.BrowserName != "chrome" {
		t.Errorf("WhatsApp linked device should look like a web session on this OS: %q / %q", waCfg.OSName, waCfg.BrowserName)
	}
	ig, err := New("instagram", &config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	igCfg := ig.Connector.(*instagram.IGConnector).Config
	if igCfg.ThreadBackfill.BatchCount != 0 {
		t.Errorf("Instagram should sync only the recent chats, like Beeper: %+v", igCfg.ThreadBackfill)
	}
}

func TestMessengerNetwork(t *testing.T) {
	fb, err := New("facebook", &config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	connector := fb.Connector.(*facebook.MetaConnector)
	if connector.Config.ThreadBackfill.BatchCount != 0 {
		t.Errorf("Messenger should sync only the recent chats, like Beeper: %+v", connector.Config.ThreadBackfill)
	}
	if !connector.Config.CacheConnectionState {
		t.Error("Expected connector cache to be true")
	}
	if got := fb.Config.CommandPrefix; got != "!fb" {
		t.Errorf("Got wrong command prefix %q", got)
	}
	if got := connector.GetName().NetworkID; got != "facebook" {
		t.Errorf("Got wrong network name %q", got)
	}
}

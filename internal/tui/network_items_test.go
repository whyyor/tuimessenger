package tui

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"testing"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/rpc"
	"github.com/whyyor/tuimessenger/internal/rpc/client"
)

// fakeNetworkBackend is a backend that reports networks; the embedded Commands fills in the rest of the interface.
type fakeNetworkBackend struct {
	*rpc.Commands
	networks []client.Network
}

func (f *fakeNetworkBackend) Run(ctx context.Context, _ rpc.EventHandler, _ rpc.ConnStateHandler) error {
	<-ctx.Done()
	return ctx.Err()
}
func (f *fakeNetworkBackend) Resync()     {}
func (f *fakeNetworkBackend) Disconnect() {}
func (f *fakeNetworkBackend) UploadMedia(context.Context, string, bool, io.Reader) (*event.MessageEventContent, error) {
	return nil, nil
}
func (f *fakeNetworkBackend) DownloadMedia(context.Context, rpc.DownloadMediaParams) (*http.Response, error) {
	return nil, nil
}
func (f *fakeNetworkBackend) MediaURL(id.ContentURI, url.Values) string { return "" }

func (f *fakeNetworkBackend) Networks(context.Context) ([]client.Network, error) {
	return f.networks, nil
}

func (f *fakeNetworkBackend) SetNetworkSetting(context.Context, string, string, bool) error {
	return nil
}

func TestNetworkItems(t *testing.T) {
	titles := func(networks ...client.Network) []string {
		backend := &fakeNetworkBackend{Commands: rpc.New(nil), networks: networks}
		p := &CommandPalette{parent: &MainView{matrix: client.New(backend)}}
		var out []string
		for _, item := range p.networkItems() {
			out = append(out, item.title)
		}
		return out
	}
	slackOn := client.Network{Name: "slack", State: "CONNECTED", Settings: map[string]bool{"include_channels": true}}
	whatsapp := client.Network{Name: "whatsapp", State: "CONNECTED", Settings: map[string]bool{}}
	if got := titles(slackOn, whatsapp); len(got) != 1 || got[0] != "Slack channels: on → off" {
		t.Errorf("got %q", got)
	}
	slackOff := slackOn
	slackOff.Settings = map[string]bool{"include_channels": false}
	if got := titles(slackOff); len(got) != 1 || got[0] != "Slack channels: off → on" {
		t.Errorf("got %q", got)
	}
	slackDown := slackOn
	slackDown.State = "TRANSIENT_DISCONNECT"
	if got := titles(slackDown); len(got) != 0 {
		t.Errorf("a disconnected network shouldn't offer its settings: %q", got)
	}
	if got := (&CommandPalette{parent: &MainView{}}).networkItems(); got != nil {
		t.Errorf("no client: got %v", got)
	}
}

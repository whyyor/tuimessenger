package tuiclient

import (
	"context"
	"net/http"

	"maunium.net/go/mautrix"

	"github.com/whyyor/tuimessenger/internal/rpc/client"
)

var _ client.NetworkSettings = (*Backend)(nil)

// Networks asks the backend which chat networks run, how they're connected, and their settings.
func (b *Backend) Networks(ctx context.Context) ([]client.Network, error) {
	var resp struct {
		Networks []client.Network `json:"networks"`
	}
	url := b.hc.Client.BuildURL(mautrix.ClientURLPath{"unstable", "com.tuimessenger", "networks"})
	if _, err := b.hc.Client.MakeRequest(ctx, http.MethodGet, url, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Networks, nil
}

// SetNetworkSetting changes one on/off setting of a network; the backend saves it and applies it right away.
func (b *Backend) SetNetworkSetting(ctx context.Context, network, key string, value bool) error {
	url := b.hc.Client.BuildURL(mautrix.ClientURLPath{"unstable", "com.tuimessenger", "networks", network, "settings"})
	_, err := b.hc.Client.MakeRequest(ctx, http.MethodPut, url, map[string]bool{key: value}, nil)
	return err
}

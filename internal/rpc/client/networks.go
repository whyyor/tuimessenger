package client

import "context"

// Network is one chat network of the backend, e.g. Slack, with its connection state and on/off settings.
type Network struct {
	Name     string          `json:"name"`
	State    string          `json:"state"`
	Settings map[string]bool `json:"settings"`
}

// NetworkSettings is implemented by backends whose networks can be listed and changed (tuimessenger's).
type NetworkSettings interface {
	Networks(ctx context.Context) ([]Network, error)
	SetNetworkSetting(ctx context.Context, network, key string, value bool) error
}

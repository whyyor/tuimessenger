package csapi

import (
	"errors"
	"net/http"

	"maunium.net/go/mautrix"
)

// Clients list the chat networks and change their on/off settings (e.g. Slack channels) here.
const (
	networksFeature = "com.tuimessenger.networks"
	networksPath    = "/_matrix/client/unstable/com.tuimessenger/networks"
)

// ErrUnknownSetting means the network doesn't exist here or has no such setting.
var ErrUnknownSetting = errors.New("unknown network or setting")

// Network is one chat network as clients see it.
type Network struct {
	Name string `json:"name"`
	// State is the connection state, e.g. CONNECTED or TRANSIENT_DISCONNECT; empty when not logged in.
	State    string          `json:"state"`
	Settings map[string]bool `json:"settings"`
}

type respNetworks struct {
	Networks []Network `json:"networks"`
}

func (s *Server) networks(r *http.Request) (any, error) {
	return &respNetworks{Networks: s.Bridges.Networks(r.Context())}, nil
}

// setNetworkSettings takes {"include_channels": false}: each key is saved to config.yaml and applied.
func (s *Server) setNetworkSettings(r *http.Request) (any, error) {
	var req map[string]bool
	if err := readJSON(r, &req); err != nil {
		return nil, err
	}
	network := r.PathValue("network")
	for key, value := range req {
		err := s.Bridges.SetNetworkSetting(r.Context(), network, key, value)
		if errors.Is(err, ErrUnknownSetting) {
			return nil, mautrix.MNotFound.WithMessage(err.Error())
		} else if err != nil {
			return nil, err
		}
		s.Log.Info().Str("network", network).Str("setting", key).Bool("value", value).Msg("Client changed a network setting")
	}
	return struct{}{}, nil
}

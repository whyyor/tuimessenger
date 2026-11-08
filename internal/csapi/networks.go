package csapi

import (
	"errors"
	"net/http"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
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
	// Running is false for a network that can't start yet; Setup then says what it needs (e.g. telegram_api_keys).
	Running bool   `json:"running"`
	Setup   string `json:"setup,omitempty"`
	// Logins are the network's accounts (at most one); Flows are the ways to log in.
	Logins []NetworkLogin       `json:"logins,omitempty"`
	Flows  []bridgev2.LoginFlow `json:"flows,omitempty"`
}

// NetworkLogin is a logged-in account of a network.
type NetworkLogin struct {
	ID      networkid.UserLoginID `json:"id"`
	Name    string                `json:"name"`    // e.g. the phone number or username
	State   string                `json:"state"`   // e.g. CONNECTED, BAD_CREDENTIALS
	Message string                `json:"message"` // the network's explanation of a bad state
}

// SetupTelegramKeys means Telegram needs the user's own app keys before it can start.
const SetupTelegramKeys = "telegram_api_keys"

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

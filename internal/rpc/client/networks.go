package client

import (
	"context"

	"maunium.net/go/mautrix/bridgev2"
)

// Network is one chat network of the backend, e.g. Slack, with its connection state, settings and accounts.
type Network struct {
	Name     string          `json:"name"`
	State    string          `json:"state"`
	Settings map[string]bool `json:"settings"`
	// Running is false for a network that can't start yet; Setup says what it needs (SetupTelegramKeys).
	Running bool                 `json:"running"`
	Setup   string               `json:"setup,omitempty"`
	Logins  []NetworkLogin       `json:"logins,omitempty"`
	Flows   []bridgev2.LoginFlow `json:"flows,omitempty"`
}

// NetworkLogin is a logged-in account of a network.
type NetworkLogin struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	State   string `json:"state"`
	Message string `json:"message"`
}

// SetupTelegramKeys: Telegram needs the user's own app keys (my.telegram.org) before it can start.
const SetupTelegramKeys = "telegram_api_keys"

// LoginStep is a login in progress and what it needs next.
type LoginStep struct {
	LoginID string              `json:"login_id"`
	Step    *bridgev2.LoginStep `json:"step"`
}

// NetworkSettings is implemented by backends whose networks can be listed and changed (tuimessenger's).
type NetworkSettings interface {
	Networks(ctx context.Context) ([]Network, error)
	SetNetworkSetting(ctx context.Context, network, key string, value bool) error
}

// NetworkLogins is implemented by backends that can log in to networks from the client.
type NetworkLogins interface {
	NetworkSettings
	StartLogin(ctx context.Context, network, flowID string) (*LoginStep, error)
	// SubmitLogin answers the current step; input is nil for a step that only waits (a QR code to scan).
	SubmitLogin(ctx context.Context, loginID string, step *bridgev2.LoginStep, input map[string]string) (*LoginStep, error)
	CancelLogin(ctx context.Context, loginID string) error
	LogoutNetwork(ctx context.Context, network, loginID string) error
	SetTelegramKeys(ctx context.Context, apiID int, apiHash string) error
}

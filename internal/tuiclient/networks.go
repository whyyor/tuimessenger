package tuiclient

import (
	"context"
	"net/http"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"

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

var _ client.NetworkLogins = (*Backend)(nil)

func (b *Backend) unstableURL(path ...any) string {
	return b.hc.Client.BuildURL(append(mautrix.ClientURLPath{"unstable", "com.tuimessenger"}, path...))
}

// noRetry: a retried login step could submit a code twice.
func noRetry(ctx context.Context) context.Context { return mautrix.WithMaxRetries(ctx, 0) }

func (b *Backend) StartLogin(ctx context.Context, network, flowID string) (*client.LoginStep, error) {
	var resp client.LoginStep
	_, err := b.hc.Client.MakeRequest(noRetry(ctx), http.MethodPost, b.unstableURL("networks", network, "login", flowID), struct{}{}, &resp)
	return &resp, err
}

func (b *Backend) SubmitLogin(ctx context.Context, loginID string, step *bridgev2.LoginStep, input map[string]string) (*client.LoginStep, error) {
	if input == nil {
		input = map[string]string{}
	}
	var resp client.LoginStep
	url := b.unstableURL("login", loginID, "step", step.StepID, string(step.Type))
	_, err := b.hc.Client.MakeRequest(noRetry(ctx), http.MethodPost, url, input, &resp)
	return &resp, err
}

func (b *Backend) CancelLogin(ctx context.Context, loginID string) error {
	_, err := b.hc.Client.MakeRequest(noRetry(ctx), http.MethodPost, b.unstableURL("login", loginID, "cancel"), struct{}{}, nil)
	return err
}

func (b *Backend) LogoutNetwork(ctx context.Context, network, loginID string) error {
	_, err := b.hc.Client.MakeRequest(noRetry(ctx), http.MethodPost, b.unstableURL("networks", network, "logout", loginID), struct{}{}, nil)
	return err
}

func (b *Backend) SetTelegramKeys(ctx context.Context, apiID int, apiHash string) error {
	body := map[string]any{"api_id": apiID, "api_hash": apiHash}
	_, err := b.hc.Client.MakeRequest(noRetry(ctx), http.MethodPut, b.unstableURL("networks", "telegram", "keys"), body, nil)
	return err
}

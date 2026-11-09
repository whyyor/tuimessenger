package app

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"

	"maunium.net/go/mautrix/bridgev2"

	"github.com/whyyor/tuimessenger/internal/config"
	"github.com/whyyor/tuimessenger/internal/csapi"
)

// Networks lists the running networks with their accounts, login flows and settings, plus Telegram when it's wanted
// but can't run yet because its app keys are missing.
func (a *App) Networks(ctx context.Context) []csapi.Network {
	a.settingsLock.Lock()
	defer a.settingsLock.Unlock()
	networks := make([]csapi.Network, 0, len(a.bridges))
	for _, br := range a.bridges {
		n := csapi.Network{Name: string(br.ID), Settings: map[string]bool{}, Running: true, Flows: offeredFlows(string(br.ID), br.Network.GetLoginFlows())}
		for _, login := range br.GetAllCachedUserLogins() {
			state := login.BridgeState.GetPrev()
			n.State = string(state.StateEvent)
			n.Logins = append(n.Logins, csapi.NetworkLogin{
				ID: login.ID, Name: login.RemoteName, State: string(state.StateEvent), Message: state.Message,
			})
		}
		for key, setting := range a.settings[n.Name] {
			n.Settings[key] = setting.Get()
		}
		networks = append(networks, n)
	}
	if a.opts.OfferTelegramSetup && a.Bridge("telegram") == nil {
		networks = append(networks, csapi.Network{Name: "telegram", Settings: map[string]bool{}, Setup: csapi.SetupTelegramKeys})
	}
	return networks
}

// SetTelegramKeys saves Telegram's app keys to config.yaml; the Telegram bridge reads them when the backend starts.
func (a *App) SetTelegramKeys(ctx context.Context, apiID int, apiHash string) error {
	path := filepath.Join(a.opts.DataDir, "config.yaml")
	if err := config.SetInt(path, "telegram", "api_id", apiID); err != nil {
		return fmt.Errorf("failed to save the keys: %w", err)
	}
	return config.SetString(path, "telegram", "api_hash", apiHash)
}

// SetNetworkSetting saves a setting to config.yaml, then puts it into effect. If saving fails, nothing changes.
func (a *App) SetNetworkSetting(ctx context.Context, network, key string, value bool) error {
	a.settingsLock.Lock()
	defer a.settingsLock.Unlock()
	setting := a.settings[network][key]
	br := a.Bridge(network)
	if setting == nil || br == nil {
		return fmt.Errorf("%w: %s.%s", csapi.ErrUnknownSetting, network, key)
	}
	if setting.Get() == value {
		return nil
	}
	if err := config.SetBool(filepath.Join(a.opts.DataDir, "config.yaml"), network, key, value); err != nil {
		return fmt.Errorf("failed to save the setting: %w", err)
	}
	return setting.Apply(ctx, br, value)
}

// hiddenLoginFlows are ways to log in that aren't offered: they didn't work in practice or don't fit a personal app.
var hiddenLoginFlows = map[string][]string{
	"instagram": {"android"}, // "Native Android": failed in testing; the web ways work
	"telegram":  {"bot"},     // logs in as a BotFather bot, not as you
}

func loginFlowHidden(network, flowID string) bool {
	return slices.Contains(hiddenLoginFlows[network], flowID)
}

// offeredFlows is a network's login flows without the hidden ones.
func offeredFlows(network string, flows []bridgev2.LoginFlow) []bridgev2.LoginFlow {
	return slices.DeleteFunc(slices.Clone(flows), func(f bridgev2.LoginFlow) bool { return loginFlowHidden(network, f.ID) })
}

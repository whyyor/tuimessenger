package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"maunium.net/go/mautrix/bridgev2"

	"github.com/whyyor/tuimessenger/internal/bridges"
	"github.com/whyyor/tuimessenger/internal/config"
	"github.com/whyyor/tuimessenger/internal/csapi"
)

// Networks lists the running networks with their accounts, login flows and settings, then the ones that aren't
// running: Setup says how to start them (turn on, or Telegram's app keys first).
func (a *App) Networks(ctx context.Context) []csapi.Network {
	a.settingsLock.Lock()
	defer a.settingsLock.Unlock()
	running := a.allBridges()
	networks := make([]csapi.Network, 0, len(running))
	for _, br := range running {
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
	// The other compiled-in networks can be turned on from the client (EnableNetwork).
	for _, name := range bridges.Available() {
		if a.Bridge(name) != nil || name == "dummy" {
			continue
		}
		setup := csapi.SetupTurnOn
		if name == "telegram" && !a.opts.Config.HasTelegramKeys() {
			setup = csapi.SetupTelegramKeys
		}
		networks = append(networks, csapi.Network{Name: name, Settings: map[string]bool{}, Setup: setup})
	}
	return networks
}

// EnableNetwork turns a network on while everything runs: it's started, then added to networks: in config.yaml so it
// starts again next time. Turning on a running network does nothing.
func (a *App) EnableNetwork(ctx context.Context, name string) error {
	a.enableLock.Lock()
	defer a.enableLock.Unlock()
	if a.Bridge(name) != nil {
		return nil
	}
	if !slices.Contains(bridges.Available(), name) {
		return fmt.Errorf("%w: %s", csapi.ErrUnknownNetwork, name)
	}
	if name == "telegram" && !a.opts.Config.HasTelegramKeys() {
		return errors.New("telegram needs its app keys first")
	}
	// Starting can take a few seconds (database setup); a client that stops waiting mustn't cut it off halfway.
	ctx = context.WithoutCancel(ctx)
	nb, err := a.newBridge(name)
	if err != nil {
		return err
	}
	if err = a.startBridge(ctx, nb.br, nb.afterStart); err != nil {
		nb.br.StopWithTimeout(5 * time.Second)
		_ = nb.db.Close()
		return err
	}
	a.publish(nb) // only now: until it has started, nothing else may touch its database
	// Saved only once it runs: a network in config.yaml that can't start would stop the next start altogether.
	wanted := a.opts.Config.WithNetwork(name)
	if err = config.SetStrings(filepath.Join(a.opts.DataDir, "config.yaml"), "", "networks", wanted); err != nil {
		return fmt.Errorf("%s is on, but couldn't be saved for next time: %w", name, err)
	}
	a.opts.Config.Networks = wanted
	return nil
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
	// messenger.com: in testing the login ended on facebook.com and never finished. iOS: poses as an old Messenger Lite
	// iPhone app. facebook.com stays (the safest), and Android as the way in without a browser.
	"facebook": {"messenger", "messenger-lite"},
	"telegram": {"bot"}, // logs in as a BotFather bot, not as you
}

func loginFlowHidden(network, flowID string) bool {
	return slices.Contains(hiddenLoginFlows[network], flowID)
}

// offeredFlows is a network's login flows without the hidden ones.
func offeredFlows(network string, flows []bridgev2.LoginFlow) []bridgev2.LoginFlow {
	return slices.DeleteFunc(slices.Clone(flows), func(f bridgev2.LoginFlow) bool { return loginFlowHidden(network, f.ID) })
}

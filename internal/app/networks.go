package app

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/whyyor/tuimessenger/internal/config"
	"github.com/whyyor/tuimessenger/internal/csapi"
)

// Networks lists the running networks, with their connection state and changeable settings.
func (a *App) Networks(ctx context.Context) []csapi.Network {
	a.settingsLock.Lock()
	defer a.settingsLock.Unlock()
	networks := make([]csapi.Network, 0, len(a.bridges))
	for _, br := range a.bridges {
		n := csapi.Network{Name: string(br.ID), Settings: map[string]bool{}}
		for _, login := range br.GetAllCachedUserLogins() {
			n.State = string(login.BridgeState.GetPrev().StateEvent)
		}
		for key, setting := range a.settings[n.Name] {
			n.Settings[key] = setting.Get()
		}
		networks = append(networks, n)
	}
	return networks
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

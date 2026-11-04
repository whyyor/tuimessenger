package bridges

import (
	instagram "go.mau.fi/mautrix-meta/pkg/igconnector"

	"github.com/whyyor/tuimessenger/internal/config"
)

func init() {
	register("instagram", newInstagram)
}

func newInstagram(_ *config.Config) (*Network, error) {
	connector := &instagram.IGConnector{}
	ig, err := loadDefaults[instagram.Config](connector)
	if err != nil {
		return nil, err
	}
	// Beeper's values: reconnect less often, and remember the connection between restarts.
	ig.CacheConnectionState = true
	ig.MinFullReconnectIntervalSeconds = 1500
	ig.ForceRefreshIntervalSeconds = 43200
	ig.DisableXMAAlways = true
	ig.DisableViewOnce = true
	return &Network{Connector: connector, Config: baseConfig("!ig")}, nil
}

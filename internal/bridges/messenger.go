package bridges

import (
	facebook "go.mau.fi/mautrix-meta/pkg/connector"

	"github.com/whyyor/tuimessenger/internal/config"
)

func init() {
	register("facebook", newFacebook)
}

func newFacebook(_ *config.Config) (*Network, error) {
	connector := &facebook.MetaConnector{}
	fb, err := loadDefaults[facebook.Config](connector)
	if err != nil {
		return nil, err
	}
	// Beeper's values: reconnect less often, and remember the connection between restarts.
	fb.CacheConnectionState = true
	fb.MinFullReconnectIntervalSeconds = 1500
	fb.ForceRefreshIntervalSeconds = 43200
	fb.DisableViewOnce = true
	// Only the first inbox page (~15-20 chats) syncs, like Beeper; older chats appear when someone writes in them.
	// Paging through the whole inbox is more traffic than Meta expects from one device, so it's left off.
	fb.ThreadBackfill.BatchCount = 0
	return &Network{Connector: connector, Config: baseConfig("!fb")}, nil
}

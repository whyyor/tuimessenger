package bridges

import (
	"time"

	whatsapp "go.mau.fi/mautrix-whatsapp/pkg/connector"

	"github.com/whyyor/tuimessenger/internal/config"
)

func init() {
	register("whatsapp", newWhatsApp)
}

func newWhatsApp(_ *config.Config) (*Network, error) {
	connector := &whatsapp.WhatsAppConnector{}
	cfg, err := loadDefaults[whatsapp.Config](connector)
	if err != nil {
		return nil, err
	}

	cfg.OSName = "tuimessenger" // name shown in WhatsApp > Linked devices
	cfg.BrowserName = "chrome"  // icon shown next to that name
	cfg.DisplaynameTemplate = `{{or .FullName .BusinessName .PushName .Phone .RedactedPhone "Unknown user"}}`

	cfg.ExtEvPolls = true
	cfg.ForceActiveDeliveryReceipts = true
	cfg.DirectMediaAutoRequest = true
	cfg.InitialAutoReconnect = true
	cfg.UseWhatsAppRetryStore = true
	cfg.LazyAvatars = true // profile photos are fetched only when shown (needs direct media, which the app turns on)

	cfg.HistorySync.RequestFullSync = false // careful: only recent history at pairing; older pages load when you scroll
	cfg.HistorySync.DispatchWait = 5 * time.Second
	cfg.HistorySync.FullSyncConfig.DaysLimit = 1825
	cfg.HistorySync.FullSyncConfig.SizeLimit = 512
	cfg.HistorySync.FullSyncConfig.StorageQuota = 16384
	cfg.HistorySync.BackwardsOnDemand = true

	// Re-compile the display name template we just replaced.
	if err := cfg.PostProcess(); err != nil {
		return nil, err
	}
	return &Network{Connector: connector, Config: baseConfig("!wa"), AfterStart: fixNumberNames}, nil
}

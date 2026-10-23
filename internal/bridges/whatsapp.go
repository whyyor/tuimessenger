package bridges

import (
	"time"

	whatsapp "go.mau.fi/mautrix-whatsapp/pkg/connector"
)

func init() {
	register("whatsapp", newWhatsApp)
}

func newWhatsApp() (*Network, error) {
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
	cfg.LazyAvatars = false // needs direct media, which we don't have yet

	cfg.HistorySync.RequestFullSync = false // only sent at pairing; turn on once messages are stored
	cfg.HistorySync.DispatchWait = 5 * time.Second
	cfg.HistorySync.FullSyncConfig.DaysLimit = 1825
	cfg.HistorySync.FullSyncConfig.SizeLimit = 512
	cfg.HistorySync.FullSyncConfig.StorageQuota = 16384
	cfg.HistorySync.BackwardsOnDemand = true

	// Re-compile the display name template we just replaced.
	if err := cfg.PostProcess(); err != nil {
		return nil, err
	}
	return &Network{Connector: connector, Config: baseConfig("!wa")}, nil
}

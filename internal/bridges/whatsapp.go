package bridges

import (
	"runtime"
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

	// WhatsApp > Linked devices shows this like a WhatsApp Web session: this computer's OS, with a Chrome icon.
	// Only applies when a device is linked (paired); existing links keep the name they were created with.
	cfg.OSName = linkedDeviceOS(runtime.GOOS)
	cfg.BrowserName = "chrome"
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

// linkedDeviceOS is the OS name WhatsApp Web reports for a computer.
func linkedDeviceOS(goos string) string {
	switch goos {
	case "darwin":
		return "Mac OS"
	case "windows":
		return "Windows"
	default:
		return "Linux"
	}
}

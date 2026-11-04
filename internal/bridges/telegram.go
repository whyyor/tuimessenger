package bridges

import (
	"fmt"

	telegram "go.mau.fi/mautrix-telegram/pkg/connector"

	"github.com/whyyor/tuimessenger/internal/config"
)

func init() {
	register("telegram", newTelegram)
}

func newTelegram(cfg *config.Config) (*Network, error) {
	connector := &telegram.TelegramConnector{}
	tg, err := loadDefaults[telegram.TelegramConfig](connector)
	if err != nil {
		return nil, err
	}

	// Your own app keys; the bridge refuses its placeholder ones.
	tg.APIID, tg.APIHash = cfg.Telegram.APIID, cfg.Telegram.APIHash
	tg.DeviceInfo.DeviceModel = "tuimessenger" // name shown in Telegram > Settings > Devices

	tg.Sync.UpdateLimit, tg.Sync.CreateLimit = -1, -1 // all your chats, like Beeper
	tg.Sync.DirectChats = true
	tg.MemberList.MaxInitialSync = 20
	tg.MaxMemberCount = 10000
	tg.ContactNames, tg.ContactAvatars = true, true
	tg.AnimatedSticker.Target = "disable" // keep stickers as they are; converting them needs extra tools
	tg.AlwaysTombstoneOnSupergroupMigration = true
	tg.DisableViewOnce = true

	if err = connector.ValidateConfig(); err != nil {
		return nil, fmt.Errorf("%w: set telegram.api_id and telegram.api_hash in config.yaml", err)
	}
	return &Network{Connector: connector, Config: baseConfig("!tg")}, nil
}

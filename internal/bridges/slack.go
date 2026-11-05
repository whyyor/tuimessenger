package bridges

import (
	slack "go.mau.fi/mautrix-slack/pkg/connector"

	"github.com/whyyor/tuimessenger/internal/config"
)

func init() {
	register("slack", newSlack)
}

func newSlack(cfg *config.Config) (*Network, error) {
	connector := &slack.SlackConnector{}
	sl, err := loadDefaults[slack.Config](connector)
	if err != nil {
		return nil, err
	}
	// Channels too, unless slack.include_channels is false (then only DMs and group DMs).
	sl.DMOnly = !cfg.Slack.IncludeChannels
	return &Network{Connector: connector, Config: baseConfig("!sl")}, nil
}

package bridges

import (
	"context"
	"fmt"

	slack "go.mau.fi/mautrix-slack/pkg/connector"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"

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
	return &Network{
		Connector: connector,
		Config:    baseConfig("!sl"),
		Settings: map[string]*Setting{
			"include_channels": {
				Get: func() bool { return !sl.DMOnly },
				Apply: func(ctx context.Context, br *bridgev2.Bridge, include bool) error {
					sl.DMOnly = !include
					if !include {
						if err := removeChannels(ctx, br); err != nil {
							return err
						}
					}
					// The bridge learns which chats are DMs when it connects, and syncs channels then too.
					go br.ResetNetworkConnections()
					return nil
				},
			},
		},
	}, nil
}

// removeChannels takes the Slack channels out of tuimessenger: you leave the rooms here and the bridge forgets
// them, but you stay in them on Slack. Turning channels on again creates them fresh, with recent messages.
func removeChannels(ctx context.Context, br *bridgev2.Bridge) error {
	portals, err := br.GetAllPortalsWithMXID(ctx)
	if err != nil {
		return err
	}
	for _, portal := range portals {
		if portal.RoomType != database.RoomTypeDefault {
			continue // DMs, group DMs and the workspace space stay
		}
		if err = br.Bot.DeleteRoom(ctx, portal.MXID, false); err != nil {
			return fmt.Errorf("failed to remove %s: %w", portal.Name, err)
		}
		if err = portal.Delete(ctx); err != nil {
			return fmt.Errorf("failed to forget %s: %w", portal.Name, err)
		}
	}
	return nil
}

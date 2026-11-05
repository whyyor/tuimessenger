package store

import (
	"strings"

	"go.mau.fi/gomuks/pkg/hicli/database"
)

// BridgeID returns the room's bridge protocol key ("whatsapp", "slackgo"…),
// or "" for native Matrix rooms.
func (rs *RoomStore) BridgeID() string {
	return bridgeIDFromMeta(rs.Meta.Current())
}

// bridgeIDFromMeta derives a room's bridge protocol key from the ghost users in
// its lazy-loading summary, e.g. "@whatsapp_15550100001:server" -> "whatsapp".
// Heroes live in room meta, so this resolves for every room in the list, whereas
// m.bridge state is only in memory for rooms that have actually been opened.
// Returns "" for native Matrix rooms.
func bridgeIDFromMeta(meta *database.Room) string {
	if meta == nil || meta.LazyLoadSummary == nil {
		return ""
	}
	for _, hero := range meta.LazyLoadSummary.Heroes {
		localpart := hero.Localpart()
		if idx := strings.IndexByte(localpart, '_'); idx > 0 {
			return localpart[:idx]
		}
		// Bridge-bot admin rooms have no ghost suffix: "@whatsappbot:server".
		if trimmed := strings.TrimSuffix(localpart, "bot"); trimmed != localpart && trimmed != "" {
			return trimmed
		}
	}
	return ""
}

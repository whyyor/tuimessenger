package tui

// Nerd Font glyphs, verified present in JetBrainsMono Nerd Font Mono. The Mono
// variant renders every glyph single-width, so the icon column stays aligned
// regardless of which bridge a room comes from.
var bridgeIcons = map[string]string{
	"whatsapp":      "\uf232",
	"telegram":      "\uf2c6",
	"slack":         "\uf198",
	"slackgo":       "\uf198",
	"instagram":     "\uf16d",
	"instagramgo":   "\uf16d",
	"signal":        "\U000f0b65",
	"discord":       "\U000f066f",
	"discordgo":     "\U000f066f",
	"facebook":      "\uf09a",
	"metago":        "\uf09a",
	"twitter":       "\uf099",
	"linkedin":      "\uf08c",
	"googlechat":    "\uf1a0",
	"gmessages":     "\uf1a0",
	"gvoice":        "\uf1a0",
	"imessage":      "\uf179",
	"imessagego":    "\uf179",
	"applemessages": "\uf179",
}

// defaultBridgeIcon (fa-comment) covers native Matrix rooms and unknown bridges.
const defaultBridgeIcon = "\uf075"

// BridgeIcon returns a single-width glyph identifying the room's source network.
func BridgeIcon(bridgeID string) string {
	if icon, ok := bridgeIcons[bridgeID]; ok {
		return icon
	}
	return defaultBridgeIcon
}

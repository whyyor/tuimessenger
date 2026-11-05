package tui

import (
	"github.com/gdamore/tcell/v2"
)

type bridgeIconSpec struct {
	Icon  string
	Color tcell.Color
}

// Nerd Font glyphs, verified present in JetBrainsMono Nerd Font Mono. The Mono
// variant renders every glyph single-width, so the icon column stays aligned.
// Colors are the networks' brand colors, which read well on a dark background.
var bridgeIcons = map[string]bridgeIconSpec{
	"whatsapp":      {"\uf232", tcell.NewHexColor(0x25d366)},
	"telegram":      {"\uf2c6", tcell.NewHexColor(0x2aabee)},
	"slack":         {"\uf198", tcell.NewHexColor(0xe01e5a)},
	"slackgo":       {"\uf198", tcell.NewHexColor(0xe01e5a)},
	"instagram":     {"\uf16d", tcell.NewHexColor(0xe4405f)},
	"instagramgo":   {"\uf16d", tcell.NewHexColor(0xe4405f)},
	"signal":        {"\U000f0b65", tcell.NewHexColor(0x3a76f0)},
	"discord":       {"\U000f066f", tcell.NewHexColor(0x5865f2)},
	"discordgo":     {"\U000f066f", tcell.NewHexColor(0x5865f2)},
	"facebook":      {"\uf09a", tcell.NewHexColor(0x0866ff)},
	"metago":        {"\uf09a", tcell.NewHexColor(0x0866ff)},
	"twitter":       {"\uf099", tcell.NewHexColor(0x1da1f2)},
	"linkedin":      {"\uf08c", tcell.NewHexColor(0x0a66c2)},
	"googlechat":    {"\uf1a0", tcell.NewHexColor(0x34a853)},
	"gmessages":     {"\uf1a0", tcell.NewHexColor(0x34a853)},
	"gvoice":        {"\uf1a0", tcell.NewHexColor(0x34a853)},
	"imessage":      {"\uf179", tcell.NewHexColor(0xf5f5f7)},
	"imessagego":    {"\uf179", tcell.NewHexColor(0xf5f5f7)},
	"applemessages": {"\uf179", tcell.NewHexColor(0xf5f5f7)},
}

// defaultBridgeIcon (fa-comment) covers native Matrix rooms and unknown bridges.
var defaultBridgeIcon = bridgeIconSpec{"\uf075", tcell.NewHexColor(0x8a8a94)}

// BridgeIcon returns a single-width glyph identifying the room's source network.
func BridgeIcon(bridgeID string) string {
	icon, _ := BridgeIconColor(bridgeID)
	return icon
}

// BridgeIconColor returns the glyph together with its brand color.
func BridgeIconColor(bridgeID string) (string, tcell.Color) {
	if spec, ok := bridgeIcons[bridgeID]; ok {
		return spec.Icon, spec.Color
	}
	return defaultBridgeIcon.Icon, defaultBridgeIcon.Color
}

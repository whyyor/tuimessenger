package tui

import (
	"github.com/gdamore/tcell/v2"
)

// A quiet slate palette to replace the stock 16-color greens. Truecolor is
// safe here: this fork targets Ghostty.
var (
	// ColorBarBackground is used for the topic bar at the top of a room.
	ColorBarBackground = tcell.NewHexColor(0x2a2a37)
	// ColorBarText is the foreground on top of ColorBarBackground.
	ColorBarText = tcell.NewHexColor(0xc8c9c5)
	// ColorSelectionBackground marks the selected room in the room list.
	ColorSelectionBackground = tcell.NewHexColor(0x2d4f67)
	// ColorSelectionText is the foreground on top of ColorSelectionBackground.
	ColorSelectionText = tcell.NewHexColor(0xdcd7ba)
	// ColorStatusText is used for the transient status line above the composer.
	ColorStatusText = tcell.NewHexColor(0x8a8a94)
)

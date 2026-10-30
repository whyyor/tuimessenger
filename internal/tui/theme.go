package tui

import (
	"github.com/gdamore/tcell/v2"
)

// A quiet slate palette to replace the stock 16-color greens. Truecolor is
// safe here: this fork targets Ghostty.
var (
	// ColorBarBackground is the selected row's bar in Ctrl+p and Ctrl+k: the
	// same grey as the borders, so the two always match.
	ColorBarBackground = ColorBorder
	// ColorBarText is the foreground on top of ColorBarBackground.
	ColorBarText = tcell.NewHexColor(0xc8c9c5)
	// ColorSelectionBackground marks the selected room in the room list.
	// Classic Monokai red: the Remastered/Pro salmons read as pink, not red.
	ColorSelectionBackground = tcell.NewHexColor(0xf92672)
	// ColorSelectionText is the foreground on top of ColorSelectionBackground,
	// Monokai's off-white.
	ColorSelectionText = tcell.NewHexColor(0xfff1f3)
	// ColorStatusText is used for the transient status line above the composer.
	ColorStatusText = tcell.NewHexColor(0x8a8a94)
	// ColorBorder is used for modal borders: the same dark grey as the
	// selection bar.
	ColorBorder = tcell.NewHexColor(0x2a2a37)
	// ColorUnreadBadge marks rooms with unread messages in the room list.
	ColorUnreadBadge = tcell.NewHexColor(0x7fb4ca)
	// ColorUnreadHighlight marks rooms with unread mentions in the room list.
	ColorUnreadHighlight = tcell.NewHexColor(0xe6c384)
	// ColorComposerBar is the accent line beside the message composer,
	// Ghostty's Monokai Remastered purple.
	ColorComposerBar = tcell.NewHexColor(0xa8a9eb)
)

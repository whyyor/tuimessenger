package tui

import "github.com/gdamore/tcell/v2"

// vivid brightens c at the same hue: channels scale until the brightest is
// full, so green stays green (mixing in white washed it out). It lights up
// the selected icon without bold, which Nerd Font icons glitch on.
func vivid(c tcell.Color) tcell.Color {
	r, g, b := c.RGB()
	top := max(r, g, b)
	if r < 0 || top <= 0 {
		return c
	}
	scale := func(v int32) int32 { return v * 255 / top }
	return tcell.NewRGBColor(scale(r), scale(g), scale(b))
}

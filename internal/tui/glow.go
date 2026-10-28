package tui

import "github.com/gdamore/tcell/v2"

// iconGlow is how far a selected row's icon moves toward white: a "lit"
// look without bold, which Nerd Font icons glitch on.
const iconGlow = 0.6

// mixColor moves c toward target by t (0..1); default colours stay as they are.
func mixColor(c, target tcell.Color, t float64) tcell.Color {
	r1, g1, b1 := c.RGB()
	r2, g2, b2 := target.RGB()
	if r1 < 0 || r2 < 0 {
		return c
	}
	mix := func(a, b int32) int32 { return a + int32(float64(b-a)*t) }
	return tcell.NewRGBColor(mix(r1, r2), mix(g1, g2), mix(b1, b2))
}

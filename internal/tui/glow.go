package tui

import (
	"math"

	"github.com/gdamore/tcell/v2"
)

// glowSaturation multiplies a selected icon's saturation (capped at full);
// glowMinLuma keeps pure blues from going dark on the bar.
const (
	glowSaturation = 1.6
	glowMinLuma    = 0.3
)

// vivid turns c into its neon self: same hue, full brightness, saturation
// boosted, so green glows green (no white mix). Near-greys stay grey. It
// lights up the selected icon without bold, which Nerd Font icons glitch on.
func vivid(c tcell.Color) tcell.Color {
	r, g, b := c.RGB()
	if r < 0 {
		return c
	}
	rf, gf, bf := float64(r)/255, float64(g)/255, float64(b)/255
	hi, lo := max(rf, gf, bf), min(rf, gf, bf)
	if hi == 0 {
		return c
	}
	var h float64
	switch d := hi - lo; {
	case d == 0:
	case hi == rf:
		h = math.Mod((gf-bf)/d, 6)
	case hi == gf:
		h = (bf-rf)/d + 2
	default:
		h = (rf-gf)/d + 4
	}
	if h < 0 {
		h += 6
	}
	// HSV back to RGB with V = 1.
	x := 1 - math.Abs(math.Mod(h, 2)-1)
	var rr, gg, bb float64
	switch int(h) % 6 {
	case 0:
		rr, gg, bb = 1, x, 0
	case 1:
		rr, gg, bb = x, 1, 0
	case 2:
		rr, gg, bb = 0, 1, x
	case 3:
		rr, gg, bb = 0, x, 1
	case 4:
		rr, gg, bb = x, 0, 1
	default:
		rr, gg, bb = 1, 0, x
	}
	orig := (hi - lo) / hi
	s := min(1, orig*glowSaturation)
	at := func(s float64) (float64, float64, float64) { return 1 - s + s*rr, 1 - s + s*gg, 1 - s + s*bb }
	// Ease saturation back while it reads too dark (blue is the dimmest hue).
	for ; s > orig; s -= 0.02 {
		r2, g2, b2 := at(s)
		if 0.2126*r2+0.7152*g2+0.0722*b2 >= glowMinLuma {
			break
		}
	}
	r2, g2, b2 := at(max(s, 0))
	ch := func(v float64) int32 { return int32(math.Round(v * 255)) }
	return tcell.NewRGBColor(ch(r2), ch(g2), ch(b2))
}

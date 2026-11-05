package tui

import "testing"

// Timeline of 100 lines, viewport of 20. ScrollOffset counts lines up from
// the bottom, so scroll=0 shows lines 80..99 and scroll=30 shows 50..69.
func TestScrollForSpan(t *testing.T) {
	const total, height = 100, 20
	cases := []struct {
		name                string
		scroll, first, last int
		want                int
	}{
		{"already visible: no movement", 0, 85, 87, 0},
		{"above the window: align to top", 0, 70, 72, 10},
		{"far above: align its top line", 0, 10, 12, 70},
		{"below the window: align to bottom", 30, 75, 76, 23},
		{"newest message below: back to scroll 0", 30, 98, 99, 0},
		{"taller than viewport: show its top", 0, 40, 75, 40},
		{"oldest line aligns to the top", 0, 0, 1, 80},
	}
	for _, c := range cases {
		if got := scrollForSpan(total, height, c.scroll, c.first, c.last); got != c.want {
			t.Errorf("%s: scrollForSpan(scroll=%d, %d..%d) = %d, want %d",
				c.name, c.scroll, c.first, c.last, got, c.want)
		}
	}
}

func TestBottomVisibleLine(t *testing.T) {
	if got := bottomVisibleLine(100, 0); got != 99 {
		t.Errorf("at the bottom: got %d, want 99", got)
	}
	if got := bottomVisibleLine(100, 30); got != 69 {
		t.Errorf("scrolled up 30: got %d, want 69", got)
	}
}

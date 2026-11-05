package messages

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/mauview"
)

func TestWaveformLevels(t *testing.T) {
	// 64 values like WhatsApp: a ramp up then down, peak 100.
	wave := make([]int, 64)
	for i := range wave {
		wave[i] = min(i, 63-i) * 100 / 31
	}
	got := waveformLevels(wave)
	if len(got) != waveformBars {
		t.Fatalf("got %d bars", len(got))
	}
	if got[0] != 0 || got[7] != 7 || got[8] != 7 || got[15] != 0 {
		t.Errorf("unexpected shape %v", got)
	}
	// Scale-independent: a 0–1024 waveform gives the same bars.
	big := make([]int, 64)
	for i, v := range wave {
		big[i] = v * 10
	}
	if b := waveformLevels(big); b[3] != got[3] || b[12] != got[12] {
		t.Errorf("scale changed bars: %v vs %v", b, got)
	}
	if z := waveformLevels([]int{0, 0}); z[0] != 0 {
		t.Errorf("silence: %v", z)
	}
	// Shorter than 16 values still fills every bar.
	if s := waveformLevels([]int{10, 20, 30}); s[15] != 7 {
		t.Errorf("short wave: %v", s)
	}
}

func TestSyntheticLevelsStable(t *testing.T) {
	a, b := syntheticLevels("$evt"), syntheticLevels("$evt")
	for i := range a {
		if a[i] != b[i] || a[i] < 1 || a[i] > 7 {
			t.Fatalf("unstable or out of range: %v %v", a, b)
		}
	}
	if a[0] > 3 || a[15] > 3 {
		t.Errorf("ends not tapered: %v", a)
	}
}

func rowText(screen mauview.Screen, width int) string {
	out := make([]rune, 0, width)
	for x := range width {
		r, _, _, _ := screen.GetContent(x, 0)
		out = append(out, r)
	}
	return string(out)
}

func TestDrawVoice(t *testing.T) {
	msg := &FileMessage{eventID: "$a", Duration: 14 * time.Second, levels: []int{0, 1, 2, 3, 4, 5, 6, 7, 7, 6, 5, 4, 3, 2, 1, 0}}
	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(60, 1)
	var screen mauview.Screen = sim
	msg.drawVoice(screen)
	if got, want := rowText(screen, 23), "▶ 0:14 ▁▂▃▄▅▆▇██▇▆▅▄▃▂▁"; got != want {
		t.Errorf("idle: got %q, want %q", got, want)
	}
	SetPlaying("$a", msg.Duration)
	defer SetPlaying("", 0)
	nowPlaying.started = time.Now().Add(-7 * time.Second)
	screen.Clear()
	msg.drawVoice(screen)
	if got, want := rowText(screen, 30), "■ 0:07 / 0:14 ▁▂▃▄▅▆▇██▇▆▅▄▃▂▁"; got != want {
		t.Errorf("playing: got %q, want %q", got, want)
	}
}

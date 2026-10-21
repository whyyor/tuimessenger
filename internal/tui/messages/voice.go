package messages

import (
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/mauview"
	"maunium.net/go/mautrix/id"
)

const waveformBars = 16

var waveformGlyphs = []rune("▁▂▃▄▅▆▇█")

// ColorVoiceAccent colours played bars and the playing head.
var ColorVoiceAccent = tcell.NewHexColor(0xa8a9eb)

var nowPlaying struct {
	sync.Mutex
	eventID  id.EventID
	started  time.Time
	duration time.Duration
}

// SetPlaying marks which voice note is playing; empty eventID clears it.
func SetPlaying(eventID id.EventID, duration time.Duration) {
	nowPlaying.Lock()
	defer nowPlaying.Unlock()
	nowPlaying.eventID = eventID
	nowPlaying.started = time.Now()
	nowPlaying.duration = duration
}

// Playing returns the playing event and its elapsed time.
func Playing() (id.EventID, time.Duration) {
	nowPlaying.Lock()
	defer nowPlaying.Unlock()
	if nowPlaying.eventID == "" {
		return "", 0
	}
	return nowPlaying.eventID, time.Since(nowPlaying.started)
}

// waveformLevels squeezes any waveform into 16 bars of 0–7, scaled to its peak.
func waveformLevels(wave []int) []int {
	levels := make([]int, waveformBars)
	if len(wave) == 0 {
		return levels
	}
	buckets := make([]int, waveformBars)
	peak := 0
	for i := range buckets {
		start, end := i*len(wave)/waveformBars, (i+1)*len(wave)/waveformBars
		if end <= start {
			end = start + 1
		}
		sum, n := 0, 0
		for j := start; j < end && j < len(wave); j++ {
			sum += max(wave[j], 0)
			n++
		}
		if n > 0 {
			buckets[i] = sum / n
		}
		peak = max(peak, buckets[i])
	}
	if peak == 0 {
		return levels
	}
	for i, v := range buckets {
		levels[i] = (v*7 + peak/2) / peak
	}
	return levels
}

// syntheticLevels is a stable speech-like shape for notes without a waveform.
func syntheticLevels(seed string) []int {
	h := fnv.New64a()
	_, _ = h.Write([]byte(seed))
	z := h.Sum64()
	levels := make([]int, waveformBars)
	for i := range levels {
		z += 0x9e3779b97f4a7c15
		x := z
		x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
		x = (x ^ (x >> 27)) * 0x94d049bb133111eb
		x ^= x >> 31
		levels[i] = int(x%7) + 1
	}
	levels[0], levels[waveformBars-1] = min(levels[0], 3), min(levels[waveformBars-1], 3)
	return levels
}

func formatDuration(d time.Duration) string {
	s := int(d.Round(time.Second) / time.Second)
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// drawVoice draws "▶ 0:14 ▁▂▃…" (idle) or "■ 0:07 / 0:14 ▁▂▃…" (playing).
func (msg *FileMessage) drawVoice(screen mauview.Screen) {
	dim := tcell.StyleDefault.Foreground(tcell.ColorGray)
	accent := tcell.StyleDefault.Foreground(ColorVoiceAccent)
	playingID, elapsed := Playing()
	playing := playingID != "" && playingID == msg.eventID
	if measuring {
		// Size the bubble for the widest (playing) form.
		playing, elapsed = true, msg.Duration
	}
	levels, barStyle := msg.levels, accent
	if msg.synthetic {
		barStyle = dim
	}
	head, headStyle, played := "▶ "+formatDuration(msg.Duration)+" ", dim, -1
	if playing {
		elapsed = min(elapsed, msg.Duration)
		head = "■ " + formatDuration(elapsed) + " / " + formatDuration(msg.Duration) + " "
		headStyle = accent.Bold(true)
		if msg.Duration > 0 {
			played = int(elapsed * waveformBars / msg.Duration)
		}
	}
	x := 0
	width, _ := screen.Size()
	put := func(r rune, style tcell.Style) {
		if x < width {
			screen.SetContent(x, 0, r, nil, style)
		}
		x++
	}
	for _, r := range head {
		put(r, headStyle)
	}
	for i, level := range levels {
		style := barStyle
		if playing {
			style = dim
			if i < played {
				style = accent
			}
		}
		put(waveformGlyphs[level], style)
	}
	tail := msg.voiceTail
	if playing && (!measuring || len(tail) < len(" · playing")) {
		tail = " · playing"
	}
	for _, r := range tail {
		put(r, dim)
	}
}

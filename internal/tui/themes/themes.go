// Package themes turns a theme name into tuimessenger's colours: the built-in "tuimessenger" theme, opencode's
// themes (bundled, MIT, see opencode/LICENSE), or a theme file of yours in opencode's format.
package themes

import (
	"embed"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Default is the theme used when none is chosen: tuimessenger's own colours.
const Default = "tuimessenger"

//go:embed opencode/*.json
var bundled embed.FS

// Mode picks a theme's dark or light colours.
type Mode string

const (
	Dark  Mode = "dark"
	Light Mode = "light"
)

// Builtin is every theme that ships with tuimessenger, Default first.
func Builtin() []string {
	entries, _ := bundled.ReadDir("opencode")
	names := []string{Default}
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), ".json"))
	}
	slices.Sort(names[1:])
	return names
}

// Available is every theme you can pick: the built-in ones, then yours from dir (*.json), sorted.
func Available(dir string) []string {
	names := Builtin()
	for _, name := range Custom(dir) {
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

// Custom lists the theme files in dir, by name.
func Custom(dir string) []string {
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	var names []string
	for _, f := range files {
		names = append(names, strings.TrimSuffix(filepath.Base(f), ".json"))
	}
	slices.Sort(names)
	return names
}

// file is opencode's theme format: named colours (defs) and the slots that use them.
type file struct {
	Defs  map[string]json.RawMessage `json:"defs"`
	Theme map[string]json.RawMessage `json:"theme"`
}

// Colors are a theme's colours by tuimessenger colour key ("accent", "panel"…), as "#rrggbb" or "default" (the
// terminal's own colour, for transparent slots). The Default theme has none: it's the built-in colours.
type Colors map[string]string

// Load resolves a theme for mode. Your theme files in dir win over built-in ones of the same name.
func Load(name string, mode Mode, dir string) (Colors, error) {
	if name == "" || name == Default {
		return Colors{}, nil
	}
	var data []byte
	err := os.ErrNotExist
	if dir != "" {
		data, err = os.ReadFile(filepath.Join(dir, name+".json"))
	}
	if os.IsNotExist(err) {
		if data, err = bundled.ReadFile("opencode/" + name + ".json"); err != nil {
			return nil, fmt.Errorf("no theme %q", name)
		}
	} else if err != nil {
		return nil, err
	}
	var f file
	if err = json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("theme %s: %w", name, err)
	}
	slots, err := f.resolve(mode)
	if err != nil {
		return nil, fmt.Errorf("theme %s: %w", name, err)
	}
	return mapSlots(slots), nil
}

// rgba is a resolved colour; a = 0 is transparent (the terminal's own colour).
type rgba struct {
	r, g, b, a uint8
}

func (c rgba) hex() string {
	if c.a == 0 {
		return "default"
	}
	return fmt.Sprintf("#%02x%02x%02x", c.r, c.g, c.b)
}

// resolve follows opencode's rules (resolveV1): "#hex", "none"/"transparent", a defs name or another slot (followed,
// loops refused), an ANSI colour number, or {"dark": …, "light": …}.
func (f *file) resolve(mode Mode) (map[string]rgba, error) {
	var resolveValue func(raw json.RawMessage, chain []string) (rgba, error)
	resolveValue = func(raw json.RawMessage, chain []string) (rgba, error) {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			switch {
			case s == "none" || s == "transparent":
				return rgba{}, nil
			case strings.HasPrefix(s, "#"):
				return parseHex(s)
			case slices.Contains(chain, s):
				return rgba{}, fmt.Errorf("colour reference loop: %s -> %s", strings.Join(chain, " -> "), s)
			}
			next, ok := f.Defs[s]
			if !ok {
				next, ok = f.Theme[s]
			}
			if !ok {
				return rgba{}, fmt.Errorf("colour %q isn't in defs or theme", s)
			}
			return resolveValue(next, append(chain, s))
		}
		var n float64
		if json.Unmarshal(raw, &n) == nil {
			return ansi(int(n)), nil
		}
		var modes map[string]json.RawMessage
		if json.Unmarshal(raw, &modes) == nil {
			if v, ok := modes[string(mode)]; ok {
				return resolveValue(v, chain)
			}
		}
		return rgba{}, fmt.Errorf("can't read colour %s", raw)
	}
	out := map[string]rgba{}
	for key, raw := range f.Theme {
		if key == "thinkingOpacity" {
			continue
		}
		// Like opencode, the chain starts empty: "warning": "warning" means defs' warning, not a loop.
		c, err := resolveValue(raw, nil)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		out[key] = c
	}
	for _, key := range []string{"primary", "accent", "text", "background"} {
		if _, ok := out[key]; !ok {
			return nil, fmt.Errorf("missing %q", key)
		}
	}
	return out, nil
}

func parseHex(s string) (rgba, error) {
	h := strings.TrimPrefix(s, "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	var c rgba
	var err error
	switch len(h) {
	case 6:
		_, err = fmt.Sscanf(h, "%02x%02x%02x", &c.r, &c.g, &c.b)
		c.a = 255
	case 8:
		_, err = fmt.Sscanf(h, "%02x%02x%02x%02x", &c.r, &c.g, &c.b, &c.a)
	default:
		err = fmt.Errorf("bad length")
	}
	if err != nil {
		return rgba{}, fmt.Errorf("%q isn't a colour", s)
	}
	return c, nil
}

// ansi is xterm's 256-colour palette.
func ansi(n int) rgba {
	base := []rgba{{0, 0, 0, 255}, {128, 0, 0, 255}, {0, 128, 0, 255}, {128, 128, 0, 255}, {0, 0, 128, 255},
		{128, 0, 128, 255}, {0, 128, 128, 255}, {192, 192, 192, 255}, {128, 128, 128, 255}, {255, 0, 0, 255},
		{0, 255, 0, 255}, {255, 255, 0, 255}, {0, 0, 255, 255}, {255, 0, 255, 255}, {0, 255, 255, 255}, {255, 255, 255, 255}}
	switch {
	case n < 0 || n > 255:
		return rgba{}
	case n < 16:
		return base[n]
	case n < 232:
		n -= 16
		step := func(v int) uint8 {
			if v == 0 {
				return 0
			}
			return uint8(55 + v*40)
		}
		return rgba{step(n / 36), step(n / 6 % 6), step(n % 6), 255}
	}
	v := uint8(8 + (n-232)*10)
	return rgba{v, v, v, 255}
}

// onColor is the text colour for something drawn on bg: the theme's background when it's solid (opencode's rule
// for selected rows), else black or white, whichever reads better.
func onColor(slots map[string]rgba, bg rgba) rgba {
	if c, ok := slots["selectedListItemText"]; ok {
		return c
	}
	if b := slots["background"]; b.a != 0 {
		return b
	}
	if luminance(bg) > 0.45 {
		return rgba{0, 0, 0, 255}
	}
	return rgba{255, 255, 255, 255}
}

func luminance(c rgba) float64 {
	lin := func(v uint8) float64 {
		f := float64(v) / 255
		if f <= 0.03928 {
			return f / 12.92
		}
		return math.Pow((f+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.r) + 0.7152*lin(c.g) + 0.0722*lin(c.b)
}

// mapSlots puts opencode's slots on tuimessenger's colours.
func mapSlots(s map[string]rgba) Colors {
	get := func(key, fallback string) rgba {
		if c, ok := s[key]; ok {
			return c
		}
		return s[fallback]
	}
	element := get("backgroundElement", "backgroundPanel")
	c := Colors{
		"accent":                 s["primary"].hex(),
		"selection":              s["primary"].hex(),
		"selection_text":         onColor(s, s["primary"]).hex(),
		"sidebar_selection":      s["accent"].hex(),
		"sidebar_selection_text": onColor(s, s["accent"]).hex(),
		"panel":                  get("backgroundPanel", "background").hex(),
		"selection_bar":          element.hex(),
		"border":                 get("border", "textMuted").hex(),
		"dim_text":               get("textMuted", "text").hex(),
		"bar_text":               s["text"].hex(),
		"palette_pill":           get("success", "primary").hex(),
		"palette_pill_text":      onColor(s, get("success", "primary")).hex(),
		"toast_success":          get("success", "primary").hex(),
		"toast_info":             get("info", "primary").hex(),
		"toast_warning":          get("warning", "accent").hex(),
		"toast_error":            get("error", "accent").hex(),
		"unread_badge":           get("info", "primary").hex(),
		"unread_mention":         get("warning", "accent").hex(),
		"pill_yellow":            get("warning", "accent").hex(),
		"pill_pink":              s["accent"].hex(),
		"pill_green":             get("success", "primary").hex(),
		"pill_blue":              get("info", "primary").hex(),
		"pill_violet":            get("secondary", "primary").hex(),
		"pill_text":              onColor(s, get("warning", "accent")).hex(),
		"code_background":        element.hex(),
		"inline_code":            get("markdownCode", "accent").hex(),
	}
	return c
}

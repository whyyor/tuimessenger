package termapp

import (
	"bytes"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

func TestTranslate(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{"Ctrl+m is mark seen's byte, not Enter", "\x1b[109;5u", "\x1e"},
		{"Ctrl+; is select's byte", "\x1b[59;5u", "\x1f"},
		{"Shift+Enter is a new line", "\x1b[13;2u", "\n"},
		{"Ctrl+Enter too", "\x1b[13;5u", "\n"},
		{"Esc", "\x1b[27u", "\x1b"},
		{"Ctrl+c", "\x1b[99;5u", "\x03"},
		{"Ctrl+Shift+k", "\x1b[107;6u", "\x0b"},
		{"Ctrl+^ and Ctrl+_ as before", "\x1b[54;5u\x1b[95;5u", "\x1e\x1f"},
		{"Ctrl+Space", "\x1b[32;5u", "\x00"},
		{"Ctrl+Backspace", "\x1b[127;5u", "\x08"},
		{"Shift+Tab", "\x1b[9;2u", "\x1b[Z"},
		{"Alt+u", "\x1b[117;3u", "\x1bu"},
		{"Alt+Shift+u", "\x1b[117;4u", "\x1bU"},
		{"Ctrl+Alt+w", "\x1b[119;7u", "\x1b\x17"},
		{"caps lock doesn't change Ctrl+m", "\x1b[109;69u", "\x1e"},
		{"key release dropped", "\x1b[109;5:3u", ""},
		{"Cmd keys dropped", "\x1b[99;9u", ""},
		{"lone modifier dropped", "\x1b[57441;2u", ""},
		{"text around it", "hi\x1b[13;2uyo", "hi\nyo"},
		{"arrows untouched", "\x1b[A\x1b[1;5C", "\x1b[A\x1b[1;5C"},
		{"mouse untouched", "\x1b[<0;10;5M", "\x1b[<0;10;5M"},
		{"plain Esc untouched", "\x1b", "\x1b"},
		{"utf-8 untouched", "héllo ✎", "héllo ✎"},
	} {
		out, pending := translate([]byte(c.in))
		if string(out) != c.want || pending != nil {
			t.Errorf("%s: %q → %q (pending %q), want %q", c.name, c.in, out, pending, c.want)
		}
	}
}

// A report cut between two reads is held back until it's complete; a bare Esc never is.
func TestTranslateAcrossReads(t *testing.T) {
	out, pending := translate([]byte("a\x1b[109;"))
	if string(out) != "a" || string(pending) != "\x1b[109;" {
		t.Fatalf("held back: %q %q", out, pending)
	}
	out, pending = translate(append(pending, "5ub"...))
	if string(out) != "\x1eb" || pending != nil {
		t.Errorf("completed: %q %q", out, pending)
	}
	if out, pending = translate([]byte("\x1b")); string(out) != "\x1b" || pending != nil {
		t.Errorf("Esc goes through at once: %q %q", out, pending)
	}
	if out, pending = translate([]byte("\x1b[")); string(out) != "\x1b[" || pending != nil {
		t.Errorf("Esc [ goes through at once: %q %q", out, pending)
	}
}

// fakeTty is a terminal for tests: keys go in through input, what's written comes out in written.
type fakeTty struct {
	input   chan []byte
	drained chan struct{}
	once    sync.Once
	mu      sync.Mutex
	written bytes.Buffer
}

func newFakeTty() *fakeTty {
	return &fakeTty{input: make(chan []byte, 16), drained: make(chan struct{})}
}

func (f *fakeTty) Start() error { return nil }
func (f *fakeTty) Stop() error  { return nil }
func (f *fakeTty) Drain() error {
	f.once.Do(func() { close(f.drained) })
	return nil
}
func (f *fakeTty) NotifyResize(func()) {}
func (f *fakeTty) WindowSize() (tcell.WindowSize, error) {
	return tcell.WindowSize{Width: 80, Height: 24}, nil
}
func (f *fakeTty) Close() error { return nil }
func (f *fakeTty) Read(p []byte) (int, error) {
	select {
	case b := <-f.input:
		return copy(p, b), nil
	case <-f.drained:
		return 0, io.EOF
	}
}
func (f *fakeTty) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.written.Write(p)
}
func (f *fakeTty) output() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.written.String()
}

// The keys from issue #43 arrive in tcell as the keys tuimessenger handles, including a report split over reads.
func TestKittyKeysReachTcell(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	fake := newFakeTty()
	keys := &kittyTty{Tty: fake, push: true}
	screen, err := tcell.NewTerminfoScreenFromTty(keys)
	if err != nil {
		t.Fatal(err)
	}
	if err = screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	keys.enable()
	if out := fake.output(); !bytes.HasSuffix([]byte(out), []byte(kittyOn)) {
		t.Errorf("protocol turned on after tcell's setup: %q", out)
	}

	next := func() *tcell.EventKey {
		t.Helper()
		got := make(chan *tcell.EventKey, 1)
		go func() {
			// Skip the resize tcell sends on start.
			for {
				ev := screen.PollEvent()
				if ev == nil {
					return
				}
				if key, ok := ev.(*tcell.EventKey); ok {
					got <- key
					return
				}
			}
		}()
		select {
		case key := <-got:
			return key
		case <-time.After(2 * time.Second):
			t.Fatal("no key")
		}
		return nil
	}
	for _, c := range []struct {
		name  string
		input []string
		key   tcell.Key
	}{
		{"Ctrl+m", []string{"\x1b[109;5u"}, tcell.KeyCtrlCarat},
		{"Ctrl+;", []string{"\x1b[59;5u"}, tcell.KeyCtrlUnderscore},
		{"Shift+Enter", []string{"\x1b[13;2u"}, tcell.KeyLF},
		{"Ctrl+m split over two reads", []string{"\x1b[10", "9;5u"}, tcell.KeyCtrlCarat},
		{"Esc", []string{"\x1b[27u"}, tcell.KeyEsc},
		{"Enter", []string{"\r"}, tcell.KeyEnter},
	} {
		for _, in := range c.input {
			fake.input <- []byte(in)
		}
		if got := next(); got.Key() != c.key {
			t.Errorf("%s: got %v, want %v", c.name, got.Name(), tcell.KeyNames[c.key])
		}
	}

	keys.disable()
	keys.disable()
	if out := fake.output(); !bytes.HasSuffix([]byte(out), []byte(kittyOff)) || bytes.Count([]byte(out), []byte(kittyOff)) != 1 {
		t.Errorf("turned off once: %q", out)
	}
}

// TUIMESSENGER_KITTY_KEYBOARD=0 leaves the protocol off; a nil kittyTty (no /dev/tty) is safe to use.
func TestKittyOptOut(t *testing.T) {
	fake := newFakeTty()
	keys := &kittyTty{Tty: fake, push: false}
	keys.enable()
	keys.disable()
	if fake.output() != "" {
		t.Errorf("nothing sent: %q", fake.output())
	}
	var none *kittyTty
	none.enable()
	none.disable()
}

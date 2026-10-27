package tui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/mauview"

	"go.mau.fi/gomuks/tui/config"
)

func freshRecent(t *testing.T, dir string) {
	t.Helper()
	old := recentEmoji
	recentEmoji = &recentEmojis{}
	recentEmoji.use(dir)
	t.Cleanup(func() { recentEmoji = old })
}

func TestRecentEmojiTopAndPersist(t *testing.T) {
	dir := t.TempDir()
	freshRecent(t, dir)
	if got := recentEmoji.Top(6); !slices.Equal(got, quickReactions) {
		t.Fatalf("empty history should show the defaults, got %v", got)
	}
	recentEmoji.Add("🔥")
	recentEmoji.Add("❤") // same as the default ❤️, without the selector
	recentEmoji.Add("🔥") // back to the front, not duplicated
	got := recentEmoji.Top(6)
	if want := []string{"🔥", "❤", "👍", "😂", "😮", "😢"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	path := filepath.Join(dir, "recent-emoji.json")
	for i := 0; i < 200; i++ {
		if data, _ := os.ReadFile(path); strings.HasPrefix(string(data), `["🔥","❤"]`) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	reloaded := (&recentEmojis{}).use(dir)
	if got := reloaded.Top(2); !slices.Equal(got, []string{"🔥", "❤"}) {
		t.Errorf("recent emoji should survive a restart, got %v", got)
	}
}

func TestInsertAtCursor(t *testing.T) {
	view := &RoomView{input: mauview.NewInputArea()}
	view.input.SetText("hello world")
	view.input.SetCursorOffset(5)
	view.insertAtCursor("🔥")
	if got := view.input.GetText(); got != "hello🔥 world" {
		t.Errorf("got %q", got)
	}
	view.insertAtCursor("👍")
	if got := view.input.GetText(); got != "hello🔥👍 world" {
		t.Errorf("cursor should sit after the first emoji: %q", got)
	}
	if got := textBeforeWidth("ab\ncd", 4); got != "ab\nc" {
		t.Errorf("newline counts as one cell: %q", got)
	}
}

func emojiView(t *testing.T) *RoomView {
	t.Helper()
	freshRecent(t, "")
	return &RoomView{config: &config.Config{}, input: mauview.NewInputArea()}
}

func TestCtrlEInsertsSearchedEmoji(t *testing.T) {
	view := emojiView(t)
	view.input.SetTextAndMoveCursor("so ")
	key := func(k tcell.Key, r rune) { view.OnKeyEvent(tcell.NewEventKey(k, r, tcell.ModNone)) }

	key(tcell.KeyCtrlE, 0)
	if view.picker == nil || !view.picker.insert {
		t.Fatal("Ctrl+e should open the emoji picker in insert mode")
	}
	for _, r := range "/fire" {
		key(tcell.KeyRune, r)
	}
	if len(view.picker.results) == 0 {
		t.Fatal("no results for fire")
	}
	want := view.picker.results[0].Emoji
	key(tcell.KeyEnter, 0)
	if view.picker != nil {
		t.Error("picking should close the popup")
	}
	if got := view.input.GetText(); got != "so "+want {
		t.Errorf("got %q, want %q", got, "so "+want)
	}

	key(tcell.KeyCtrlE, 0)
	if view.picker.quick[0] != want {
		t.Errorf("the pick should lead the quick row next time: %v", view.picker.quick)
	}
	// Slot 2 is the first default once one emoji is recent.
	key(tcell.KeyRune, '2')
	if got := view.input.GetText(); got != "so "+want+quickReactions[0] {
		t.Errorf("number keys insert from the quick row: %q", got)
	}
}

func TestEmojiPickerDrawsAboveComposer(t *testing.T) {
	view := emojiView(t)
	view.OpenEmojiPicker()
	const w, h = 60, 20
	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(w, h)
	view.picker.Draw(sim, 0, h, -1)
	if view.picker.y != h-view.picker.h || view.picker.x != 0 {
		t.Errorf("box should sit at the bottom-left, got x=%d y=%d h=%d", view.picker.x, view.picker.y, view.picker.h)
	}
	if text := strings.Join(screenText(sim, w, h), "\n"); !strings.Contains(text, "Emoji") {
		t.Errorf("title should say Emoji:\n%s", text)
	}
}

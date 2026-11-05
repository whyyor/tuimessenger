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
	"go.mau.fi/gomuks/tui/lib/emoji"
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

func TestEmojiGridBrowseAndPick(t *testing.T) {
	view := emojiView(t)
	view.input.SetTextAndMoveCursor("hmm ")
	view.OpenEmojiPicker()
	p := view.picker
	key := func(k tcell.Key, r rune) { p.OnKeyEvent(tcell.NewEventKey(k, r, tcell.ModNone)) }

	key(tcell.KeyRune, '/')
	if !p.gridMode() || p.cat != 1 {
		t.Fatalf("/ should open the grid on smileys, cat=%d", p.cat)
	}
	key(tcell.KeyRight, 0)
	key(tcell.KeyDown, 0)
	want := emoji.Categories()[0].Emojis[gridCols+1].Emoji
	if row := p.nameRow(); len(row) == 0 || !strings.HasPrefix(row[0].text, " :") {
		t.Errorf("the name row should show the shortcode: %+v", row)
	}
	key(tcell.KeyEnter, 0)
	if view.picker != nil || view.input.GetText() != "hmm "+want {
		t.Errorf("enter should insert %s, got %q", want, view.input.GetText())
	}
}

func TestEmojiGridTabsAndScroll(t *testing.T) {
	view := emojiView(t)
	view.OpenEmojiPicker()
	p := view.picker
	key := func(k tcell.Key, r rune) { p.OnKeyEvent(tcell.NewEventKey(k, r, tcell.ModNone)) }
	key(tcell.KeyRune, '/')

	key(tcell.KeyBacktab, 0)
	if p.cat != 0 || p.gridItems()[0] != quickReactions[0] {
		t.Errorf("shift+tab from smileys goes to recent: cat=%d", p.cat)
	}
	key(tcell.KeyBacktab, 0)
	if p.cat != len(emoji.Categories()) {
		t.Errorf("tabs wrap around to flags: cat=%d", p.cat)
	}
	key(tcell.KeyTab, 0)
	key(tcell.KeyTab, 0)
	for i := 0; i < gridRows+2; i++ {
		key(tcell.KeyDown, 0)
	}
	if p.gridScroll != 3 || p.gridSel != (gridRows+2)*gridCols {
		t.Errorf("down past the last visible row scrolls: sel=%d scroll=%d", p.gridSel, p.gridScroll)
	}

	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(60, 20)
	p.Draw(sim, 0, 20, -1)
	h := p.h
	key(tcell.KeyTab, 0)
	p.Draw(sim, 0, 20, -1)
	if p.h != h || p.w != gridInner+2 {
		t.Errorf("the box keeps its size across tabs: h %d→%d, w=%d", h, p.h, p.w)
	}

	key(tcell.KeyRune, 'a')
	if p.gridMode() {
		t.Error("typing leaves the grid for search results")
	}
	key(tcell.KeyBackspace2, 0)
	key(tcell.KeyBackspace2, 0)
	if p.searching {
		t.Error("backspace on an empty query still returns to the quick row")
	}
}

func TestEmojiGridClicks(t *testing.T) {
	view := emojiView(t)
	view.OpenEmojiPicker()
	p := view.picker
	p.OnKeyEvent(tcell.NewEventKey(tcell.KeyRune, '/', tcell.ModNone))
	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(60, 20)
	p.Draw(sim, 0, 20, -1)

	p.OnMouseEvent(p.slotX(3), p.y+2)
	if p.cat != 3 {
		t.Errorf("clicking the fourth tab selects it: cat=%d", p.cat)
	}
	want := p.gridItems()[gridCols+2]
	p.OnMouseEvent(p.slotX(2)+1, p.y+4)
	if view.picker != nil || view.input.GetText() != want {
		t.Errorf("clicking a cell inserts it: want %s, got %q", want, view.input.GetText())
	}
}

func TestDrawEmojiCellKeepsClustersTogether(t *testing.T) {
	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(10, 1)
	cell := func(x int) (rune, []rune) {
		r, comb, _, _ := sim.GetContent(x, 0)
		return r, comb
	}
	drawEmojiCell(sim, 0, 0, "🧑‍🚒", tcell.StyleDefault)
	if r, comb := cell(0); r != '🧑' || string(comb) != "\u200d🚒" {
		t.Errorf("a ZWJ sequence stays one cluster: %q %q", r, comb)
	}
	drawEmojiCell(sim, 3, 0, "🇮🇳", tcell.StyleDefault)
	if r, _ := cell(3); r != '🇮' {
		t.Errorf("flag first half: %q", r)
	}
	if r, _ := cell(4); r != '🇳' {
		t.Errorf("flag second half: %q", r)
	}
	drawEmojiCell(sim, 6, 0, "❤️", tcell.StyleDefault)
	if r, comb := cell(6); r != '❤' || len(comb) != 0 {
		t.Errorf("a narrow base is drawn bare: %q %q", r, comb)
	}
}

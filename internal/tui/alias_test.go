package tui

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/util/jsontime"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
	"github.com/whyyor/tuimessenger/internal/hicli/jsoncmd"
	"github.com/whyyor/tuimessenger/internal/rpc/client"
	"github.com/whyyor/tuimessenger/internal/rpc/store"
	"github.com/whyyor/tuimessenger/internal/tui/config"
)

// aliasMain is a store with Mariana (alias "sis") and Peter, newest first.
func aliasMain(t *testing.T) *MainView {
	t.Helper()
	gs := store.NewStore()
	room := func(roomID id.RoomID, name string, age time.Duration, ad map[event.Type]*database.AccountData) *jsoncmd.SyncRoom {
		return &jsoncmd.SyncRoom{
			Meta: &database.Room{ID: roomID, Name: &name,
				SortingTimestamp: jsontime.UM(time.Now().Add(-age))},
			AccountData: ad,
		}
	}
	gs.ApplySync(&jsoncmd.SyncComplete{Rooms: map[id.RoomID]*jsoncmd.SyncRoom{
		"!mariana:x": room("!mariana:x", "Mariana", time.Hour, map[event.Type]*database.AccountData{
			AccountDataAlias: {Type: AccountDataAlias.Type, Content: []byte(`{"alias":"sis"}`)},
		}),
		"!peter:x": room("!peter:x", "Peter", time.Minute, nil),
	}})
	cfg := &config.Config{}
	main := &MainView{matrix: &client.Client{Store: gs}, config: cfg, parent: &App{}}
	main.roomList = &RoomList{parent: main}
	if len(gs.ReversedRoomList.Current()) != 2 {
		t.Fatalf("test rooms missing from the room list: %d", len(gs.ReversedRoomList.Current()))
	}
	return main
}

func switcherNames(fs *FuzzySearchModal) []string {
	var names []string
	for _, m := range fs.matches {
		names = append(names, fs.roomList[m.OriginalIndex].Name)
	}
	return names
}

func TestSwitcherFindsChatByAlias(t *testing.T) {
	main := aliasMain(t)
	fs := NewFuzzySearchModal(main, 42, 12)
	if chatAlias(main.matrix.GetRoom("!mariana:x")) != "sis" {
		t.Fatal("alias not read from account data")
	}
	fs.changeHandler("sis")
	if got := switcherNames(fs); len(got) != 1 || got[0] != "Mariana" {
		t.Errorf("sis should find Mariana by alias, got %v", got)
	}
	fs.changeHandler("mari")
	if got := switcherNames(fs); len(got) == 0 || got[0] != "Mariana" {
		t.Errorf("the real name still works, got %v", got)
	}
	// "s" prefixes the alias but is only a fuzzy hit inside other names.
	fs.changeHandler("s")
	if got := switcherNames(fs); len(got) == 0 || got[0] != "Mariana" {
		t.Errorf("an alias prefix ranks first, got %v", got)
	}
}

func drawSwitcher(fs *FuzzySearchModal) string {
	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(40, 4)
	(&fuzzyResultsView{parent: fs}).Draw(sim)
	return strings.Join(screenText(sim, 40, 4), "\n")
}

func TestAliasShownDimAndHideable(t *testing.T) {
	main := aliasMain(t)
	fs := NewFuzzySearchModal(main, 42, 12)
	fs.changeHandler("sis")
	if text := drawSwitcher(fs); !strings.Contains(text, "Mariana · sis") {
		t.Errorf("alias should follow the name:\n%s", text)
	}

	main.config.Preferences.HideAliases = true
	fs = NewFuzzySearchModal(main, 42, 12)
	fs.changeHandler("sis")
	if got := switcherNames(fs); len(got) != 1 {
		t.Errorf("hidden aliases still match in search, got %v", got)
	}
	if text := drawSwitcher(fs); strings.Contains(text, "sis") {
		t.Errorf("hidden aliases are not drawn:\n%s", text)
	}
}

func TestPaletteMatchesAlias(t *testing.T) {
	p := &CommandPalette{}
	p.items = []*paletteItem{
		{section: sectionChats, title: "Mariana", alias: "sis"},
		{section: sectionChats, title: "Sisterhood - SIS"},
		{section: sectionChats, title: "Peter"},
	}
	p.query = []rune("@sis")
	p.refilter()
	var got []string
	for _, r := range p.rows {
		if r.item != nil {
			got = append(got, r.item.title)
		}
	}
	if len(got) != 2 || !slices.Contains(got, "Mariana") || slices.Contains(got, "Peter") {
		t.Errorf("the alias and the name both match, Peter doesn't: %v", got)
	}
	p.query = []rune("@sis ")
	p.items[1].title = "Old SISters group"
	p.refilter()
	if p.rows[0].item.title != "Mariana" {
		t.Errorf("an exact alias beats a mid-word name match: %v", p.rows[0].item.title)
	}
}

func TestWriteNameAliasTruncation(t *testing.T) {
	draw := func(w int, name, alias string) string {
		sim := tcell.NewSimulationScreen("")
		_ = sim.Init()
		sim.SetSize(30, 1)
		writeNameAlias(sim, 0, 0, w, name, alias, tcell.StyleDefault, aliasDim(tcell.StyleDefault))
		return screenText(sim, 30, 1)[0]
	}
	if got := draw(20, "Mariana", "sis"); got != "Mariana · sis" {
		t.Errorf("roomy: %q", got)
	}
	if got := draw(12, "Mariana", "sister"); got != "Mariana · s…" {
		t.Errorf("the alias is cut first: %q", got)
	}
	if got := draw(10, "Mariana", "sis"); got != "Mariana" {
		t.Errorf("under five cells the alias is dropped: %q", got)
	}
	if got := draw(5, "Mariana", "sis"); got != "Mari…" {
		t.Errorf("the name still truncates: %q", got)
	}
}

func TestSwitcherSkipsOpenChat(t *testing.T) {
	main := aliasMain(t)
	main.currentRoom = &RoomView{Room: main.matrix.GetRoom("!peter:x")}
	fs := NewFuzzySearchModal(main, 42, 12)
	if got := switcherNames(fs); len(got) != 1 || got[0] != "Mariana" {
		t.Errorf("the open chat should be left out, got %v", got)
	}
	fs.changeHandler("peter")
	if got := switcherNames(fs); len(got) != 0 {
		t.Errorf("searching for the open chat finds nothing, got %v", got)
	}
	if n := len(main.matrix.ReversedRoomList.Current()); n != 2 {
		t.Errorf("the store's room list must not change: %d", n)
	}
}

func TestSwitcherIsAPanelWithPillTitle(t *testing.T) {
	fs := NewFuzzySearchModal(aliasMain(t), 46, 16)
	const w, h = 60, 18
	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(w, h)
	fs.Draw(sim)
	lines := screenText(sim, w, h)
	text := strings.Join(lines, "\n")
	for _, gone := range []string{"┌", "└", "╭", "│", "▀", "▄", "rooms", "room>"} {
		if strings.Contains(text, gone) {
			t.Errorf("no border, half-rows or stock title (%q):\n%s", gone, text)
		}
	}
	bgAt := func(x, y int) tcell.Color {
		_, _, st, _ := sim.GetContent(x, y)
		_, bg, _ := st.Decompose()
		return bg
	}
	find := func(s string) int {
		for y, l := range lines {
			if strings.Contains(l, s) {
				return y
			}
		}
		t.Fatalf("no %q:\n%s", s, text)
		return -1
	}
	top := find("Chats")
	left, right := -1, -1
	for x := 0; x < w; x++ {
		if bgAt(x, top) == ColorPanel {
			if left < 0 {
				left = x
			}
			right = x
		}
	}
	if left < 0 {
		t.Fatal("the pill sits inside the panel's top row")
	}
	col := len([]rune(lines[top][:strings.Index(lines[top], "Chats")]))
	if bgAt(col, top) != ColorTitlePill || ColorTitlePill != ColorSelectionBackground {
		t.Error("the title is a pill in the selection's red")
	}
	if centre := (left + right) / 2; col < centre-4 || col > centre+2 {
		t.Errorf("the pill is centred: %d in %d..%d", col, left, right)
	}
	// One row of space everywhere: under the pill, above the prompt, and a
	// padding row under it. No caption.
	if strings.Contains(text, "ctrl+p peek") || strings.Contains(text, "enter open") {
		t.Errorf("no keys caption:\n%s", text)
	}
	if strings.TrimSpace(lines[top+1]) != "" || bgAt(left, top+1) != ColorPanel {
		t.Errorf("a blank panel row under the pill: %q", lines[top+1])
	}
	prompt := find("\U000f0349")
	if strings.Contains(text, "2 / 2") {
		t.Error("no match count")
	}
	// The panel pads one column; the magnifier sits one left of the chat icons.
	if x := len([]rune(lines[prompt][:strings.Index(lines[prompt], "\U000f0349")])); x != left+2 {
		t.Errorf("the magnifier sits just left of the chat icons: col %d, panel from %d", x, left)
	}
	if strings.TrimSpace(lines[prompt-1]) != "" {
		t.Errorf("a blank row above the prompt: %q", lines[prompt-1])
	}
	if r, _, _, _ := sim.GetContent(left+1, prompt-2); r != '▌' {
		t.Error("the best match sits nearest the prompt, selected")
	}
	if _, _, st, _ := sim.GetContent(left+5, prompt); func() bool { _, bg, _ := st.Decompose(); return bg != ColorPanel }() {
		t.Error("the prompt is on the panel's own background")
	}
	if bgAt(left, prompt+1) != ColorPanel || strings.TrimSpace(lines[prompt+1]) != "" || bgAt(left, prompt+2) == ColorPanel {
		t.Error("one padding row under the prompt")
	}
}

func TestSwitcherArrowsFollowTheScreen(t *testing.T) {
	main := aliasMain(t)
	main.config.Keybindings = config.ParsedKeybindings{Modal: map[config.Keybind]string{
		{Key: tcell.KeyDown}: "select_next", {Key: tcell.KeyUp}: "select_prev",
	}}
	fs := NewFuzzySearchModal(main, 46, 16)
	fs.OnKeyEvent(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone))
	if fs.selected != 1 {
		t.Errorf("↑ moves up the list to the next match, got %d", fs.selected)
	}
	fs.OnKeyEvent(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	if fs.selected != 0 {
		t.Errorf("↓ goes back toward the prompt, got %d", fs.selected)
	}
}

func TestSwitcherFitsSmallScreens(t *testing.T) {
	fs := NewFuzzySearchModal(aliasMain(t), 60, 18)
	for _, size := range [][2]int{{100, 30}, {40, 12}} {
		sim := tcell.NewSimulationScreen("")
		_ = sim.Init()
		sim.SetSize(size[0], size[1])
		fs.Draw(sim)
		left, right := -1, -1
		for x := 0; x < size[0]; x++ {
			_, _, st, _ := sim.GetContent(x, size[1]/2)
			if _, bg, _ := st.Decompose(); bg == ColorPanel || bg == ColorBarBackground {
				if left < 0 {
					left = x
				}
				right = x
			}
		}
		want := min(60, size[0]-4)
		if right-left+1 != want || left < 2 {
			t.Errorf("%dx%d: panel %d..%d, want %d wide inside the edges", size[0], size[1], left, right, want)
		}
	}
}

func paletteTitles(main *MainView) []string {
	var titles []string
	for _, item := range (&CommandPalette{parent: main}).commandItems() {
		titles = append(titles, item.title)
	}
	return titles
}

func hasTitle(titles []string, prefix string) bool {
	return slices.ContainsFunc(titles, func(s string) bool { return strings.HasPrefix(s, prefix) })
}

func TestPaletteHasEveryCommand(t *testing.T) {
	main := aliasMain(t)
	// No chat open: the commands that act on every chat are still there.
	none := paletteTitles(main)
	for _, want := range []string{"Mute sync", "Mute prune", "Delete played media cache", "Quit"} {
		if !hasTitle(none, want) {
			t.Errorf("without a chat open, missing %q", want)
		}
	}
	if hasTitle(none, "Remove alias") || hasTitle(none, "Open media") {
		t.Error("chat commands need a chat")
	}
	// A chat with an alias open: the chat commands too.
	main.currentRoom = &RoomView{Room: main.matrix.GetRoom("!mariana:x"), parent: main}
	open := paletteTitles(main)
	for _, want := range []string{"Remove alias (sis)", "Change alias (sis)", "Open media with the system app", "Mute sync"} {
		if !hasTitle(open, want) {
			t.Errorf("with a chat open, missing %q", want)
		}
	}
}

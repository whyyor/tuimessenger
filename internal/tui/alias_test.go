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

	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
	"go.mau.fi/gomuks/pkg/rpc/client"
	"go.mau.fi/gomuks/pkg/rpc/store"
	"go.mau.fi/gomuks/tui/config"
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
	main := &MainView{matrix: &client.GomuksClient{GomuksStore: gs}, config: cfg, parent: &GomuksTUI{}}
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
	fs.changeHandler("mans")
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
	if got := draw(5, "Mariana", "sis"); got != "Mans…" {
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

func TestSwitcherUsesOurFrame(t *testing.T) {
	fs := NewFuzzySearchModal(aliasMain(t), 42, 12)
	const w, h = 60, 16
	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(w, h)
	fs.Draw(sim)
	lines := screenText(sim, w, h)
	text := strings.Join(lines, "\n")
	for _, want := range []string{"╭─", "Chats", "╰", " › "} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "rooms") || strings.Contains(text, "room>") {
		t.Errorf("the stock title and prompt are gone:\n%s", text)
	}
	for y, line := range lines {
		if x := strings.Index(line, "Chats"); x >= 0 {
			_, _, st, _ := sim.GetContent(len([]rune(line[:x])), y)
			if fg, _, _ := st.Decompose(); fg != ColorPillYellow {
				t.Errorf("title in the popups' yellow, got %v", fg)
			}
		}
	}
}

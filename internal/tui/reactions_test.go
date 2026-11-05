package tui

import (
	"encoding/json"
	"testing"

	"github.com/gdamore/tcell/v2"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/cmdspec"
	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/tui/lib/emoji"
	"go.mau.fi/gomuks/tui/messages"
)

func reactionEvt(sender id.UserID, evtID id.EventID, key string, redacted bool) *database.Event {
	content, _ := json.Marshal(&event.ReactionEventContent{RelatesTo: event.RelatesTo{
		Type: event.RelAnnotation, EventID: "$msg", Key: key,
	}})
	evt := &database.Event{ID: evtID, Sender: sender, Type: event.EventReaction.Type, Content: content}
	if redacted {
		evt.RedactedBy = "$redaction"
	}
	return evt
}

func TestOwnReactionKeys(t *testing.T) {
	evts := []*database.Event{
		reactionEvt("@me:x", "$1", "👍", false),
		reactionEvt("@other:x", "$2", "❤️", false),
		reactionEvt("@me:x", "$3", "😂", true),
	}
	got := ownReactionKeys(evts, "@me:x")
	if len(got) != 1 || got["👍"] != "$1" {
		t.Errorf("got %v, want only 👍 → $1", got)
	}
}

func TestSameReaction(t *testing.T) {
	if !sameReaction("❤", "❤️") || sameReaction("👍", "❤️") {
		t.Error("variation selectors must not matter, different emoji must")
	}
}

func TestFormatReactions(t *testing.T) {
	got := messages.FormatReactions(map[string]int{"❤️": 1, "👍": 2, "😂": 0})
	if got != "👍 2  ❤️" {
		t.Errorf("got %q", got)
	}
}

func TestEmojiSearchRanksPrefixFirst(t *testing.T) {
	res := emoji.Search("heart", 50)
	if len(res) == 0 || res[0].Shortcode[:5] != "heart" {
		t.Fatalf("prefix match should lead: %v", res)
	}
	seen := map[string]bool{}
	for _, m := range res {
		if seen[m.Emoji] {
			t.Fatalf("duplicate emoji %s", m.Emoji)
		}
		seen[m.Emoji] = true
	}
	if emoji.Search("", 10) != nil {
		t.Error("empty query should give nothing")
	}
}

func TestReactPickerPlacement(t *testing.T) {
	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(80, 20)
	p := &ReactPicker{}
	// Room above the message: sits directly over it.
	p.Draw(sim, 5, 10, 11)
	if p.y != 7 || p.h != 3 || p.x != 5 {
		t.Errorf("above: x=%d y=%d h=%d", p.x, p.y, p.h)
	}
	// No room above: goes below.
	p.Draw(sim, 5, 1, 2)
	if p.y != 3 {
		t.Errorf("below: y=%d", p.y)
	}
	// Anchor near the right edge is pulled back inside.
	p.Draw(sim, 79, 10, 11)
	if p.x+p.w > 80 {
		t.Errorf("overflows: x=%d w=%d", p.x, p.w)
	}
}

func TestReactPickerKeysSwitchToSearch(t *testing.T) {
	p := &ReactPicker{}
	p.OnKeyEvent(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))
	if p.sel != 1 {
		t.Fatalf("→ should move to slot 2, got %d", p.sel)
	}
	p.OnKeyEvent(tcell.NewEventKey(tcell.KeyRune, 'f', tcell.ModNone))
	if !p.searching || p.query != "f" || len(p.results) == 0 {
		t.Fatalf("a letter should start a search: %+v", p)
	}
	p.OnKeyEvent(tcell.NewEventKey(tcell.KeyBackspace2, 0, tcell.ModNone))
	p.OnKeyEvent(tcell.NewEventKey(tcell.KeyBackspace2, 0, tcell.ModNone))
	if p.searching {
		t.Error("backspace on an empty query should return to the quick row")
	}
}

func TestBareReactParses(t *testing.T) {
	for _, cmd := range LocalCommands {
		if cmd.Command == CmdReact {
			parsed, err := cmd.ParseInput(cmdspec.FakeGomuksSender, cmdSigils, "/react")
			if err != nil || parsed == nil {
				t.Fatalf("bare /react: %v", err)
			}
			return
		}
	}
}

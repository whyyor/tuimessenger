package tui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/mauview"

	"go.mau.fi/gomuks/pkg/rpc"
)

func TestEmptyHintShowsWithSidebar(t *testing.T) {
	main := aliasMain(t)
	main.flex = mauview.NewFlex().SetDirection(mauview.FlexColumn)
	main.roomView = mauview.NewBox(nil).SetBorder(false)
	main.flex.AddFixedComponent(main.roomList, roomListWidth).AddProportionalComponent(main.roomView, 1)
	main.config.Preferences.HideRoomList = false
	main.parent.ConnState = rpc.ConnStateConnected // no "connecting…" dialog over the hint

	const w, h = 100, 24
	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(w, h)
	main.Draw(sim)
	found := false
	for y, line := range screenText(sim, w, h) {
		if i := strings.Index(line, "Ctrl+k"); i >= 0 {
			found = true
			if col := len([]rune(line[:i])); col < roomListWidth {
				t.Errorf("hint on row %d starts inside the sidebar (col %d)", y, col)
			}
		}
	}
	if !found {
		t.Fatalf("no hint with the sidebar open:\n%s", strings.Join(screenText(sim, w, h), "\n"))
	}

	main.hintRows = []hintRow{{y: 3, roomID: "!mariana:x"}}
	if main.hintClick(click{tcell.NewEventMouse(5, 3, tcell.Button1, 0)}) {
		t.Error("a click on the sidebar isn't a hint click")
	}
}

func TestEmptyHintFitsNarrowArea(t *testing.T) {
	main := aliasMain(t)
	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(50, 10)
	main.drawEmptyHint(sim)
	if text := strings.Join(screenText(sim, 50, 10), "\n"); !strings.Contains(text, "Ctrl+p commands") {
		t.Errorf("a narrow area gets the short hint, uncut:\n%s", text)
	}
}

// click is a mouse press without motion, as mauview delivers it.
type click struct{ *tcell.EventMouse }

func (click) HasMotion() bool { return false }

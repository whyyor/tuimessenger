package tui

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/util/jsontime"

	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/pkg/rpc/store"
)

func evtAt(t time.Time) *database.Event {
	return &database.Event{Timestamp: jsontime.UM(t)}
}

func TestHiddenByClear(t *testing.T) {
	cleared := time.UnixMilli(1_000_000)
	if !hiddenByClear(evtAt(cleared.Add(-time.Second)), cleared) {
		t.Error("older message should be hidden")
	}
	if hiddenByClear(evtAt(cleared.Add(time.Second)), cleared) {
		t.Error("newer message should show")
	}
	if hiddenByClear(evtAt(cleared.Add(-time.Hour)), time.Time{}) {
		t.Error("never cleared: nothing hidden")
	}
}

func TestHistoryExhaustedStopsPagination(t *testing.T) {
	cleared := time.UnixMilli(1_000_000)
	old, recent := evtAt(cleared.Add(-time.Minute)), evtAt(cleared.Add(time.Minute))
	if !historyExhausted([]*database.Event{old, recent}, cleared) {
		t.Error("oldest loaded is hidden: should stop paginating")
	}
	if historyExhausted([]*database.Event{recent}, cleared) {
		t.Error("only visible messages loaded: older ones may still be visible")
	}
	if historyExhausted(nil, cleared) {
		t.Error("empty timeline: must load at least once")
	}
	if historyExhausted([]*database.Event{old}, time.Time{}) {
		t.Error("never cleared: always paginate")
	}
}

func TestListedRooms(t *testing.T) {
	cleared := time.UnixMilli(1_000_000)
	rooms := []*store.RoomListEntry{
		{RoomID: "never-cleared", SortingTimestamp: cleared.Add(-time.Hour)},
		{RoomID: "cleared-quiet", SortingTimestamp: cleared.Add(-time.Minute), ClearedAt: cleared},
		{RoomID: "cleared-new-msg", SortingTimestamp: cleared.Add(time.Minute), ClearedAt: cleared},
		{RoomID: "cleared-open", SortingTimestamp: cleared.Add(-time.Minute), ClearedAt: cleared},
	}
	got := listedRooms(rooms, "cleared-open")
	var ids []string
	for _, r := range got {
		ids = append(ids, string(r.RoomID))
	}
	want := []string{"never-cleared", "cleared-new-msg", "cleared-open"}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", ids, want)
	}
	if len(rooms) != 4 || rooms[1].RoomID != "cleared-quiet" {
		t.Error("listedRooms modified the shared room list")
	}
}

func TestClearStateHidden(t *testing.T) {
	var state store.ClearState
	state, changed := state.WithHidden("$a")
	if !changed || !state.IsHidden("$a") || state.IsHidden("$b") {
		t.Fatalf("hide $a: %+v changed=%v", state, changed)
	}
	if _, changed = state.WithHidden("$a"); changed {
		t.Error("hiding twice should be a no-op")
	}
	if _, changed = state.WithHidden(""); changed {
		t.Error("a message without an ID can't be hidden")
	}
	if !state.At().IsZero() {
		t.Error("hiding a message must not set a clear point")
	}
}

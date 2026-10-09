package tui

import (
	"testing"
	"time"

	"go.mau.fi/util/jsontime"

	"go.mau.fi/gomuks/pkg/hicli/database"
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

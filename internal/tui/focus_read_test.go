package tui

import (
	"testing"
	"time"
)

func TestAttention(t *testing.T) {
	now := time.Now()
	a := &attention{}
	if a.looking(now) {
		t.Error("no focus info and no activity: not looking")
	}
	a.touch()
	if !a.looking(time.Now()) {
		t.Error("a key just now counts when focus is unknown")
	}
	if a.looking(time.Now().Add(activityWindow + time.Second)) {
		t.Error("stale activity doesn't count")
	}
	a.setFocus(false)
	if a.looking(time.Now()) {
		t.Error("terminal reports unfocused: not looking, even right after a key")
	}
	a.setFocus(true)
	if !a.looking(time.Now().Add(time.Hour)) {
		t.Error("focused terminal: looking, however long since the last key")
	}
}

func TestFocusEventReachesAttention(t *testing.T) {
	orig := userAttention
	userAttention = &attention{}
	defer func() { userAttention = orig }()
	gained := 0
	origGained := onFocusGained
	onFocusGained = func() { gained++ }
	defer func() { onFocusGained = origGained }()

	br := &bufferedRoot{}
	br.OnFocusEvent(false)
	br.OnFocusEvent(true)
	if !userAttention.looking(time.Now()) || gained != 1 {
		t.Errorf("focus in should count as looking and read once; looking=%v gained=%d", userAttention.looking(time.Now()), gained)
	}
}

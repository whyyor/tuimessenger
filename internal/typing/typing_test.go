package typing

import (
	"testing"
	"time"
)

func TestSetAndExpire(t *testing.T) {
	tr := New()
	start := tr.Version()
	changed := tr.Changes()
	tr.Set("!room", "@alice", 50*time.Millisecond)
	select {
	case <-changed:
	default:
		t.Fatal("starting to type should notify")
	}
	if users := tr.Current("!room"); len(users) != 1 {
		t.Fatalf("Current = %v", users)
	}
	v := tr.Version()
	tr.Set("!room", "@alice", 50*time.Millisecond)
	if tr.Version() != v {
		t.Error("still typing should not count as a change")
	}
	if rooms := tr.ChangedSince(start); len(rooms["!room"]) != 1 {
		t.Errorf("ChangedSince = %v", rooms)
	}
	time.Sleep(150 * time.Millisecond)
	if users := tr.Current("!room"); len(users) != 0 {
		t.Errorf("typing should expire, got %v", users)
	}
	if rooms := tr.ChangedSince(v); len(rooms["!room"]) != 0 || tr.Version() == v {
		t.Errorf("expiry should be a change with an empty list: %v", rooms)
	}
}

func TestStop(t *testing.T) {
	tr := New()
	tr.Set("!room", "@alice", time.Minute)
	tr.Set("!room", "@alice", 0)
	if users := tr.Current("!room"); len(users) != 0 {
		t.Errorf("stopped typing still listed: %v", users)
	}
	v := tr.Version()
	tr.Set("!room", "@bob", 0)
	if tr.Version() != v {
		t.Error("stopping when not typing should not count as a change")
	}
}

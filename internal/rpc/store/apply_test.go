package store

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
)

// An edit that arrives while the message is on screen is what the message shows, not the message itself
// (upstream 6a7f21bf).
func TestLiveEditIsShown(t *testing.T) {
	rs := NewRoomStore(&Store{}, &database.Room{ID: "!r:x"})
	editRowID := database.EventRowID(2)
	orig := &database.Event{RowID: 1, ID: "$orig:x", RoomID: "!r:x", Type: "m.room.message",
		Content: json.RawMessage(`{"msgtype":"m.text","body":"old"}`), LastEditRowID: &editRowID}
	rs.ApplyFetched(orig)
	edit := &database.Event{RowID: editRowID, ID: "$edit:x", RoomID: "!r:x", Type: "m.room.message",
		RelationType: event.RelReplace, RelatesTo: "$orig:x",
		Content: json.RawMessage(`{"msgtype":"m.text","body":"* new","m.new_content":{"msgtype":"m.text","body":"new"}}`)}
	rs.ApplyFetched(edit)
	if got := rs.GetEventByID("$orig:x").LastEditRef; got != edit {
		t.Fatalf("the message should point at its edit, got %+v", got)
	}
}

// Room state can arrive while the TUI reads the room (upstream 10f8da23). Run with -race.
func TestApplyStateWhileReading(t *testing.T) {
	rs := NewRoomStore(&Store{}, &database.Room{ID: "!r:x"})
	member := func(i int) *database.Event {
		key := fmt.Sprintf("@u%d:x", i)
		return &database.Event{RowID: database.EventRowID(i + 1), ID: id.EventID(fmt.Sprintf("$s%d:x", i)),
			RoomID: "!r:x", Type: event.StateMember.Type, StateKey: &key, Content: json.RawMessage(`{"membership":"join"}`)}
	}
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < 300; i++ {
			rs.ApplyState(member(i))
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 30; i++ {
			rs.ApplyFullState([]*database.Event{member(i), member(i + 1)}, false)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 300; i++ {
			rs.GetEventByID(id.EventID(fmt.Sprintf("$s%d:x", i)))
			rs.GetStateEvent(event.StateMember, fmt.Sprintf("@u%d:x", i))
		}
	}()
	wg.Wait()
}

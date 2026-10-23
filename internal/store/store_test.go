package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

const (
	room  = id.RoomID("!room:test")
	alice = id.UserID("@whatsapp_alice:test")
)

func openTemp(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "store.db")
	s, err := Open(context.Background(), path, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

func member(eventID id.EventID, name string) *event.Event {
	key := alice.String()
	return &event.Event{
		ID: eventID, RoomID: room, Sender: alice, Type: event.StateMember, StateKey: &key,
		Content: event.Content{Parsed: &event.MemberEventContent{Membership: event.MembershipJoin, Displayname: name}},
	}
}

func TestWriteAndRead(t *testing.T) {
	ctx := context.Background()
	s, path := openTemp(t)
	changed := s.Changes()

	err := s.Write(ctx, func(ctx context.Context, w *Writer) error {
		if _, err := w.AddEvent(ctx, member("$m1", "Alice")); err != nil {
			return err
		}
		_, err := w.AddEvent(ctx, &event.Event{
			ID: "$msg", RoomID: room, Sender: alice, Type: event.EventMessage,
			Content: event.Content{Parsed: &event.MessageEventContent{MsgType: event.MsgText, Body: "hi"}},
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.LastPos() != 2 {
		t.Errorf("LastPos = %d, want 2", s.LastPos())
	}
	select {
	case <-changed:
	default:
		t.Error("Changes channel was not closed after a write")
	}

	msg, err := s.GetEvent(ctx, "$msg")
	if err != nil || msg == nil {
		t.Fatalf("GetEvent: %v %v", msg, err)
	}
	if err = msg.Content.ParseRaw(msg.Type); err != nil {
		t.Fatal(err)
	}
	if body := msg.Content.AsMessage().Body; body != "hi" || msg.Pos != 2 || msg.Unsigned.BeeperHSOrder != 2 {
		t.Errorf("got body %q pos %d order %d", body, msg.Pos, msg.Unsigned.BeeperHSOrder)
	}
	if missing, err := s.GetEvent(ctx, "$nope"); missing != nil || err != nil {
		t.Errorf("missing event: %v %v", missing, err)
	}

	// A newer member event replaces the current state.
	if err = s.Write(ctx, func(ctx context.Context, w *Writer) error {
		_, err := w.AddEvent(ctx, member("$m2", "Alice 2"))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	state, err := s.GetState(ctx, room, event.StateMember, alice.String())
	if err != nil || state == nil || state.ID != "$m2" {
		t.Fatalf("GetState: %v %v", state, err)
	}
	members, err := s.GetStateOfType(ctx, room, event.StateMember)
	if err != nil || len(members) != 1 {
		t.Errorf("GetStateOfType: %d members, %v", len(members), err)
	}

	// A failed write saves nothing and does not move the position.
	failure := errors.New("boom")
	err = s.Write(ctx, func(ctx context.Context, w *Writer) error {
		if _, err := w.AddEvent(ctx, &event.Event{ID: "$lost", RoomID: room, Sender: alice, Type: event.EventMessage}); err != nil {
			return err
		}
		return failure
	})
	if !errors.Is(err, failure) || s.LastPos() != 3 {
		t.Errorf("failed write: err %v, LastPos %d", err, s.LastPos())
	}
	if lost, _ := s.GetEvent(ctx, "$lost"); lost != nil {
		t.Error("event from a failed write was saved")
	}

	// Reopening the file remembers the last position.
	_ = s.Close()
	s2, err := Open(ctx, path, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if s2.LastPos() != 3 {
		t.Errorf("after reopen LastPos = %d, want 3", s2.LastPos())
	}
}

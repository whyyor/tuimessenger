package store

import (
	"context"
	"encoding/json"
	"testing"

	"maunium.net/go/mautrix/event"
)

func TestAccountData(t *testing.T) {
	ctx := context.Background()
	s, _ := openTemp(t)

	mute := func(until int64) error {
		return s.Write(ctx, func(ctx context.Context, w *Writer) error {
			return w.SetAccountData(ctx, room, event.AccountDataBeeperMute, &event.BeeperMuteEventContent{MutedUntil: until})
		})
	}
	if err := mute(100); err != nil {
		t.Fatal(err)
	}
	if err := mute(200); err != nil {
		t.Fatal(err)
	}

	raw, err := s.GetAccountData(ctx, room, event.AccountDataBeeperMute)
	var content event.BeeperMuteEventContent
	if err != nil || json.Unmarshal(raw, &content) != nil || content.MutedUntil != 200 {
		t.Fatalf("GetAccountData: %s %v", raw, err)
	}
	if missing, err := s.GetAccountData(ctx, "", event.AccountDataPushRules); missing != nil || err != nil {
		t.Errorf("missing account data: %s %v", missing, err)
	}

	list, err := s.AccountDataSince(ctx, 0)
	if err != nil || len(list) != 1 || list[0].Pos != 2 {
		t.Fatalf("AccountDataSince(0): %+v %v", list, err)
	}
	if list, _ = s.AccountDataSince(ctx, 2); len(list) != 0 {
		t.Errorf("AccountDataSince(2) = %d items, want 0", len(list))
	}
}

func TestProfilesAndStateByKey(t *testing.T) {
	ctx := context.Background()
	s, _ := openTemp(t)

	var changed1, changed2 bool
	err := s.Write(ctx, func(ctx context.Context, w *Writer) (err error) {
		if changed1, err = w.SetProfile(ctx, alice, Profile{Displayname: "Alice"}); err != nil {
			return err
		}
		if changed2, err = w.SetProfile(ctx, alice, Profile{Displayname: "Alice"}); err != nil {
			return err
		}
		_, err = w.AddEvent(ctx, member("$m1", "Alice"))
		return err
	})
	if err != nil || !changed1 || changed2 {
		t.Fatalf("SetProfile: changed %v %v, err %v", changed1, changed2, err)
	}
	if p, err := s.GetProfile(ctx, alice); err != nil || p.Displayname != "Alice" {
		t.Errorf("GetProfile: %+v %v", p, err)
	}
	if p, err := s.GetProfile(ctx, "@nobody:test"); err != nil || p != (Profile{}) {
		t.Errorf("unknown profile: %+v %v", p, err)
	}
	memberships, err := s.GetStateByKey(ctx, event.StateMember, alice.String())
	if err != nil || len(memberships) != 1 || memberships[0].RoomID != room {
		t.Errorf("GetStateByKey: %v %v", memberships, err)
	}
}

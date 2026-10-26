package store

import (
	"context"
	"testing"

	"maunium.net/go/mautrix/event"
)

func TestReceipts(t *testing.T) {
	ctx := context.Background()
	s, _ := openTemp(t)

	err := s.Write(ctx, func(ctx context.Context, w *Writer) error {
		if err := w.SetReceipt(ctx, &Receipt{RoomID: room, UserID: alice, Type: event.ReceiptTypeRead, EventID: "$e1"}); err != nil {
			return err
		}
		return w.SetReceipt(ctx, &Receipt{RoomID: room, UserID: "@bob:test", Type: event.ReceiptTypeRead, EventID: "$e1"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.ReceiptSince(ctx, 0); err != nil || len(got) != 2 {
		t.Fatalf("after 2 receipts: got %d, err %v", len(got), err)
	}

	// Alice reads a newer message: her receipt is replaced, not added.
	err = s.Write(ctx, func(ctx context.Context, w *Writer) error {
		return w.SetReceipt(ctx, &Receipt{RoomID: room, UserID: alice, Type: event.ReceiptTypeRead, EventID: "$e2"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.ReceiptSince(ctx, 0); err != nil || len(got) != 2 {
		t.Fatalf("after update: got %d, err %v", len(got), err)
	}

	// Only Alice's update happened after position 2.
	got, err := s.ReceiptSince(ctx, 2)
	if err != nil || len(got) != 1 || got[0].UserID != alice || got[0].EventID != "$e2" || got[0].Pos != 3 {
		t.Fatalf("since 2: got %+v, err %v", got, err)
	}

	// A receipt without an event ID is refused and saves nothing.
	err = s.Write(ctx, func(ctx context.Context, w *Writer) error {
		return w.SetReceipt(ctx, &Receipt{RoomID: room, UserID: alice, Type: event.ReceiptTypeRead})
	})
	if err == nil || s.LastPos() != 3 {
		t.Errorf("invalid receipt: err %v, LastPos %d", err, s.LastPos())
	}
}

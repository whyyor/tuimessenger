package tui

import (
	"context"

	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
	"go.mau.fi/gomuks/pkg/rpc/store"
	"go.mau.fi/gomuks/tui/debug"
)

// receiptFetchLimit bounds how many loaded events we ask receipts for.
const receiptFetchLimit = 500

// loadReceipts refills read receipts for an opened chat from the local
// database, so ticks don't depend on gomuks having been watching at the time.
func (view *MainView) loadReceipts(room *store.RoomStore) {
	defer debug.Recover()
	ids := room.LoadedEventIDs(receiptFetchLimit)
	if len(ids) == 0 {
		return
	}
	receipts, err := view.matrix.GetReceipts(context.TODO(), &jsoncmd.GetReceiptsParams{RoomID: room.ID, EventIDs: ids})
	if err != nil {
		debug.Print("Failed to load receipts for", room.ID, err)
		return
	}
	room.ApplyFetchedReceipts(receipts)
	notePublicReceipts(room, receipts)
	view.parent.Render()
}

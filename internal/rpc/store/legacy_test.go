package store

import (
	"encoding/json"
	"testing"

	"maunium.net/go/mautrix/event"

	"github.com/whyyor/tuimessenger/internal/hicli/cmdspec"
	"github.com/whyyor/tuimessenger/internal/hicli/database"
)

func TestRenameLegacyAccountData(t *testing.T) {
	ad := func(name, value string) *database.AccountData {
		return &database.AccountData{Type: name, Content: json.RawMessage(`{"v":"` + value + `"}`)}
	}
	typ := func(name string) event.Type { return event.Type{Type: name} }
	nothingStored := func(event.Type) bool { return false }

	// Only the old name: filed under the new one.
	batch := map[event.Type]*database.AccountData{typ("com.whyyor.gomuks.alias"): ad("com.whyyor.gomuks.alias", "sis")}
	renameLegacyAccountData(batch, nothingStored)
	got := batch[event.Type{Type: "com.whyyor.tuimessenger.alias", Class: event.AccountDataEventType}]
	if len(batch) != 1 || got == nil || got.Type != "com.whyyor.tuimessenger.alias" || string(got.Content) != `{"v":"sis"}` {
		t.Errorf("old name alone: %+v", batch)
	}

	// Old and new in one batch: the new one wins.
	batch = map[event.Type]*database.AccountData{
		typ("fi.mau.gomuks.preferences"):           ad("fi.mau.gomuks.preferences", "old"),
		typ("com.whyyor.tuimessenger.preferences"): ad("com.whyyor.tuimessenger.preferences", "new"),
	}
	renameLegacyAccountData(batch, nothingStored)
	if len(batch) != 1 || string(batch[typ("com.whyyor.tuimessenger.preferences")].Content) != `{"v":"new"}` {
		t.Errorf("both in batch: %+v", batch)
	}

	// Old name arriving when the new one is already stored: dropped.
	batch = map[event.Type]*database.AccountData{typ("com.whyyor.gomuks.seen"): ad("com.whyyor.gomuks.seen", "old")}
	renameLegacyAccountData(batch, func(t event.Type) bool { return t.Type == "com.whyyor.tuimessenger.seen" })
	if len(batch) != 0 {
		t.Errorf("new already stored: %+v", batch)
	}

	// Anything else is left alone.
	batch = map[event.Type]*database.AccountData{typ("m.tag"): ad("m.tag", "fav")}
	renameLegacyAccountData(batch, nothingStored)
	if len(batch) != 1 || batch[typ("m.tag")] == nil {
		t.Errorf("unrelated: %+v", batch)
	}
}

func TestLegacyNames(t *testing.T) {
	if !cmdspec.IsFakeSender("@tuimessenger") || !cmdspec.IsFakeSender("@gomuks") || cmdspec.IsFakeSender("@me:x") {
		t.Error("the client's own notices must be recognised under both names, and nobody else")
	}
	if !database.IsPaginationComplete(database.PrevBatchPaginationComplete) ||
		!database.IsPaginationComplete("fi.mau.gomuks.pagination_complete") || database.IsPaginationComplete("t123") {
		t.Error("the history-complete marker must be recognised under both names")
	}
}

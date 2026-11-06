package store

import (
	"maunium.net/go/mautrix/event"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
)

// legacyAccountData maps the names the client's settings were saved under before the rename to the current ones.
var legacyAccountData = map[string]string{
	"com.whyyor.gomuks.alias":   "com.whyyor.tuimessenger.alias",
	"com.whyyor.gomuks.view":    "com.whyyor.tuimessenger.view",
	"com.whyyor.gomuks.privacy": "com.whyyor.tuimessenger.privacy",
	"com.whyyor.gomuks.seen":    "com.whyyor.tuimessenger.seen",
	"com.whyyor.gomuks.cleared": "com.whyyor.tuimessenger.cleared",
	"fi.mau.gomuks.preferences": "com.whyyor.tuimessenger.preferences",
}

// renameLegacyAccountData files settings that arrive under an old name under the new one, so the rest of the client
// only knows the new names. A new-name value in the same batch or already stored (have) always wins over an old one.
func renameLegacyAccountData(batch map[event.Type]*database.AccountData, have func(event.Type) bool) {
	for evtType, ad := range batch {
		newName, ok := legacyAccountData[evtType.Type]
		if !ok {
			continue
		}
		delete(batch, evtType)
		newType := event.Type{Type: newName, Class: event.AccountDataEventType}
		if batchHas(batch, newName) || have(newType) {
			continue
		}
		renamed := *ad
		renamed.Type = newName
		batch[newType] = &renamed
	}
}

// batchHas compares names only: types decoded from JSON map keys have no Class.
func batchHas(batch map[event.Type]*database.AccountData, name string) bool {
	for evtType := range batch {
		if evtType.Type == name {
			return true
		}
	}
	return false
}

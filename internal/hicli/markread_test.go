package hicli

import (
	"testing"

	"maunium.net/go/mautrix/event"
)

func TestReadMarkerRequests(t *testing.T) {
	pub, markers, err := readMarkerRequests("$e", event.ReceiptTypeRead)
	if err != nil || !pub || markers.Read != "" || markers.FullyRead != "$e" || markers.ReadPrivate != "" {
		t.Errorf("public: send /receipt, keep only fully_read in read_markers; got %v %+v %v", pub, markers, err)
	}
	pub, markers, err = readMarkerRequests("$e", event.ReceiptTypeReadPrivate)
	if err != nil || pub || markers.ReadPrivate != "$e" || markers.FullyRead != "$e" {
		t.Errorf("private: unchanged read_markers only; got %v %+v %v", pub, markers, err)
	}
	if _, _, err = readMarkerRequests("$e", "m.bogus"); err == nil {
		t.Error("unknown receipt types are rejected")
	}
}

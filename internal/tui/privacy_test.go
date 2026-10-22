package tui

import (
	"testing"

	"maunium.net/go/mautrix/event"

	"go.mau.fi/gomuks/pkg/hicli/cmdspec"
	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
)

func TestChatPrivacyDefaultsAndOptIn(t *testing.T) {
	_, room := bubbleView(t)
	if p := chatPrivacy(room); p.Typing || p.ReadReceipts {
		t.Fatalf("defaults must share nothing: %+v", p)
	}
	if got := receiptTypeFor(room); got != event.ReceiptTypeReadPrivate {
		t.Errorf("default receipt %q, want private", got)
	}
	room.ApplySync(&jsoncmd.SyncRoom{AccountData: map[event.Type]*database.AccountData{
		AccountDataPrivacy: {Type: AccountDataPrivacy.Type, Content: []byte(`{"read_receipts":true}`)},
	}})
	if p := chatPrivacy(room); p.Typing || !p.ReadReceipts {
		t.Errorf("seen only: %+v", p)
	}
	if got := receiptTypeFor(room); got != event.ReceiptTypeRead {
		t.Errorf("opted-in chat sends %q, want public m.read", got)
	}
}

func TestParseToggle(t *testing.T) {
	for _, c := range []struct {
		arg       string
		cur, want bool
	}{{"on", false, true}, {"OFF", true, false}, {"", false, true}, {"", true, false}, {"maybe", true, false}} {
		if got := parseToggle(c.arg, c.cur); got != c.want {
			t.Errorf("%q from %v: got %v", c.arg, c.cur, got)
		}
	}
}

func TestTypingAndSeenCommandsParse(t *testing.T) {
	for _, input := range []string{"/typing", "/typing on", "/seen off", "/seen"} {
		ok := false
		for _, cmd := range LocalCommands {
			if parsed, err := cmd.ParseInput(cmdspec.FakeGomuksSender, cmdSigils, input); parsed != nil && err == nil {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf("%q didn't parse", input)
		}
	}
}

func TestTypingNotSentByDefault(t *testing.T) {
	view, room := bubbleView(t)
	rv := &RoomView{Room: room, config: view.config}
	// No matrix client: sending would panic, so this proves nothing is sent.
	(&MainView{}).InputChanged(rv, "hello")
	if !rv.lastTypingSent.IsZero() {
		t.Error("typing went out in a chat that didn't opt in")
	}
}

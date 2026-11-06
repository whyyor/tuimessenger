package tui

import (
	"testing"

	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
	"github.com/whyyor/tuimessenger/internal/tui/messages"
)

func TestRetryKind(t *testing.T) {
	const me = id.UserID("@me:x")
	msg := func(sender id.UserID, sendError, txn string) *messages.UIMessage {
		return &messages.UIMessage{Event: &database.Event{Sender: sender, SendError: sendError, TransactionID: txn}}
	}
	cases := []struct {
		name          string
		msg           *messages.UIMessage
		networkFailed bool
		want          retryKind
	}{
		{"nothing selected", nil, false, retryNone},
		{"someone else's message", msg("@them:x", "", ""), true, retryNotOwn},
		{"backend refused it", msg(me, "M_FORBIDDEN: nope", "hicli-1"), false, retryResend},
		{"still waiting to send", msg(me, "not sent", "hicli-1"), false, retryNone},
		{"network refused it", msg(me, "", ""), true, retrySendCopy},
		{"delivered fine", msg(me, "", ""), false, retryNone},
	}
	for _, c := range cases {
		if got := retryKindFor(c.msg, me, c.networkFailed); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

func TestNetworkRefused(t *testing.T) {
	deliveries.lock.Lock()
	deliveries.entries["$failed"] = &deliveryEntry{state: deliveryFailed}
	deliveries.entries["$ok"] = &deliveryEntry{state: deliveryDelivered}
	deliveries.lock.Unlock()
	if !networkRefused("$failed") || networkRefused("$ok") || networkRefused("$unknown") {
		t.Error("only a FAIL_PERMANENT status counts as refused")
	}
}

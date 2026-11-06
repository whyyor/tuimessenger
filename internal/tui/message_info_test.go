package tui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/util/jsontime"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli/cmdspec"
	"github.com/whyyor/tuimessenger/internal/hicli/database"
	"github.com/whyyor/tuimessenger/internal/tui/messages"
	"github.com/whyyor/tuimessenger/internal/tui/messages/tstring"
)

func infoEvt(evtID id.EventID, sender id.UserID, typ string, at time.Time, content any) *database.Event {
	raw, _ := json.Marshal(content)
	return &database.Event{ID: evtID, RoomID: "!room:beeper.com", Sender: sender, Type: typ,
		Content: raw, Timestamp: jsontime.UM(at)}
}

func receipt(user id.UserID, typ event.ReceiptType, at time.Time) *database.Receipt {
	return &database.Receipt{UserID: user, ReceiptType: typ, Timestamp: jsontime.UM(at)}
}

func TestWhoSaw(t *testing.T) {
	base := time.Date(2026, 10, 26, 14, 0, 0, 0, time.Local)
	msg := infoEvt("$msg", me, "m.room.message", base, nil)
	later := []*database.Event{
		infoEvt("$reply", "@roy:x", "m.room.message", base.Add(5*time.Minute), map[string]any{"body": "ok"}),
		infoEvt("$bot", "@whatsappbot:x", "m.room.message", base.Add(6*time.Minute), nil),
	}
	receipts := map[id.EventID][]*database.Receipt{
		"$msg": {
			receipt("@nadia:x", event.ReceiptTypeRead, base.Add(time.Minute)),
			receipt("@priv:x", event.ReceiptTypeReadPrivate, base.Add(time.Minute)),
			receipt(me, event.ReceiptTypeRead, base),
			receipt(cmdspec.FakeSender, event.ReceiptTypeRead, base),
		},
		"$reply": {receipt("@roy:x", event.ReceiptTypeRead, base.Add(7*time.Minute))},
		"$older": {receipt("@old:x", event.ReceiptTypeRead, base.Add(time.Hour))},
	}
	got := whoSaw(msg, me, later, receipts)
	if len(got) != 2 || got[0].user != "@nadia:x" || got[1].user != "@roy:x" {
		t.Fatalf("want nadia then roy, got %+v", got)
	}
	if !got[1].replied || !got[1].at.Equal(base.Add(5*time.Minute)) {
		t.Errorf("roy's reply is earlier than his receipt and should win: %+v", got[1])
	}
}

func TestDeliveryInfo(t *testing.T) {
	base := time.Now()
	at, failed := deliveryInfo([]*database.Event{
		infoEvt("$s2", "@bot:x", messageStatusEvent, base.Add(time.Second), map[string]any{"status": "SUCCESS"}),
		infoEvt("$s1", "@bot:x", messageStatusEvent, base, map[string]any{"status": "SUCCESS"}),
	})
	if at.UnixMilli() != base.UnixMilli() || failed != "" {
		t.Errorf("first success wins: %v %q", at, failed)
	}
	_, failed = deliveryInfo([]*database.Event{infoEvt("$f", "@bot:x", messageStatusEvent, base,
		map[string]any{"status": "FAIL_PERMANENT", "message": "not on WhatsApp"})})
	if failed != "not on WhatsApp" {
		t.Errorf("failure message: %q", failed)
	}
}

func TestReactionLines(t *testing.T) {
	react := func(evtID id.EventID, sender id.UserID, key string) *database.Event {
		return infoEvt(evtID, sender, event.EventReaction.Type, time.Now(), map[string]any{
			"m.relates_to": map[string]any{"rel_type": "m.annotation", "event_id": "$msg", "key": key}})
	}
	enc := react("$r5", "@mariana:x", "👍")
	enc.Type, enc.DecryptedType, enc.Decrypted, enc.Content = "m.room.encrypted", event.EventReaction.Type, enc.Content, []byte(`{}`)
	gone := react("$r4", "@x:x", "😢")
	gone.RedactedBy = "$redaction"
	got := reactionLines([]*database.Event{
		react("$r1", "@peter:x", "👍"), react("$r2", "@mariana:x", "❤️"), react("$r3", me, "👍"), gone, enc,
	}, me, func(u id.UserID) string { return strings.TrimPrefix(strings.Split(string(u), ":")[0], "@") })
	want := []string{"👍 peter, You, mariana", "❤️ mariana"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMessageInfoPopup(t *testing.T) {
	mv, room := bubbleView(t)
	view := mv.parent
	base := time.Now().Add(-10 * time.Minute)
	evt := infoEvt("$mine", me, "m.room.message", base, map[string]any{"body": "hi"})
	msg := messages.NewExpandedTextMessage(evt, room, tstring.NewTString("hi"))

	old := fetchInfo
	t.Cleanup(func() { fetchInfo = old })
	done := make(chan struct{})
	fetchInfo = func(*RoomView, *database.Event, []*database.Event) ([]*database.Event, []*database.Event, map[id.EventID][]*database.Receipt) {
		defer close(done)
		mss := []*database.Event{infoEvt("$s", "@bot:x", messageStatusEvent, base.Add(2*time.Second), map[string]any{"status": "SUCCESS"})}
		return mss, nil, map[id.EventID][]*database.Receipt{"$mine": {receipt("@nadia:x", event.ReceiptTypeRead, base.Add(time.Minute))}}
	}
	view.OpenMessageInfo(msg)
	if view.info == nil {
		t.Fatal("popup should open")
	}
	<-done
	for i := 0; i < 100; i++ {
		view.info.mu.Lock()
		loading := view.info.loading
		view.info.mu.Unlock()
		if !loading {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(70, 14)
	view.info.Draw(sim)
	text := strings.Join(screenText(sim, 70, 14), "\n")
	for _, want := range []string{"Message info", "Sent", "Delivered", "Seen", "nadia", "esc close"} {
		if !strings.Contains(text, want) {
			t.Errorf("popup missing %q:\n%s", want, text)
		}
	}

	view.onInfoKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if view.info != nil {
		t.Error("esc should close the popup")
	}
}

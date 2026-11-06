package tui

import (
	"encoding/json"
	"testing"
	"time"

	"go.mau.fi/util/jsontime"
	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
	"github.com/whyyor/tuimessenger/internal/hicli/jsoncmd"
)

func statusEvent(target, status, message, reason string, at time.Time) *database.Event {
	content, _ := json.Marshal(map[string]any{
		"m.relates_to": map[string]string{"rel_type": "m.reference", "event_id": target},
		"status":       status, "message": message, "reason": reason,
	})
	return &database.Event{Type: messageStatusEvent, Content: content, Timestamp: jsontime.UM(at)}
}

func TestSendFailures(t *testing.T) {
	now := time.Now()
	sync := func(since string, events ...*database.Event) *jsoncmd.SyncComplete {
		evt := &jsoncmd.SyncComplete{Rooms: map[id.RoomID]*jsoncmd.SyncRoom{"!a:x": {Events: events}}}
		if since != "" {
			evt.Since = ptr.Ptr(since)
		}
		return evt
	}
	svg := statusEvent("$svg", "FAIL_PERMANENT", "unsupported media type (image/svg+xml in m.image)", "m.event_not_handled", now)

	if got := sendFailures(sync("", svg), now); len(got) != 0 {
		t.Errorf("startup data shouldn't report old failures: %+v", got)
	}
	got := sendFailures(sync("s1", svg,
		statusEvent("$ok", "SUCCESS", "", "", now),
		statusEvent("$retry", "FAIL_RETRIABLE", "", "m.foreign_network_error", now),
		statusEvent("$old", "FAIL_PERMANENT", "too old", "", now.Add(-time.Hour)),
	), now)
	if len(got) != 2 {
		t.Fatalf("want the svg and the retriable failure, got %+v", got)
	}
	byTarget := map[id.EventID]sendFailure{}
	for _, f := range got {
		byTarget[f.target] = f
	}
	if f := byTarget["$svg"]; !f.permanent || f.reason != "unsupported media type (image/svg+xml in m.image)" || f.roomID != "!a:x" {
		t.Errorf("svg failure: %+v", f)
	}
	if f := byTarget["$retry"]; f.permanent || f.reason != "m.foreign_network_error" {
		t.Errorf("retriable failure should fall back to the reason code: %+v", f)
	}
}

func TestFirstReport(t *testing.T) {
	if !firstReport("$once") || firstReport("$once") {
		t.Error("a failure should be reported exactly once")
	}
}

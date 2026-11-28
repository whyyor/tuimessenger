package tui

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/util/jsontime"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
	"github.com/whyyor/tuimessenger/internal/hicli/jsoncmd"
	"github.com/whyyor/tuimessenger/internal/rpc/store"
	"github.com/whyyor/tuimessenger/internal/tui/config"
	"github.com/whyyor/tuimessenger/internal/tui/messages"
)

const burstSender = id.UserID("@whatsapp_friend:x")

func burstEvt(room *store.RoomStore, row int, sender id.UserID, at time.Time, text string) *database.Event {
	return &database.Event{RowID: database.EventRowID(row), TimelineRowID: database.TimelineRowID(row * 2),
		ID: id.EventID(fmt.Sprintf("$e%d:x", row)), RoomID: room.ID, Sender: sender, Type: "m.room.message",
		Content: []byte(fmt.Sprintf(`{"msgtype":"m.text","body":%q}`, text)), Timestamp: jsontime.UM(at)}
}

func liveSync(room *store.RoomStore, evts ...*database.Event) {
	var tl []database.TimelineRowTuple
	for _, e := range evts {
		tl = append(tl, database.TimelineRowTuple{Timeline: e.TimelineRowID, Event: e.RowID})
	}
	room.ApplySync(&jsoncmd.SyncRoom{Events: evts, Timeline: tl})
}

func timelineMessageRows(room *store.RoomStore) (rows []database.EventRowID) {
	for _, e := range *room.TimelineCache.Current() {
		if e.Type == "m.room.message" {
			rows = append(rows, e.RowID)
		}
	}
	return
}

// The notifier and the open chat read a new message at the same moment. The content used to be shared before it was
// parsed, so now and then the chat got an empty message (drawn as nothing until restart) or a torn one (a crash).
func TestConcurrentParseNeverDropsMessage(t *testing.T) {
	_, room := bubbleView(t)
	cfg := &config.Config{}
	n := 50000
	if testing.Short() {
		n = 5000
	}
	lost := 0
	for i := range n {
		evt := burstEvt(room, i+1, burstSender, time.Now(), "oho")
		var wg sync.WaitGroup
		wg.Add(1)
		go func() { // NotifyMessage
			defer wg.Done()
			_ = evt.GetMautrixContent().AsMessage().Body
		}()
		if messages.ParseEvent(nil, &cfg.Preferences, room, evt) == nil { // MessageView.update
			lost++
		}
		wg.Wait()
	}
	if lost > 0 {
		t.Errorf("%d of %d messages had nothing to draw", lost, n)
	}
}

// A message that once parsed to nothing is parsed again on the next update, not skipped until restart.
func TestTypedNilRenderMetaIsReparsed(t *testing.T) {
	view, room := bubbleView(t)
	base := time.Now().Add(-time.Hour)
	a, b := burstEvt(room, 1, burstSender, base, "a"), burstEvt(room, 2, burstSender, base.Add(time.Second), "oho")
	b.RenderMeta = (*messages.UIMessage)(nil)
	liveSync(room, a, b)
	view.update(80)
	if got := bufferRows(view); !slices.Equal(got, []database.EventRowID{1, 2}) {
		t.Errorf("drawn %v, want both", got)
	}
}

// Today's shape of the bug: my message (pending, then its echo) and a burst of hers, one sync each, chat open.
func TestBurstWhileChatOpenIsAllDrawn(t *testing.T) {
	view, room := bubbleView(t)
	base := time.Now().Add(-time.Hour)
	for i := range 20 {
		liveSync(room, burstEvt(room, 100+i, burstSender, base.Add(time.Duration(i)*time.Second), fmt.Sprintf("old %d", i)))
	}
	view.update(80)
	at := base.Add(time.Minute)
	room.ApplyPending(burstEvt(room, 200, me, at, "zaid ne"))
	view.update(80)
	liveSync(room, burstEvt(room, 200, me, at, "zaid ne"))
	texts := []string{"oho", "sahi h to kafi", "bhyi pesa hi pesa", "yee", "nind ni aundiii", "🤣🤣🤣"}
	for i, txt := range texts {
		liveSync(room, burstEvt(room, 201+i, burstSender, at.Add(time.Duration(i+1)*time.Second), txt))
		view.update(80)
		if got, want := bufferRows(view), timelineMessageRows(room); !slices.Equal(got, want) {
			t.Fatalf("after %q: drawn %v, timeline %v", txt, got, want)
		}
	}
	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(80, 30)
	view.Draw(sim)
	screen := strings.Join(screenText(sim, 80, 30), "\n")
	for _, txt := range append(texts, "zaid ne") {
		if !strings.Contains(screen, txt) {
			t.Errorf("%q not on screen:\n%s", txt, screen)
		}
	}
}

// Random orders of her messages, my sends (pending, echo and send-complete either way round) and send-status events.
func TestRandomLiveOrdersAreAllDrawn(t *testing.T) {
	seeds := 3000
	if testing.Short() {
		seeds = 300
	}
	for seed := range seeds {
		rng := rand.New(rand.NewPCG(uint64(seed), 1))
		view, room := bubbleView(t)
		base := time.Now().Add(-time.Hour)
		row := 1000
		for i := range 20 {
			row++
			liveSync(room, burstEvt(room, row, burstSender, base.Add(time.Duration(i)*time.Second), "old"))
		}
		view.update(80)
		type send struct {
			row   int
			at    time.Time
			stage int
		}
		var sends []*send
		var steps []string
		for step := range 40 {
			at := base.Add(time.Minute + time.Duration(step)*time.Second)
			switch rng.IntN(5) {
			case 0, 1:
				row++
				liveSync(room, burstEvt(room, row, burstSender, at, fmt.Sprintf("her%d", row)))
				steps = append(steps, fmt.Sprintf("her %d", row))
			case 2:
				row++
				pending := burstEvt(room, row, me, at, fmt.Sprintf("me%d", row))
				pending.ID = id.EventID(fmt.Sprintf("~local%d", row))
				room.ApplyPending(pending)
				sends = append(sends, &send{row: row, at: at})
				steps = append(steps, fmt.Sprintf("pending %d", row))
			default:
				if len(sends) == 0 {
					continue
				}
				s := sends[rng.IntN(len(sends))]
				if rng.IntN(2) == 0 && s.stage&1 == 0 {
					room.ApplySendComplete(burstEvt(room, s.row, me, s.at, fmt.Sprintf("me%d", s.row)))
					s.stage |= 1
					steps = append(steps, fmt.Sprintf("sendcomplete %d", s.row))
				} else if s.stage&2 == 0 {
					echo := burstEvt(room, s.row, me, s.at, fmt.Sprintf("me%d", s.row))
					row++
					status := &database.Event{RowID: database.EventRowID(row), TimelineRowID: database.TimelineRowID(row * 2),
						ID: id.EventID(fmt.Sprintf("$s%d:x", row)), RoomID: room.ID, Sender: "@whatsappbot:x",
						Type: "com.beeper.message_send_status", RelationType: "m.reference", RelatesTo: echo.ID,
						Content: []byte(`{"status":"SUCCESS"}`), Timestamp: jsontime.UM(at)}
					liveSync(room, echo)
					liveSync(room, status)
					s.stage |= 2
					steps = append(steps, fmt.Sprintf("echo %d", s.row))
				}
			}
			view.update(80)
			if got, want := bufferRows(view), timelineMessageRows(room); !slices.Equal(got, want) {
				t.Fatalf("seed %d step %d: drawn %v\ntimeline %v\nsteps %v", seed, step, got, want, steps)
			}
		}
	}
}

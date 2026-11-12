package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/mauview"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
	"github.com/whyyor/tuimessenger/internal/rpc/store"
	"github.com/whyyor/tuimessenger/internal/tui/config"
)

// freshDrafts gives the test its own empty draft store saving into dir ("" = memory only).
func freshDrafts(t *testing.T, dir string) {
	t.Helper()
	old := drafts
	drafts = &draftStore{}
	drafts.use(dir)
	t.Cleanup(func() {
		// A pending save would write into the test's folder while it's being removed.
		drafts.mu.Lock()
		if drafts.timer != nil {
			drafts.timer.Stop()
		}
		drafts.mu.Unlock()
		drafts = old
	})
}

func TestDraftStore(t *testing.T) {
	dir := t.TempDir()
	freshDrafts(t, dir)
	drafts.set("!a:x", draft{Text: "hello"})
	drafts.set("!b:x", draft{Text: "tmp"})
	drafts.set("!b:x", draft{})
	if d, _ := drafts.get("!a:x"); d.Text != "hello" || !drafts.has("!a:x") {
		t.Errorf("kept: %+v", d)
	}
	if drafts.has("!b:x") {
		t.Error("an empty draft removes it")
	}
	drafts.save()
	// A restart reads them back.
	drafts = &draftStore{}
	drafts.use(dir)
	if d, _ := drafts.get("!a:x"); d.Text != "hello" {
		t.Errorf("survives a restart: %+v", d)
	}
	if info, err := os.Stat(filepath.Join(dir, "drafts.json")); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("drafts.json is yours only: %v %v", err, info)
	}
}

// Shortly after you stop typing, the draft is on disk, so a crash doesn't lose it.
func TestDraftSavedAfterTyping(t *testing.T) {
	old := draftSaveDelay
	draftSaveDelay = 20 * time.Millisecond
	t.Cleanup(func() { draftSaveDelay = old })
	dir := t.TempDir()
	freshDrafts(t, dir)
	drafts.set("!a:x", draft{Text: "almost done"})
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(filepath.Join(dir, "drafts.json")); err == nil && strings.Contains(string(data), "almost done") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the draft wasn't saved after typing")
}

func draftRoom(t *testing.T) *store.RoomStore {
	t.Helper()
	gs := &store.Store{}
	gs.UserID = me
	return store.NewRoomStore(gs, &database.Room{ID: "!chat:x"})
}

func draftView(room *store.RoomStore) *RoomView {
	return &RoomView{Room: room, config: &config.Config{}, input: mauview.NewInputArea()}
}

// Leave a chat with text, a reply and a file; coming back puts all three back.
func TestDraftRoundTrip(t *testing.T) {
	freshDrafts(t, "")
	room := draftRoom(t)
	quoted := &database.Event{RowID: 1, ID: "$q:x", RoomID: room.ID, Sender: "@peter:x"}
	room.ApplyFetched(quoted)

	view := draftView(room)
	view.input.SetTextAndMoveCursor("see you at")
	view.replying = quoted
	view.attached.items = []pendingFile{{path: "/tmp/photo.jpg"}}
	view.keepDraft()
	if len(view.Attachments()) != 0 {
		t.Error("the files go with the draft")
	}

	back := draftView(room)
	back.restoreDraft()
	if back.input.GetText() != "see you at" || back.input.GetCursorOffset() != len("see you at") {
		t.Errorf("text with the cursor at the end: %q at %d", back.input.GetText(), back.input.GetCursorOffset())
	}
	if back.replying != quoted {
		t.Error("the reply comes back")
	}
	if files := back.Attachments(); len(files) != 1 || files[0].path != "/tmp/photo.jpg" {
		t.Errorf("the files come back: %v", files)
	}
}

// While editing, the draft is the text you parked, not the edit.
func TestDraftWhileEditing(t *testing.T) {
	freshDrafts(t, "")
	room := draftRoom(t)
	view := draftView(room)
	view.editing = &database.Event{ID: "$mine:x"}
	view.editMoveText = "my draft"
	view.input.SetText("fixed typo")
	view.keepDraft()
	if d, _ := drafts.get(room.ID); d.Text != "my draft" {
		t.Errorf("parked draft kept: %+v", d)
	}
}

// Putting a draft back isn't typing: the other side isn't told.
func TestDraftRestoreIsntTyping(t *testing.T) {
	freshDrafts(t, "")
	room := draftRoom(t)
	drafts.set(room.ID, draft{Text: "hi"})
	view := draftView(room)
	restoringSeen := false
	view.input.SetChangedFunc(func(string) { restoringSeen = view.restoring })
	view.restoreDraft()
	if !restoringSeen || view.restoring {
		t.Errorf("the change during restore is marked (seen %v), and only then (now %v)", restoringSeen, view.restoring)
	}
}

// Chats with a draft get ✎ in the sidebar, except the open one.
func TestDraftMarkerInSidebar(t *testing.T) {
	freshDrafts(t, "")
	main := aliasMain(t)
	drafts.set("!mariana:x", draft{Text: "later"})
	drafts.set("!peter:x", draft{Text: "now"})
	main.roomList.selected = "!peter:x"
	sim := tcell.NewSimulationScreen("")
	_ = sim.Init()
	sim.SetSize(30, 4)
	main.roomList.Draw(sim)
	rows := screenText(sim, 30, 4)
	var mariana, peter string
	for _, row := range rows {
		switch {
		case strings.Contains(row, "Mariana"):
			mariana = row
		case strings.Contains(row, "Peter"):
			peter = row
		}
	}
	if !strings.Contains(mariana, draftIcon) {
		t.Errorf("Mariana has a draft: %q", mariana)
	}
	if strings.Contains(peter, draftIcon) {
		t.Errorf("the open chat shows its draft in the box, not a marker: %q", peter)
	}
}

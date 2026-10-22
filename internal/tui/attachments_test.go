package tui

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"go.mau.fi/mauview"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/gomuks/pkg/hicli/database"
	"go.mau.fi/gomuks/tui/config"
)

type sentFile struct {
	path, caption string
	reply         bool
}

func TestAttachCaptionAndSend(t *testing.T) {
	var mu sync.Mutex
	var sent []sentFile
	done := make(chan struct{}, 4)
	orig := sendFile
	sendFile = func(_ *RoomView, path, caption string, rel *event.RelatesTo) {
		mu.Lock()
		sent = append(sent, sentFile{filepath.Base(path), caption, rel != nil})
		mu.Unlock()
		done <- struct{}{}
	}
	defer func() { sendFile = orig }()

	dir := t.TempDir()
	var paths []string
	for _, name := range []string{"a b.pdf", "photo.jpg", "c.zip"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	cfg := &config.Config{}
	cfg.LoadKeybindings()
	view := &RoomView{config: cfg, input: mauview.NewInputArea()}

	// A drop of two escaped paths, then a third: three chips, nothing sent.
	view.OnPasteEvent(fakePaste{text: strings.ReplaceAll(paths[0], " ", `\ `) + " " + paths[1]})
	view.AttachFiles(paths[2], paths[1])
	if got := len(view.Attachments()); got != 3 {
		t.Fatalf("want 3 chips (deduped), got %d", got)
	}
	if view.input.GetText() != "" {
		t.Error("a dropped path must not land in the text box")
	}
	// Backspace on an empty box drops the last chip.
	view.OnKeyEvent(tcell.NewEventKey(tcell.KeyBackspace2, 0, tcell.ModNone))
	if got := len(view.Attachments()); got != 2 {
		t.Fatalf("backspace: %d chips", got)
	}

	view.replying = &database.Event{ID: "$parent"}
	view.InputSubmit("here are the docs")
	for range 2 {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("not sent")
		}
	}
	want := []sentFile{{"a b.pdf", "here are the docs", true}, {"photo.jpg", "", false}}
	mu.Lock()
	defer mu.Unlock()
	if len(sent) != 2 || sent[0] != want[0] || sent[1] != want[1] {
		t.Errorf("sent %+v, want %+v", sent, want)
	}
	if len(view.Attachments()) != 0 || view.replying != nil || view.input.GetText() != "" {
		t.Error("chips, reply and caption should be cleared after sending")
	}
}

func TestEscapeDropsChips(t *testing.T) {
	cfg := &config.Config{}
	cfg.LoadKeybindings()
	view := &RoomView{config: cfg, input: mauview.NewInputArea()}
	view.AttachFiles("/tmp/x.pdf")
	if !view.ClearAttachments() || len(view.Attachments()) != 0 {
		t.Error("Escape should drop the chips")
	}
	if view.ClearAttachments() {
		t.Error("nothing left to drop: Escape falls through to its usual job")
	}
}

func TestAttachmentChips(t *testing.T) {
	items := []pendingFile{{path: "/a/report.pdf"}, {path: "/b/photo.jpg"}, {path: "/c/archive.zip"}}
	chips, more := attachmentChips(items, 100)
	if len(chips) != 3 || more != 0 || chips[0] != "[\uf1c1 report.pdf]" || chips[1] != "[\uf03e photo.jpg]" {
		t.Errorf("chips %q more %d", chips, more)
	}
	chips, more = attachmentChips(items, 31)
	if len(chips) != 2 || more != 1 {
		t.Errorf("narrow: chips %q more %d, want 2 and +1", chips, more)
	}
	long := []pendingFile{{path: "/x/" + strings.Repeat("n", 60) + ".txt"}}
	if chips, _ = attachmentChips(long, 100); !strings.Contains(chips[0], "…") {
		t.Errorf("long names are cut: %q", chips)
	}
}

package tui

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"go.mau.fi/mauview"

	"go.mau.fi/gomuks/pkg/hicli/cmdspec"
)

func TestSplitShellWords(t *testing.T) {
	got := splitShellWords(`/a/My\ File.pdf '/b/c d.png' "/e/f g.zip" /h`)
	want := []string{"/a/My File.pdf", "/b/c d.png", "/e/f g.zip", "/h"}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDroppedFiles(t *testing.T) {
	dir := t.TempDir()
	pdf := filepath.Join(dir, "My Report.pdf")
	zip := filepath.Join(dir, "a.zip")
	for _, p := range []string{pdf, zip} {
		if err := os.WriteFile(p, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	escaped := filepath.Join(dir, `My\ Report.pdf`)
	if got := droppedFiles(escaped + " " + zip); !slices.Equal(got, []string{pdf, zip}) {
		t.Errorf("two dropped files: %q", got)
	}
	if got := droppedFiles(pdf); !slices.Equal(got, []string{pdf}) {
		t.Errorf("unescaped path with a space: %q", got)
	}
	if got := droppedFiles("hello " + zip); got != nil {
		t.Errorf("ordinary text must not count as files: %q", got)
	}
	if got := droppedFiles(dir); got != nil {
		t.Errorf("a folder is not a file: %q", got)
	}
}

type fakePaste struct {
	mauview.PasteEvent
	text string
}

func (f fakePaste) Text() string { return f.text }

func TestPastedPathBecomesChip(t *testing.T) {
	file := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	view := &RoomView{input: mauview.NewInputArea()}
	view.OnPasteEvent(fakePaste{text: file})
	if got := view.Attachments(); len(got) != 1 || got[0].path != file || view.input.GetText() != "" {
		t.Errorf("chips %+v, text %q", got, view.input.GetText())
	}
}

func TestBareUploadParses(t *testing.T) {
	for _, cmd := range LocalCommands {
		if cmd.Command == CmdUpload {
			if parsed, err := cmd.ParseInput(cmdspec.FakeGomuksSender, cmdSigils, "/upload"); err != nil || parsed == nil {
				t.Fatalf("bare /upload: %v", err)
			}
			return
		}
	}
	t.Fatal("upload not registered")
}

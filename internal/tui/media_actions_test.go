package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"maunium.net/go/mautrix/event"
)

// The wedding invite that failed to download: a 426-byte caption in body and
// the real name in filename.
const inviteCaption = "Hey! ❤️\nIt’s finally happening! ✨\nWe’d love to have you with us as we " +
	"celebrate all the special moments leading up to our wedding.\n\nSharing our wedding " +
	"e-invite with all the details for the Sangeet, Haldi, Mehendi, Reception & Wedding. 💍✨\n\n" +
	"Your presence would mean the world to us, and we can’t wait to celebrate, dance, laugh " +
	"and make some beautiful memories together! 🥂🫶🏻\n\nCan’t wait to see you there! ❤️\n— Manasi & Devanshu"

func TestMediaFileName(t *testing.T) {
	cases := []struct {
		name                   string
		fileName, body, fileID string
		mime                   string
		msgType                event.MessageType
		want                   string
	}{
		{"filename wins over caption", "Mishra Wedding E-invite.pdf", inviteCaption, "abc", "application/pdf", event.MsgFile, "Mishra Wedding E-invite.pdf"},
		{"legacy body is the name", "", "IMG-20261005.jpg", "abc", "image/jpeg", event.MsgImage, "IMG-20261005.jpg"},
		{"bare body gets mime extension", "", "voice note", "abc", "audio/ogg", event.MsgAudio, "voice note.ogg"},
		{"nothing but the media id", "", "", "AbCdEf", "video/mp4", event.MsgVideo, "AbCdEf.mp4"},
		{"no path traversal", "../../etc/passwd", "", "abc", "", event.MsgFile, "-..-etc-passwd"},
		{"pdf extension from mime table", "", "invite", "abc", "application/pdf", event.MsgFile, "invite.pdf"},
		{"unknown file type gets no extension", "", "notes", "abc", "", event.MsgFile, "notes"},
		{"voice note mime with codec parameter", "", "voice", "abc", "audio/ogg; codecs=opus", event.MsgAudio, "voice.ogg"},
		{"matroska is mkv", "", "clip", "abc", "video/x-matroska", event.MsgVideo, "clip.mkv"},
		{"plain text is txt", "", "notes", "abc", "text/plain", event.MsgFile, "notes.txt"},
		{"octet-stream means unknown", "", "blob", "abc", "application/octet-stream", event.MsgFile, "blob"},
		{"sentence dot is not an extension", "", "Check this out. Amazing", "abc", "image/jpeg", event.MsgImage, "Check this out. Amazing.jpg"},
		{"newlines collapse", "", "line one\nline two.txt", "abc", "", event.MsgFile, "line one line two.txt"},
	}
	for _, c := range cases {
		if got := mediaFileName(c.fileName, c.body, c.fileID, c.mime, c.msgType); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// A caption used as a name (no filename field) must still fit on disk and keep
// valid UTF-8 after truncation.
func TestMediaFileNameCaptionWithoutFilename(t *testing.T) {
	got := mediaFileName("", inviteCaption, "abc", "application/pdf", event.MsgFile)
	if len(got) > maxFileNameBytes+len(".pdf") {
		t.Fatalf("name too long: %d bytes", len(got))
	}
	if !strings.HasSuffix(got, ".pdf") {
		t.Errorf("expected a .pdf extension, got %q", got)
	}
	if strings.ContainsAny(got, "\n/") {
		t.Errorf("unsafe characters left in %q", got)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, got), []byte("x"), 0o600); err != nil {
		t.Fatalf("name rejected by the filesystem: %v", err)
	}
}

func TestUniquePathNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	first := uniquePath(dir, "invite.pdf")
	if filepath.Base(first) != "invite.pdf" {
		t.Fatalf("free name changed: %q", first)
	}
	if err := os.WriteFile(first, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	second := uniquePath(dir, "invite.pdf")
	if filepath.Base(second) != "invite (1).pdf" {
		t.Fatalf("taken name: got %q, want invite (1).pdf", filepath.Base(second))
	}
}

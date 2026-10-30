package media

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndOpen(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id1, err := s.Save([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	id2, err := s.Save([]byte("hello"))
	if err != nil || id1 != id2 {
		t.Fatalf("same content gave IDs %q and %q (err %v)", id1, id2, err)
	}

	file, err := s.Open(id1)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if data, _ := io.ReadAll(file); string(data) != "hello" {
		t.Errorf("read back %q", data)
	}

	entries, _ := os.ReadDir(s.dir)
	if len(entries) != 1 {
		t.Errorf("folder has %d files, want 1 (no leftovers, no duplicates)", len(entries))
	}
}

func TestSaveFileFromOtherFolder(t *testing.T) {
	s, _ := New(t.TempDir())
	outside := filepath.Join(t.TempDir(), "voice.ogg")
	if err := os.WriteFile(outside, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveFile(outside); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Error("original file should be moved away")
	}
}

func TestOpenRejectsBadIDs(t *testing.T) {
	s, _ := New(t.TempDir())
	for _, id := range []string{"", "../secret", "zz", "../../etc/passwd"} {
		if _, err := s.Open(id); err != ErrInvalidID {
			t.Errorf("Open(%q) error = %v, want ErrInvalidID", id, err)
		}
	}
}

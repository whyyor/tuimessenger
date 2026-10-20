package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func touch(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.WriteFile(path, make([]byte, size), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestFreshTempMediaDirKeepsOnlyTheNextFile(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	dir, err := freshTempMediaDir()
	if err != nil {
		t.Fatal(err)
	}
	touch(t, filepath.Join(dir, "old-reel.mp4"), 10)
	touch(t, filepath.Join(dir, "old-photo.jpg"), 10)

	if dir, err = freshTempMediaDir(); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("previous media left behind: %d files", len(entries))
	}
}

func TestClearMediaCache(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	dir, _ := freshTempMediaDir()
	touch(t, filepath.Join(dir, "video.mp4"), 1000)
	touch(t, filepath.Join(tmp, "gomuks-DeeCOejClvX.mp4"), 500) // legacy yt-dlp reel
	touch(t, filepath.Join(tmp, "other-app.log"), 7)            // not ours
	if err := os.Mkdir(filepath.Join(tmp, "gomuks-somedir"), 0700); err != nil {
		t.Fatal(err)
	}

	count, size := clearMediaCache()
	if count != 2 || size != 1500 {
		t.Errorf("got %d files / %d bytes, want 2 / 1500", count, size)
	}
	for _, keep := range []string{"other-app.log", "gomuks-somedir"} {
		if _, err := os.Stat(filepath.Join(tmp, keep)); err != nil {
			t.Errorf("%s should be untouched", keep)
		}
	}
}

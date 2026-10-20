package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.mau.fi/gomuks/tui/debug"
)

// tempMediaDir holds media fetched to play or open. Players such as IINA
// queue every video in the file's folder, so it only ever holds the latest.
func tempMediaDir() string {
	return filepath.Join(os.TempDir(), "gomuks-media")
}

// freshTempMediaDir empties tempMediaDir and returns it, ready for one new
// file. Something still playing keeps its file: an unlinked open file stays
// readable on macOS.
func freshTempMediaDir() (string, error) {
	dir := tempMediaDir()
	clearDir(dir, nil)
	return dir, os.MkdirAll(dir, 0700)
}

// clearDir removes the entries of dir that keep allows (nil keeps all),
// returning how many and their total size. dir itself stays.
func clearDir(dir string, keep func(os.DirEntry) bool) (count int, size int64) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0
	}
	for _, entry := range entries {
		if keep != nil && !keep(entry) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if info, err := entry.Info(); err == nil {
			size += info.Size()
		}
		if err = os.RemoveAll(path); err == nil {
			count++
		}
	}
	return count, size
}

// legacyTempFile matches files older builds left directly in the temp
// folder: yt-dlp reels and helper files, all named gomuks-….
func legacyTempFile(entry os.DirEntry) bool {
	return entry.Type().IsRegular() && strings.HasPrefix(entry.Name(), "gomuks-")
}

// clearMediaCache empties tempMediaDir plus the legacy gomuks-* temp files.
func clearMediaCache() (int, int64) {
	n1, s1 := clearDir(tempMediaDir(), nil)
	n2, s2 := clearDir(os.TempDir(), legacyTempFile)
	return n1 + n2, s1 + s2
}

func (view *RoomView) ClearMediaCache() {
	defer debug.Recover()
	count, size := clearMediaCache()
	view.AddServiceMessage("Cleared %d files (%s)", count, humanSize(size))
	view.parent.parent.Render()
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

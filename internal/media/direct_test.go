package media

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDirectID(t *testing.T) {
	id := DirectID("whatsapp", []byte{0x00, 0xff, 'a', '-'})
	if !IsDirect(id) || IsDirect("abc123") {
		t.Errorf("IsDirect is wrong for %q", id)
	}
	network, mediaID, ok := ParseDirectID(id)
	if !ok || network != "whatsapp" || !bytes.Equal(mediaID, []byte{0x00, 0xff, 'a', '-'}) {
		t.Errorf("ParseDirectID(%q) = %q %v %v", id, network, mediaID, ok)
	}
	for _, bad := range []string{"", "dm-", "dm-whatsapp", "dm--abc", "dm-whatsapp-!!!", "f00d"} {
		if _, _, ok := ParseDirectID(bad); ok {
			t.Errorf("%q should not parse", bad)
		}
	}
}

func put(t *testing.T, c *Cache, key, data string) {
	t.Helper()
	tmp, err := c.TempFile()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = tmp.WriteString(data)
	_ = tmp.Close()
	if err = c.Put(key, tmp.Name(), "image/png"); err != nil {
		t.Fatal(err)
	}
}

func TestCache(t *testing.T) {
	c, err := NewCache(filepath.Join(t.TempDir(), "cache"), 10)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok := c.Get("missing"); ok {
		t.Error("empty cache should miss")
	}
	put(t, c, "a", "aaaa")
	file, typ, ok := c.Get("a")
	if !ok || typ != "image/png" {
		t.Fatalf("Get a: %v %q", ok, typ)
	}
	data, _ := io.ReadAll(file)
	_ = file.Close()
	if string(data) != "aaaa" {
		t.Errorf("read %q", data)
	}

	// Make "a" the oldest, then go over the 10-byte limit: "a" is evicted, newer ones stay.
	old := time.Now().Add(-time.Hour)
	path, _ := c.paths("a")
	_ = os.Chtimes(path, old, old)
	put(t, c, "b", "bbbb")
	put(t, c, "c", "cccc")
	if _, _, ok = c.Get("a"); ok {
		t.Error("the least recently used file should have been evicted")
	}
	for _, key := range []string{"b", "c"} {
		if f, _, ok := c.Get(key); !ok {
			t.Errorf("%s should still be cached", key)
		} else {
			_ = f.Close()
		}
	}
}

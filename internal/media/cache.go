package media

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Cache keeps recently opened direct media on disk, up to a size limit, so opening the same photo twice only
// downloads it once. When it's full, the files opened longest ago are removed.
type Cache struct {
	dir      string
	maxBytes int64
	lock     sync.Mutex
}

func NewCache(dir string, maxBytes int64) (*Cache, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Cache{dir: dir, maxBytes: maxBytes}, nil
}

// paths returns where a key's file and its content type are kept; keys are hashed so any string is safe.
func (c *Cache) paths(key string) (file, contentType string) {
	sum := sha256.Sum256([]byte(key))
	name := filepath.Join(c.dir, hex.EncodeToString(sum[:]))
	return name, name + ".type"
}

// Get opens a cached file and returns its content type; ok is false if it isn't cached.
func (c *Cache) Get(key string) (file *os.File, contentType string, ok bool) {
	path, typePath := c.paths(key)
	file, err := os.Open(path)
	if err != nil {
		return nil, "", false
	}
	typ, _ := os.ReadFile(typePath)
	now := time.Now()
	_ = os.Chtimes(path, now, now) // marks it recently used, so it's evicted last
	return file, string(typ), true
}

// TempFile creates a file inside the cache folder to download into, so moving it into place is a cheap rename.
func (c *Cache) TempFile() (*os.File, error) {
	return os.CreateTemp(c.dir, ".download-*")
}

// Put moves a finished download into the cache, then trims the cache to its size limit.
func (c *Cache) Put(key, tempPath, contentType string) error {
	path, typePath := c.paths(key)
	if err := os.WriteFile(typePath, []byte(contentType), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	c.trim()
	return nil
}

func (c *Cache) trim() {
	c.lock.Lock()
	defer c.lock.Unlock()
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	type cached struct {
		path   string
		size   int64
		usedAt time.Time
	}
	var files []cached
	var total int64
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) == ".type" || name[0] == '.' {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, cached{path: filepath.Join(c.dir, name), size: info.Size(), usedAt: info.ModTime()})
		total += info.Size()
	}
	sort.Slice(files, func(i, j int) bool { return files[i].usedAt.Before(files[j].usedAt) })
	for _, f := range files {
		if total <= c.maxBytes {
			return
		}
		if err := os.Remove(f.path); err == nil || errors.Is(err, os.ErrNotExist) {
			_ = os.Remove(f.path + ".type")
			total -= f.size
		}
	}
}

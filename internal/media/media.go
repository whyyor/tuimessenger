// Package media keeps files that bridges upload, named by the SHA-256 hash of their content.
package media

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Store is a folder of files; the same file is only ever stored once.
type Store struct {
	dir string
}

var ErrInvalidID = errors.New("invalid media ID")

func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create media folder: %w", err)
	}
	return &Store{dir: dir}, nil
}

// Save stores data and returns its ID.
func (s *Store) Save(data []byte) (string, error) {
	tmp, err := s.TempFile()
	if err != nil {
		return "", err
	}
	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("failed to write media: %w", err)
	}
	if err = tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	return s.SaveFile(tmp.Name())
}

// TempFile creates an empty file inside the store, for callers that stream data into it before SaveFile.
func (s *Store) TempFile() (*os.File, error) {
	return os.CreateTemp(s.dir, ".incoming-*")
}

// SaveFile moves a finished file into the store and returns its ID. The original path is gone afterwards.
func (s *Store) SaveFile(path string) (string, error) {
	id, err := hashFile(path)
	if err != nil {
		return "", err
	}
	final := filepath.Join(s.dir, id)
	if _, err = os.Stat(final); err == nil {
		// Already stored (same content), so skip the write; nice for the Pi's flash.
		return id, os.Remove(path)
	}
	if err = os.Rename(path, final); err != nil {
		// Rename fails across disks (e.g. from /tmp), so copy instead.
		if err = copyFile(path, final); err != nil {
			return "", err
		}
		_ = os.Remove(path)
	}
	return id, nil
}

// Open opens a stored file for reading.
func (s *Store) Open(id string) (*os.File, error) {
	if !validID(id) {
		return nil, ErrInvalidID
	}
	return os.Open(filepath.Join(s.dir, id))
}

// validID only accepts 64 hex characters, so an ID like "../../etc/passwd" can never escape the folder.
func validID(id string) bool {
	if len(id) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to hash media: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func copyFile(from, to string) error {
	src, err := os.Open(from)
	if err != nil {
		return err
	}
	defer src.Close()
	tmp, err := os.CreateTemp(filepath.Dir(to), ".copy-*")
	if err != nil {
		return err
	}
	if _, err = io.Copy(tmp, src); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("failed to copy media: %w", err)
	}
	if err = tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), to)
}

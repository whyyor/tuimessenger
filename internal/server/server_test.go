package server

import (
	"errors"
	"testing"
)

func TestLockDir(t *testing.T) {
	dir := t.TempDir()
	first, err := lockDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = lockDir(dir); !errors.Is(err, ErrLocked) {
		t.Errorf("second backend on the same folder: %v", err)
	}
	_ = first.Close()
	again, err := lockDir(dir)
	if err != nil {
		t.Errorf("after the first one stopped: %v", err)
	} else {
		_ = again.Close()
	}
}

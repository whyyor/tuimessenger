package main

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The client was imported from another project; only its license headers (which credit the authors) may name it.
func TestNoOldName(t *testing.T) {
	const old = "go" + "muks" // split so this file doesn't match itself
	const header = "// " + old + " - A terminal Matrix client"
	for _, root := range []string{"../../internal", "../../cmd"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			scanner := bufio.NewScanner(f)
			scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
			for line := 1; scanner.Scan(); line++ {
				text := scanner.Text()
				if strings.Contains(strings.ToLower(text), old) && !strings.HasPrefix(text, header) {
					t.Errorf("%s:%d mentions %s: %s", path, line, old, strings.TrimSpace(text))
				}
			}
			return scanner.Err()
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

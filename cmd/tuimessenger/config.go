package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/whyyor/tuimessenger/internal/tui/config"
)

const configUsage = `Usage: tuimessenger config [init]

  config       where tuimessenger.yaml is, and the mistakes in it (exit code 1 if there are some)
  config init  write the commented template if there's no tuimessenger.yaml yet

Every setting: ` + config.DocsURL

// runConfig is `tuimessenger config`.
func runConfig(args []string) int {
	path := config.UserFilePath()
	legacy := filepath.Join(defaultDataDir(), "tui", "config", config.UserFileName)
	switch {
	case len(args) == 1 && args[0] == "init":
		if _, err := os.Stat(path); err == nil {
			fmt.Println(path, "already exists; it isn't changed")
			return 0
		}
		if err := config.WriteTemplate(path); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return 1
		}
		fmt.Println("Wrote", path)
		return 0
	case len(args) > 0:
		fmt.Fprintln(os.Stderr, configUsage)
		return 2
	}
	problems := 0
	found := false
	for _, p := range []string{legacy, path} {
		data, err := os.ReadFile(p)
		if os.IsNotExist(err) {
			continue
		} else if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", p, err)
			problems++
			continue
		}
		found = true
		if p == legacy {
			fmt.Printf("%s (older location, still read; %s wins over it)\n", p, path)
		} else {
			fmt.Println(p)
		}
		_, issues := config.ParseUserFile(data, config.ThemesDir())
		for _, issue := range issues {
			fmt.Println("  " + issue)
		}
		if len(issues) == 0 {
			fmt.Println("  no mistakes")
		}
		problems += len(issues)
	}
	if !found {
		fmt.Printf("No tuimessenger.yaml yet. tuimessenger writes a commented one at %s when it starts, or run: tuimessenger config init\n", path)
	}
	if problems > 0 {
		return 1
	}
	return 0
}

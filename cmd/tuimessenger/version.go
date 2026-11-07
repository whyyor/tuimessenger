package main

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// version is set by release builds (-ldflags "-X main.version=v0.1.0"); other builds describe their commit.
var version = ""

// versionString is e.g. "v0.1.0 (76d9a89, 2026-11-08)" or "dev (76d9a89-modified, 2026-11-08)".
func versionString() string {
	v := version
	if v == "" {
		v = "dev"
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return v
	}
	var rev, date string
	modified := false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.time":
			date = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if rev == "" {
		return v
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	if modified {
		rev += "-modified"
	}
	if len(date) >= 10 {
		rev += ", " + date[:10]
	}
	return fmt.Sprintf("%s (%s)", v, rev)
}

func runVersion() int {
	fmt.Printf("tuimessenger %s, %s, %s/%s\n", versionString(), runtime.Version(), runtime.GOOS, runtime.GOARCH)
	return 0
}

package tui

import (
	"os/exec"
	"regexp"
	"strconv"
	"sync"
)

// lessWrapKeys makes / and ? wrap around the file: the pager opens at the
// newest message, where a plain forward search finds nothing.
const lessWrapKeys = "#command\n/ forw-search ^W\n? back-search ^W\n"

// lessKeyContentVersion is the first less with --lesskey-content.
const lessKeyContentVersion = 643

var lessCandidates = []string{"/run/current-system/sw/bin/less", "/opt/homebrew/bin/less"}

var lessVersionPattern = regexp.MustCompile(`^less (\d+)`)

func lessVersion(path string) int {
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return 0
	}
	m := lessVersionPattern.FindSubmatch(out)
	if m == nil {
		return 0
	}
	v, _ := strconv.Atoi(string(m[1]))
	return v
}

// pickLess returns the newest of the paths; macOS ships an old 581.
func pickLess(paths []string, version func(string) int) (best string, bestVersion int) {
	for _, path := range paths {
		if path == "" {
			continue
		}
		if v := version(path); best == "" || v > bestVersion {
			best, bestVersion = path, v
		}
	}
	return
}

var findLess = sync.OnceValues(func() (string, int) {
	paths := append([]string{}, lessCandidates...)
	if p, err := exec.LookPath("less"); err == nil {
		paths = append([]string{p}, paths...)
	}
	var existing []string
	for _, p := range paths {
		if _, err := exec.LookPath(p); err == nil {
			existing = append(existing, p)
		}
	}
	return pickLess(existing, lessVersion)
})

// lessArgs opens text at the newest line with wrap-around search when the
// less supports it; older ones get a hint to search upward with ?.
func lessArgs(version int) []string {
	args := []string{"+G", "--mouse", "--wheel-lines=3", "-~"}
	if version >= lessKeyContentVersion {
		return append(args, "--lesskey-content="+lessWrapKeys, "-Ps j/k scroll · / search · n next · q back ")
	}
	return append(args, "-Ps j/k scroll · ? search up · n next · q back ")
}

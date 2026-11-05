package tui

import (
	"slices"
	"strings"
	"testing"
)

func TestPickLessPrefersNewest(t *testing.T) {
	versions := map[string]int{"/usr/bin/less": 581, "/run/current-system/sw/bin/less": 710}
	got, v := pickLess([]string{"/usr/bin/less", "/run/current-system/sw/bin/less"}, func(p string) int { return versions[p] })
	if got != "/run/current-system/sw/bin/less" || v != 710 {
		t.Errorf("got %s %d", got, v)
	}
	if got, _ := pickLess(nil, func(string) int { return 0 }); got != "" {
		t.Errorf("no less: %q", got)
	}
}

func TestLessArgs(t *testing.T) {
	newArgs := lessArgs(710)
	if !slices.ContainsFunc(newArgs, func(a string) bool { return strings.HasPrefix(a, "--lesskey-content=") }) {
		t.Errorf("710 should get wrap-around keys: %q", newArgs)
	}
	old := strings.Join(lessArgs(581), " ")
	if strings.Contains(old, "lesskey") || !strings.Contains(old, "? search up") {
		t.Errorf("581 can't take --lesskey-content and should hint ?: %q", old)
	}
	if newArgs[0] != "+G" {
		t.Error("still opens at the newest message")
	}
}

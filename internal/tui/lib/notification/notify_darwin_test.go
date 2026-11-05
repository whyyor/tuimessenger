package notification

import (
	"os"
	"path/filepath"
	"testing"
)

func fakeBundle(t *testing.T, dir, name string) string {
	t.Helper()
	bin := filepath.Join(dir, name+".app", "Contents", "MacOS", "terminal-notifier")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestNotifierForPicksNetworkBundleThenFallback(t *testing.T) {
	orig := notifierDir
	defer func() { notifierDir = orig }()
	notifierDir = t.TempDir()

	whatsapp := fakeBundle(t, notifierDir, "WhatsApp")
	slack := fakeBundle(t, notifierDir, "Slack")

	if got, custom := notifierFor("whatsapp"); got != whatsapp || !custom {
		t.Errorf("whatsapp: got %q custom=%v", got, custom)
	}
	// Beeper's Go bridges report as slackgo; it shares the Slack bundle.
	if got, _ := notifierFor("slackgo"); got != slack {
		t.Errorf("slackgo: got %q, want the Slack bundle", got)
	}

	// No network bundle and no fallback yet: brew's terminal-notifier, if any.
	if got, custom := notifierFor("telegram"); custom {
		t.Errorf("telegram without bundles: unexpected custom notifier %q", got)
	}

	fallback := fakeBundle(t, notifierDir, "gomuks")
	for _, source := range []string{"telegram", ""} {
		if got, custom := notifierFor(source); got != fallback || !custom {
			t.Errorf("%q: got %q custom=%v, want the gomuks fallback", source, got, custom)
		}
	}
}

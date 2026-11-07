package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlackChannelsDefault(t *testing.T) {
	cases := []struct {
		name, file string
		want       bool
	}{
		{"no file", "", true},
		{"not mentioned", "telegram:\n  api_id: 1\n", true},
		{"turned off", "slack:\n  include_channels: false\n", false},
		{"turned on", "slack:\n  include_channels: true\n", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if c.file != "" {
				if err := os.WriteFile(path, []byte(c.file), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatal(err)
			} else if cfg.Slack.IncludeChannels != c.want {
				t.Errorf("include_channels = %v, want %v", cfg.Slack.IncludeChannels, c.want)
			}
		})
	}
}

func TestSetBool(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	orig := "# tuimessenger settings.\ntelegram:\n  # Your own app keys.\n  api_id: 123\n  api_hash: abc\n\nslack:\n  # Bridge channels too.\n  include_channels: true\n"
	if err := os.WriteFile(path, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SetBool(path, "slack", "include_channels", false); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	for _, want := range []string{"# tuimessenger settings.", "# Your own app keys.", "api_hash: abc", "# Bridge channels too.", "include_channels: false"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("missing %q in:\n%s", want, data)
		}
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("file mode %v, want 0600", info.Mode().Perm())
	}
	cfg, err := Load(path)
	if err != nil || cfg.Slack.IncludeChannels || cfg.Telegram.APIID != 123 {
		t.Errorf("read back: %+v %v", cfg, err)
	}

	// No file and no section yet.
	path = filepath.Join(t.TempDir(), "config.yaml")
	if err = SetBool(path, "slack", "include_channels", false); err != nil {
		t.Fatal(err)
	}
	if cfg, err = Load(path); err != nil || cfg.Slack.IncludeChannels {
		t.Errorf("new file: %+v %v", cfg, err)
	}
	// An empty "slack:" line.
	_ = os.WriteFile(path, []byte("slack:\n"), 0o600)
	if err = SetBool(path, "slack", "include_channels", true); err != nil {
		t.Fatal(err)
	}
	if cfg, err = Load(path); err != nil || !cfg.Slack.IncludeChannels {
		t.Errorf("empty section: %+v %v", cfg, err)
	}
}

func TestEnabledNetworks(t *testing.T) {
	cfg := defaults()
	if got := strings.Join(cfg.EnabledNetworks(), ","); got != "whatsapp,slack,instagram" {
		t.Errorf("no Telegram keys: %s", got)
	}
	cfg.Telegram = Telegram{APIID: 1, APIHash: "x"}
	if got := strings.Join(cfg.EnabledNetworks(), ","); got != "whatsapp,telegram,slack,instagram" {
		t.Errorf("with Telegram keys: %s", got)
	}
	cfg.Networks = []string{"slack"}
	if got := strings.Join(cfg.EnabledNetworks(), ","); got != "slack" {
		t.Errorf("listed: %s", got)
	}
}

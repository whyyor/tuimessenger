package config

import (
	"os"
	"path/filepath"
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

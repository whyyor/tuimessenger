// Package config reads tuimessenger's own settings file, config.yaml in the data folder.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	// Networks to run, e.g. [whatsapp, slack]; empty means all of DefaultNetworks that can run.
	Networks []string `yaml:"networks"`
	Telegram Telegram `yaml:"telegram"`
	Slack    Slack    `yaml:"slack"`
}

type Telegram struct {
	// Your own app keys from https://my.telegram.org (API development tools).
	APIID   int    `yaml:"api_id"`
	APIHash string `yaml:"api_hash"`
}

type Slack struct {
	// Channels are bridged by default; false keeps Slack to DMs and group DMs.
	IncludeChannels bool `yaml:"include_channels"`
}

// DefaultNetworks are the networks run when config.yaml doesn't list any.
var DefaultNetworks = []string{"whatsapp", "telegram", "slack", "instagram"}

// EnabledNetworks is the networks to run: the ones listed, or the default ones that can run (Telegram needs its app keys).
func (c *Config) EnabledNetworks() []string {
	if len(c.Networks) > 0 {
		return c.Networks
	}
	var networks []string
	for _, name := range DefaultNetworks {
		if name == "telegram" && (c.Telegram.APIID == 0 || c.Telegram.APIHash == "") {
			continue
		}
		networks = append(networks, name)
	}
	return networks
}

// defaults is what a setting is when config.yaml doesn't mention it.
func defaults() *Config {
	return &Config{Slack: Slack{IncludeChannels: true}}
}

// Load reads the settings file; a missing file just means "all defaults".
func Load(path string) (*Config, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return defaults(), nil
	} else if err != nil {
		return nil, err
	}
	// It holds secrets like the Telegram keys, so refuse to use it if other users on this machine can read it.
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%s can be read by other users; run: chmod 600 %q", path, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Decoding into the defaults only overwrites the settings the file mentions.
	cfg := defaults()
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true) // a typo like "api-id" is an error instead of being silently ignored
	if err = decoder.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}
	return cfg, nil
}

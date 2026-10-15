// Package bridges knows which chat networks exist and how to create each one.
package bridges

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/bridgeconfig"
)

// Network is everything needed to start one chat network.
type Network struct {
	Connector bridgev2.NetworkConnector
	Config    bridgeconfig.BridgeConfig
}

// Factory builds a fresh Network.
type Factory func() (*Network, error)

var registry = map[string]Factory{}

// register is called from each network file's init() to add itself to the list.
func register(name string, factory Factory) {
	if _, exists := registry[name]; exists {
		panic("bridges: network registered twice: " + name)
	}
	registry[name] = factory
}

// New creates the network with the given name, e.g. "whatsapp".
func New(name string) (*Network, error) {
	factory, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown network %q (available: %v)", name, Available())
	}
	return factory()
}

// Available lists all compiled-in network names in alphabetical order.
func Available() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// loadDefaults fills a connector's config struct from its built-in example-config.yaml.
func loadDefaults[T any](connector bridgev2.NetworkConnector) (*T, error) {
	example, data, _ := connector.GetConfig()
	cfg, ok := data.(*T)
	if !ok {
		return nil, fmt.Errorf("unexpected config type %T", data)
	}
	if err := yaml.Unmarshal([]byte(example), cfg); err != nil {
		return nil, fmt.Errorf("failed to parse default config: %w", err)
	}
	return cfg, nil
}

// baseConfig holds the bridgev2 settings shared by every network.
func baseConfig(commandPrefix string) bridgeconfig.BridgeConfig {
	return bridgeconfig.BridgeConfig{
		CommandPrefix:             commandPrefix,
		PersonalFilteringSpaces:   true,
		SplitPortals:              true,
		NoBridgeInfoStateKey:      true,
		BridgeStatusNotices:       "none",
		BridgeMatrixLeave:         true,
		DeduplicateMatrixMessages: true,
		CrossRoomReplies:          true,
		OutgoingMessageReID:       false, // we assign event IDs ourselves when the client sends
		KickMatrixUsers:           true,
		EnableSendStateRequests:   true,
		PhoneNumbersInProfile:     true,
		Permissions: bridgeconfig.PermissionConfig{
			"*": {SendEvents: true, Commands: true, Login: true, DoublePuppet: true, Admin: true},
		},
		Backfill: bridgeconfig.BackfillConfig{
			Enabled:              true,
			MaxInitialMessages:   50,
			MaxCatchupMessages:   500,
			UnreadHoursThreshold: 720,
			Threads:              bridgeconfig.BackfillThreadsConfig{MaxInitialMessages: 50},
			Queue:                bridgeconfig.BackfillQueueConfig{Manual: true, BatchSize: 50, MaxBatches: -1},
		},
	}
}

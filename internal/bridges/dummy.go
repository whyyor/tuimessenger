//go:build devdummy

package bridges

import (
	dummy "github.com/beeper/dummybridge/pkg/connector"
)

// Fake test network; only compiled with `-tags devdummy`, never in normal builds.
func init() {
	register("dummy", newDummy)
}

func newDummy() (*Network, error) {
	connector := &dummy.DummyConnector{}
	cfg, err := loadDefaults[dummy.Config](connector)
	if err != nil {
		return nil, err
	}
	cfg.Automation.Portals.Count = 3   // create 3 fake chats when the login connects
	cfg.Automation.Portals.Members = 2 // with 2 fake contacts each
	return &Network{Connector: connector, Config: baseConfig("!dummy")}, nil
}

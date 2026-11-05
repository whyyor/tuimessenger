package app

import (
	"context"
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/status"
)

// reconnectInterval is the least time between two reconnects asked for by clients: a held-down resync key sends many.
const reconnectInterval = 30 * time.Second

// unhealthy are the connection states a reconnect can fix. Bad credentials or a logout need a new login instead.
var unhealthy = map[status.BridgeStateEvent]bool{
	status.StateTransientDisconnect: true,
	status.StateUnknownError:        true,
}

// ReconnectUnhealthy reconnects the networks whose connection looks broken, and returns their names.
func (a *App) ReconnectUnhealthy(ctx context.Context) []string {
	a.reconnectLock.Lock()
	defer a.reconnectLock.Unlock()
	if time.Since(a.lastReconnect) < reconnectInterval {
		return nil
	}
	var names []string
	for _, br := range a.bridges {
		if !needsReconnect(br) {
			continue
		}
		names = append(names, string(br.ID))
		// The same reset the networks get when the computer changes networks; it can take seconds, so don't wait.
		go br.ResetNetworkConnections()
	}
	if len(names) > 0 {
		a.lastReconnect = time.Now()
	}
	return names
}

func needsReconnect(br *bridgev2.Bridge) bool {
	for _, login := range br.GetAllCachedUserLogins() {
		if unhealthy[login.BridgeState.GetPrev().StateEvent] {
			return true
		}
	}
	return false
}

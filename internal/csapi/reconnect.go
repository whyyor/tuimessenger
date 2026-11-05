package csapi

import "net/http"

// A client's resync can only fix what's between it and us. When a network's own connection is broken (a message
// stuck on WhatsApp's side), resync calls this to reconnect the networks that look unhealthy.
const (
	reconnectFeature = "com.tuimessenger.reconnect"
	reconnectPath    = "/_matrix/client/unstable/com.tuimessenger/reconnect"
)

type respReconnect struct {
	// Reconnecting lists the networks being reconnected; empty if all were healthy or it happened moments ago.
	Reconnecting []string `json:"reconnecting"`
}

func (s *Server) reconnect(r *http.Request) (any, error) {
	names := s.Bridges.ReconnectUnhealthy(r.Context())
	if names == nil {
		names = []string{}
	}
	if len(names) > 0 {
		s.Log.Info().Strs("networks", names).Msg("Client asked to reconnect, reconnecting unhealthy networks")
	}
	return &respReconnect{Reconnecting: names}, nil
}

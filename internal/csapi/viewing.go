package csapi

import (
	"context"
	"net/http"
	"time"

	"maunium.net/go/mautrix/id"
)

// The client says which chat is open on screen (or none). Networks use it like their own apps do:
// WhatsApp goes "online" only while a chat is open, which is when WhatsApp sends you typing notices.
const (
	viewingPath = "/_matrix/client/unstable/com.tuimessenger/viewing"
	// If the client stops refreshing (closed, crashed, laptop asleep), treat it as viewing nothing.
	viewingTimeout = 2 * time.Minute
)

type reqViewing struct {
	RoomID id.RoomID `json:"room_id"`
}

func (s *Server) viewing(r *http.Request) (any, error) {
	var req reqViewing
	if err := readJSON(r, &req); err != nil {
		return nil, err
	}
	s.setViewing(req.RoomID)
	return nil, nil
}

// setViewing passes a change of the open chat to the bridges, and arms the timeout that clears it.
func (s *Server) setViewing(roomID id.RoomID) {
	s.viewingLock.Lock()
	defer s.viewingLock.Unlock()
	if s.viewingTimer != nil {
		s.viewingTimer.Stop()
	}
	if roomID != "" {
		s.viewingTimer = time.AfterFunc(viewingTimeout, func() { s.setViewing("") })
	} else if s.viewingRoom == "" {
		return // nothing was open, so there's nothing to clear
	}
	s.viewingRoom = roomID
	// Refreshes are passed on too: bridgev2 expects them, and networks only act when something changed.
	go func() {
		ctx, cancel := context.WithTimeout(s.Log.WithContext(context.Background()), forwardTimeout)
		defer cancel()
		s.Bridges.SetViewing(ctx, roomID)
	}()
}

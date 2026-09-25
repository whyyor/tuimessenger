// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/rs/zerolog"
	"go.mau.fi/util/ptr"

	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
)

var (
	ErrNotConnectedToWebsocket               = errors.New("not connected to websocket")
	ErrWebsocketClosedBeforeResponseReceived = errors.New("websocket closed before response received")
)

type wrappedEvent struct {
	Data  any
	ReqID int64
}

// connectOnce opens a single websocket connection and returns the context
// belonging to it. The context is cancelled when the connection dies, so
// callers can use it to detect disconnection.
func (gr *GomuksRPC) connectOnce(ctx context.Context) (context.Context, error) {
	connCtx, cancel := context.WithCancel(ctx)
	if stopFn := gr.stop.Swap(&cancel); stopFn != nil {
		(*stopFn)()
	}
	wsURL := gr.BuildRawURL(GomuksURLPath{"websocket"})
	wsURL.Scheme = strings.Replace(wsURL.Scheme, "http", "ws", 1)
	query := url.Values{}
	lastReqID := gr.lastReqID.Load()
	if runID := gr.getRunID(); runID != "" && lastReqID != 0 {
		query.Set("run_id", runID)
		query.Set("last_received_event", strconv.FormatInt(lastReqID, 10))
	}
	wsURL.RawQuery = query.Encode()
	zerolog.Ctx(connCtx).Info().Stringer("url", wsURL).Msg("Connecting to websocket")
	ws, resp, err := websocket.Dial(connCtx, wsURL.String(), &websocket.DialOptions{
		HTTPClient: gr.http,
		HTTPHeader: http.Header{"User-Agent": {gr.UserAgent}},
	})
	if err != nil {
		cancel()
		// Don't leave the previous, dead connection in place: rawRequest would
		// write into it instead of returning ErrNotConnectedToWebsocket.
		gr.conn.Store(nil)
		if resp != nil {
			return nil, fmt.Errorf("failed to connect to websocket (HTTP %d): %w", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("failed to connect to websocket: %w", err)
	}
	ws.SetReadLimit(50 * 1024 * 1024)
	evtChan := make(chan wrappedEvent, 256)
	go gr.eventLoop(connCtx, evtChan)
	go gr.readLoop(connCtx, ws, cancel, evtChan)
	go gr.pingLoop(connCtx, ws)
	gr.connCtx.Store(&connCtx)
	gr.conn.Store(ws)
	return connCtx, nil
}

// Connect makes a single connection attempt and returns once it has either
// succeeded or failed. It does not reconnect; see ConnectWithRetry.
func (gr *GomuksRPC) Connect(ctx context.Context) error {
	_, err := gr.connectOnce(ctx)
	return err
}

const (
	reconnectBaseDelay = 1 * time.Second
	reconnectMaxDelay  = 30 * time.Second
	// connStableAfter is how long a connection must survive before it's
	// considered healthy and the backoff counter is reset. Without this, a
	// server that accepts connections and immediately drops them would be
	// hammered at the minimum delay forever.
	connStableAfter = 60 * time.Second
)

func reconnectDelay(attempt int) time.Duration {
	delay := reconnectBaseDelay << min(attempt, 5)
	if delay > reconnectMaxDelay {
		delay = reconnectMaxDelay
	}
	// Half jitter, so many clients reconnecting after the same server blip
	// don't all retry in lockstep.
	return delay/2 + time.Duration(rand.Int64N(int64(delay/2)))
}

func sleepContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// Resync drops the current connection and reconnects immediately, skipping the
// reconnect backoff. Forgetting the run ID means the server cannot replay from
// its event buffer and sends full initial data instead, which is equivalent to
// reloading the web client. That also recovers from state divergence, which a
// plain resume cannot.
func (gr *GomuksRPC) Resync() {
	gr.runID.Store(nil)
	gr.lastReqID.Store(0)
	select {
	case gr.resyncCh <- struct{}{}:
	default: // a resync is already pending
	}
	gr.Disconnect()
}

// waitBeforeRetry sleeps for d, returning early if a resync was requested.
// forced reports an early wake; alive is false when the client is shutting down.
func (gr *GomuksRPC) waitBeforeRetry(ctx context.Context, d time.Duration) (forced, alive bool) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return false, true
	case <-gr.resyncCh:
		return true, true
	case <-ctx.Done():
		return false, false
	}
}

// ConnectWithRetry maintains a websocket connection until ctx is cancelled,
// reconnecting with exponential backoff when it drops. Missed events are
// replayed from the server's buffer using the run ID and last received event
// ID, so a short disconnection is resumed rather than resynced.
func (gr *GomuksRPC) ConnectWithRetry(ctx context.Context) error {
	log := zerolog.Ctx(ctx)
	attempt := 0
	for {
		if attempt == 0 {
			gr.setConnState(ConnStateConnecting, nil)
		}
		connCtx, err := gr.connectOnce(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			attempt++
			delay := reconnectDelay(attempt)
			log.Warn().Err(err).
				Int("attempt", attempt).
				Dur("retry_in", delay).
				Msg("Failed to connect to websocket, retrying")
			gr.setConnState(ConnStateReconnecting, err)
			forced, alive := gr.waitBeforeRetry(ctx, delay)
			if !alive {
				return ctx.Err()
			}
			if forced {
				attempt = 0
			}
			continue
		}

		connectedAt := time.Now()
		gr.setConnState(ConnStateConnected, nil)
		log.Info().Msg("Connected to websocket")

		// readLoop cancels this when the connection dies.
		<-connCtx.Done()

		// Fail pending requests immediately instead of leaving callers blocked
		// until their own contexts expire.
		gr.clearPendingRequests()

		if ctx.Err() != nil {
			gr.setConnState(ConnStateDisconnected, nil)
			gr.Disconnect()
			return ctx.Err()
		}

		// A user-requested resync reconnects immediately instead of backing off.
		select {
		case <-gr.resyncCh:
			attempt = 0
			continue
		default:
		}

		if time.Since(connectedAt) > connStableAfter {
			attempt = 0
		}
		attempt++
		delay := reconnectDelay(attempt)
		cause := context.Cause(connCtx)
		log.Warn().Err(cause).
			Int("attempt", attempt).
			Dur("retry_in", delay).
			Msg("Websocket disconnected, reconnecting")
		gr.setConnState(ConnStateReconnecting, cause)
		forced, alive := gr.waitBeforeRetry(ctx, delay)
		if !alive {
			return ctx.Err()
		}
		if forced {
			attempt = 0
		}
	}
}

func (gr *GomuksRPC) Disconnect() {
	connCtx := gr.connCtx.Swap(nil)
	if connCtx == nil {
		connCtx = ptr.Ptr(context.Background())
	}
	if conn := gr.conn.Swap(nil); conn != nil {
		err := conn.Close(websocket.StatusNormalClosure, "Client disconnecting")
		if err != nil {
			zerolog.Ctx(*connCtx).Warn().Err(err).Msg("Failed to send close notice to websocket")
		}
	}
	if stopFn := gr.stop.Swap(nil); stopFn != nil {
		(*stopFn)()
	}
	gr.clearPendingRequests()
}

func (gr *GomuksRPC) cancelRequest(reqID int64, reason string) {
	ctxPtr := gr.connCtx.Load()
	conn := gr.conn.Load()
	if ctxPtr == nil || conn == nil {
		return
	}
	ctx := *ctxPtr
	if ctx.Err() != nil {
		return
	}
	wr, err := conn.Writer(ctx, websocket.MessageText)
	if err != nil {
		return
	}
	_ = json.NewEncoder(wr).Encode(jsoncmd.Cancel.Format(&jsoncmd.CancelRequestParams{
		RequestID: reqID,
		Reason:    reason,
	}, 0))
}

func writeWebsocketJSON(ctx context.Context, conn *websocket.Conn, data any) error {
	wr, err := conn.Writer(ctx, websocket.MessageText)
	if err != nil {
		return fmt.Errorf("failed to create websocket writer: %w", err)
	}
	err = json.NewEncoder(wr).Encode(data)
	if err != nil {
		return fmt.Errorf("failed to encode JSON command: %w", err)
	}
	err = wr.Close()
	if err != nil {
		return fmt.Errorf("failed to close websocket writer: %w", err)
	}
	return nil
}

func (gr *GomuksRPC) getNextRequestIDNoWait() (reqID int64) {
	reqID, _, _ = gr.getNextRequestID(false)
	return
}

func (gr *GomuksRPC) getNextRequestID(wait bool) (reqID int64, ch chan *jsoncmd.Container[json.RawMessage], remove func()) {
	gr.pendingRequestsLock.Lock()
	defer gr.pendingRequestsLock.Unlock()
	gr.reqIDCounter++
	reqID = gr.reqIDCounter
	if wait {
		ch = make(chan *jsoncmd.Container[json.RawMessage], 1)
		gr.pendingRequests[reqID] = ch
		remove = func() {
			gr.pendingRequestsLock.Lock()
			defer gr.pendingRequestsLock.Unlock()
			if gr.pendingRequests[reqID] == ch {
				close(ch)
				delete(gr.pendingRequests, reqID)
			}
		}
	}
	return
}

func executeRequest[Req, Resp any](gr *GomuksRPC, ctx context.Context, spec jsoncmd.ClientCommandSpec[Req, Resp], data Req) (Resp, error) {
	reqID, ch, remove := gr.getNextRequestID(true)
	defer remove()

	formatted := spec.Format(data, reqID)
	rawData, err := gr.rawRequest(ctx, formatted, reqID, formatted.Command, ch)
	if err != nil {
		return *new(Resp), err
	}
	return spec.Parse(rawData)
}

func executeRequestNoResponse[Req any](gr *GomuksRPC, ctx context.Context, spec jsoncmd.ClientCommandSpec[Req, *jsoncmd.Empty], data Req) error {
	_, err := executeRequest(gr, ctx, spec, data)
	return err
}

func (gr *GomuksRPC) rawRequest(
	ctx context.Context,
	payload any,
	reqID int64,
	cmd jsoncmd.Name,
	ch chan *jsoncmd.Container[json.RawMessage],
) (json.RawMessage, error) {
	conn := gr.conn.Load()
	if conn == nil {
		return nil, ErrNotConnectedToWebsocket
	}

	zerolog.Ctx(ctx).Trace().Int64("req_id", reqID).Stringer("command", cmd).Msg("Sending websocket request")
	err := writeWebsocketJSON(ctx, conn, payload)
	if err != nil {
		return nil, err
	}
	select {
	case resp := <-ch:
		if resp == nil {
			return nil, ErrWebsocketClosedBeforeResponseReceived
		} else if resp.Command == jsoncmd.RespError {
			var errMsg string
			_ = json.Unmarshal(resp.Data, &errMsg)
			if errMsg == "" {
				errMsg = string(resp.Data)
			}
			return nil, errors.New(errMsg)
		}
		return resp.Data, nil
	case <-ctx.Done():
		go gr.cancelRequest(reqID, context.Cause(ctx).Error())
		return nil, fmt.Errorf("context finished while waiting for response: %w", context.Cause(ctx))
	}
}

func (gr *GomuksRPC) eventLoop(ctx context.Context, evtChan <-chan wrappedEvent) {
	for {
		select {
		case evt := <-evtChan:
			if evt.Data == nil {
				return
			}
			gr.handleEvent(ctx, evt.Data)
			gr.lastReqID.Store(evt.ReqID)
		case <-ctx.Done():
			return
		}
	}
}

func (gr *GomuksRPC) handleEvent(ctx context.Context, evt any) {
	defer func() {
		err := recover()
		if err != nil {
			logEvt := zerolog.Ctx(ctx).Error().
				Bytes(zerolog.ErrorStackFieldName, debug.Stack())
			if realErr, ok := err.(error); ok {
				logEvt = logEvt.Err(realErr)
			} else {
				logEvt = logEvt.Any(zerolog.ErrorFieldName, err)
			}
			logEvt.Msg("Panic in event handler")
		}
	}()
	gr.EventHandler(ctx, evt)
}

const PingInterval = 15 * time.Second

func (gr *GomuksRPC) pingLoop(ctx context.Context, ws *websocket.Conn) {
	ticker := time.NewTicker(PingInterval)
	for {
		select {
		case <-ticker.C:
			err := writeWebsocketJSON(ctx, ws, &jsoncmd.Container[jsoncmd.PingParams]{
				Command:   jsoncmd.ReqPing,
				RequestID: gr.getNextRequestIDNoWait(),
				Data: jsoncmd.PingParams{
					LastReceivedID: gr.lastReqID.Load(),
				},
			})
			if err != nil {
				zerolog.Ctx(ctx).Err(err).Msg("Failed to send ping over websocket")
			}
		case <-ctx.Done():
			return
		}
	}
}

func (gr *GomuksRPC) readLoop(ctx context.Context, ws *websocket.Conn, cancelFunc context.CancelFunc, evtChan chan<- wrappedEvent) {
	log := zerolog.Ctx(ctx)
	defer cancelFunc()
	defer close(evtChan)
	for {
		if !gr.readLoopItem(ctx, log, ws, evtChan) {
			break
		}
	}
}

func parseEvent(ctx context.Context, evt *jsoncmd.Container[json.RawMessage]) any {
	var data any
	switch evt.Command {
	case jsoncmd.EventSyncComplete:
		data = &jsoncmd.SyncComplete{}
	case jsoncmd.EventSyncStatus:
		data = &jsoncmd.SyncStatus{}
	case jsoncmd.EventEventsDecrypted:
		data = &jsoncmd.EventsDecrypted{}
	case jsoncmd.EventTyping:
		data = &jsoncmd.Typing{}
	case jsoncmd.EventSendComplete:
		data = &jsoncmd.SendComplete{}
	case jsoncmd.EventClientState:
		data = &jsoncmd.ClientState{}
	case jsoncmd.EventRunID:
		data = &jsoncmd.RunData{}
	case jsoncmd.EventImageAuthToken:
		data = ptr.Ptr(jsoncmd.ImageAuthToken(""))
	case jsoncmd.EventInitComplete:
		// No data, just return immediately
		return &jsoncmd.InitComplete{}
	default:
		return evt
	}
	if err := json.Unmarshal(evt.Data, &data); err != nil {
		zerolog.Ctx(ctx).Err(err).
			Int64("req_id", evt.RequestID).
			Stringer("command", evt.Command).
			Msg("Failed to unmarshal event data")
		return evt
	}
	return data
}

var newlineBytes = []byte("\n")

func (gr *GomuksRPC) readLoopItem(ctx context.Context, log *zerolog.Logger, ws *websocket.Conn, evtHandler chan<- wrappedEvent) bool {
	var cmd *jsoncmd.Container[json.RawMessage]
	msgType, reader, err := ws.Reader(ctx)
	defer func() {
		if reader != nil {
			data, _ := io.ReadAll(reader)
			if len(data) != 0 && !bytes.Equal(data, newlineBytes) {
				log.Warn().
					Bytes("data", data).
					Msg("Unexpected data in websocket reader")
			}
		}
	}()
	if err != nil {
		log.Err(err).Msg("Error reading from websocket")
		return false
	} else if msgType != websocket.MessageText {
		log.Warn().Msg("Unexpected message type from websocket")
	} else if err = json.NewDecoder(reader).Decode(&cmd); err != nil {
		log.Err(err).Msg("Failed to decode JSON from websocket")
	} else if cmd.Command == jsoncmd.RespPong {
		log.Trace().Int64("ping_id", cmd.RequestID).Msg("Received pong from server")
	} else if cmd.Command == jsoncmd.RespError || cmd.Command == jsoncmd.RespSuccess {
		gr.pendingRequestsLock.Lock()
		pendingRequest, ok := gr.pendingRequests[cmd.RequestID]
		if ok {
			delete(gr.pendingRequests, cmd.RequestID)
		}
		gr.pendingRequestsLock.Unlock()
		if !ok {
			log.Warn().
				Int64("request_id", cmd.RequestID).
				RawJSON("response_data", cmd.Data).
				Msg("Received response for unknown request")
		} else {
			log.Trace().
				Int64("request_id", cmd.RequestID).
				Msg("Received response")
			pendingRequest <- cmd
			close(pendingRequest)
		}
	} else {
		parsedCmd := parseEvent(ctx, cmd)
		switch typedCmd := parsedCmd.(type) {
		case *jsoncmd.RunData:
			gr.setRunID(typedCmd.RunID)
		}
		we := wrappedEvent{Data: parsedCmd, ReqID: cmd.RequestID}
		select {
		case evtHandler <- we:
		default:
			log.Warn().
				Int64("req_id", cmd.RequestID).
				Stringer("command", cmd.Command).
				Msg("Event channel didn't accept event immediately, blocking websocket reads")
			select {
			case evtHandler <- we:
				log.Trace().
					Int64("req_id", cmd.RequestID).
					Stringer("command", cmd.Command).
					Msg("Event channel accepted event")
			case <-ctx.Done():
				return false
			}
		}
	}
	return true
}

func (gr *GomuksRPC) clearPendingRequests() {
	gr.pendingRequestsLock.Lock()
	defer gr.pendingRequestsLock.Unlock()
	for _, pendingRequest := range gr.pendingRequests {
		close(pendingRequest)
	}
	clear(gr.pendingRequests)
}

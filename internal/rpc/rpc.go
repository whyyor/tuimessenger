// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package rpc has the client's typed commands for hicli, sent through a Transport (in memory in tuimessenger).
package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"

	"github.com/rs/zerolog"
	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli/jsoncmd"
)

type EventHandler = func(ctx context.Context, event any)

type ConnState int

const (
	ConnStateConnecting ConnState = iota
	ConnStateConnected
	ConnStateReconnecting
	ConnStateDisconnected
)

func (cs ConnState) String() string {
	switch cs {
	case ConnStateConnecting:
		return "connecting"
	case ConnStateConnected:
		return "connected"
	case ConnStateReconnecting:
		return "reconnecting"
	case ConnStateDisconnected:
		return "disconnected"
	default:
		return "unknown"
	}
}

type ConnStateHandler = func(state ConnState, err error)

type DownloadMediaParams struct {
	MXC               id.ContentURI
	FallbackColor     string
	FallbackCharacter string
	AvatarThumbnail   bool
	Encrypted         bool
}

// Transport handles one command and returns its response.
type Transport func(ctx context.Context, cmd *jsoncmd.Container[json.RawMessage]) *jsoncmd.Container[json.RawMessage]

// Commands is every hicli command as a typed Go method (see requests.go), sent through a Transport.
type Commands struct {
	transport Transport
	lastReqID atomic.Int64
}

var _ jsoncmd.API = (*Commands)(nil)

// New makes the typed commands for a transport.
func New(transport Transport) *Commands {
	return &Commands{transport: transport}
}

func executeRequest[Req, Resp any](c *Commands, ctx context.Context, spec jsoncmd.ClientCommandSpec[Req, Resp], data Req) (Resp, error) {
	rawData, err := c.request(ctx, spec.Format(data, c.lastReqID.Add(1)))
	if err != nil {
		return *new(Resp), err
	}
	return spec.Parse(rawData)
}

func executeRequestNoResponse[Req any](c *Commands, ctx context.Context, spec jsoncmd.ClientCommandSpec[Req, *jsoncmd.Empty], data Req) error {
	_, err := executeRequest(c, ctx, spec, data)
	return err
}

// request round-trips the command through JSON, so the caller and hicli each get their own copy.
func (c *Commands) request(ctx context.Context, payload any) (json.RawMessage, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var cmd jsoncmd.Container[json.RawMessage]
	if err = json.Unmarshal(raw, &cmd); err != nil {
		return nil, err
	}
	resp := c.transport(ctx, &cmd)
	if resp == nil {
		return nil, errors.New("no response to command")
	}
	return responseData(resp)
}

func responseData(resp *jsoncmd.Container[json.RawMessage]) (json.RawMessage, error) {
	if resp.Command == jsoncmd.RespError {
		var errMsg string
		_ = json.Unmarshal(resp.Data, &errMsg)
		if errMsg == "" {
			errMsg = string(resp.Data)
		}
		return nil, errors.New(errMsg)
	}
	return resp.Data, nil
}

// ParseEvent decodes an event from hicli into its Go type, or returns the container if the type is unknown.
func ParseEvent(ctx context.Context, evt *jsoncmd.Container[json.RawMessage]) any {
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

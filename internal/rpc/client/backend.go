// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package client

import (
	"context"
	"io"
	"net/http"
	"net/url"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
	"go.mau.fi/gomuks/pkg/rpc"
)

// Backend is what the client talks to: the gomuks daemon over a websocket, or a backend in the same process.
type Backend interface {
	jsoncmd.GomuksAPI
	// Run connects and delivers events to handler until ctx is cancelled; onState reports connection changes.
	Run(ctx context.Context, handler rpc.EventHandler, onState rpc.ConnStateHandler) error
	// Resync reconnects right away and asks for a fresh copy of the client state.
	Resync()
	Disconnect()
	UploadMedia(ctx context.Context, filename string, encrypt bool, data io.Reader) (*event.MessageEventContent, error)
	DownloadMedia(ctx context.Context, params rpc.DownloadMediaParams) (*http.Response, error)
	// MediaURL is an HTTP address the user can open to get a media file.
	MediaURL(mxc id.ContentURI, query url.Values) string
}

var _ Backend = (*rpc.GomuksRPC)(nil)

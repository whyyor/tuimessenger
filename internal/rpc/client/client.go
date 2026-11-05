// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package client

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strconv"
	"sync"

	"go.mau.fi/util/exsync"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/hicli/cmdspec"
	"github.com/whyyor/tuimessenger/internal/hicli/database"
	"github.com/whyyor/tuimessenger/internal/hicli/jsoncmd"
	"github.com/whyyor/tuimessenger/internal/rpc"
	"github.com/whyyor/tuimessenger/internal/rpc/store"
)

type Client struct {
	jsoncmd.API
	*store.Store
	Backend Backend

	InitComplete *exsync.Event
	EventHandler rpc.EventHandler

	SendNotification func(room *store.RoomStore, notif jsoncmd.SyncNotification)
	// OnNotice gets hicli's own replies (fake events) to sends; returning
	// true keeps them out of the timeline.
	OnNotice func(evt *database.Event) bool

	stateRequestQueue     []database.RoomStateGUID
	stateRequestQueueLock sync.Mutex
}

// New makes a client for a backend (in tuimessenger: hicli in the same process).
func New(backend Backend) *Client {
	return &Client{
		API:          backend,
		Backend:      backend,
		Store:        store.NewStore(),
		InitComplete: exsync.NewEvent(),
	}
}

// Connect runs the backend connection until ctx is cancelled, applying its events to the store.
func (gc *Client) Connect(ctx context.Context, onState rpc.ConnStateHandler) error {
	return gc.Backend.Run(ctx, gc.handleEvent, onState)
}

func (gc *Client) handleEvent(ctx context.Context, rawEvt any) {
	switch evt := rawEvt.(type) {
	case *jsoncmd.ClientState:
		gc.Store.ClientState = *evt
	case *jsoncmd.SyncStatus:
		// TODO
	case *jsoncmd.InitComplete:
		gc.InitComplete.Set()
	case *jsoncmd.SyncComplete:
		gc.Store.ApplySync(evt)
		for _, room := range evt.Rooms {
			if len(room.Notifications) == 0 {
				continue
			}
			roomStore := gc.Store.GetRoom(room.Meta.ID)
			for _, notif := range room.Notifications {
				notif.Room = roomStore.Meta.Current()
				notif.Event = roomStore.GetEventByRowID(notif.RowID)
				if notif.Event == nil {
					continue
				}
				gc.SendNotification(roomStore, notif)
			}
		}
	case *jsoncmd.EventsDecrypted:
		callRoomMethod(gc, evt.RoomID, (*store.RoomStore).ApplyDecrypted, evt)
	case *jsoncmd.SendComplete:
		callRoomMethod(gc, evt.Event.RoomID, (*store.RoomStore).ApplySendComplete, evt.Event)
	case *jsoncmd.ImageAuthToken:
		gc.Store.ImageAuthToken = string(*evt)
	case *jsoncmd.Typing:
		callRoomMethod(gc, evt.RoomID, (*store.RoomStore).ApplyTyping, evt.UserIDs)
	}
	if gc.EventHandler != nil {
		gc.EventHandler(ctx, rawEvt)
	}
}

func callRoomMethod[T any](gc *Client, roomID id.RoomID, fn func(room *store.RoomStore, val T), val T) {
	room := gc.Store.GetRoom(roomID)
	if room == nil {
		return
	}
	fn(room, val)
}

func (gc *Client) RequestEvent(ctx context.Context, room *store.RoomStore, eventID id.EventID) {

}

func (gc *Client) SendMessage(ctx context.Context, params *jsoncmd.SendMessageParams) error {
	room := gc.Store.GetRoom(params.RoomID)
	if room == nil {
		return fmt.Errorf("room not found in store")
	}
	dbEvt, err := gc.API.SendMessage(ctx, params)
	if err != nil {
		return err
	} else if dbEvt != nil {
		if dbEvt.Sender == cmdspec.FakeSender && gc.OnNotice != nil && gc.OnNotice(dbEvt) {
			return nil
		}
		room.ApplyPending(dbEvt)
	}
	return nil
}

func (gc *Client) QueueRoomStateRequest(key database.RoomStateGUID) {
	gc.stateRequestQueueLock.Lock()
	defer gc.stateRequestQueueLock.Unlock()
	gc.stateRequestQueue = append(gc.stateRequestQueue, key)
}

func (gc *Client) FlushRoomStateRequests(ctx context.Context) error {
	gc.stateRequestQueueLock.Lock()
	keys := gc.stateRequestQueue
	gc.stateRequestQueue = nil
	gc.stateRequestQueueLock.Unlock()
	if len(keys) == 0 {
		return nil
	}
	return gc.LoadSpecificRoomState(ctx, keys)
}

func (gc *Client) LoadSpecificRoomState(ctx context.Context, keys []database.RoomStateGUID) error {
	keys = slices.DeleteFunc(keys, func(guid database.RoomStateGUID) bool {
		room := gc.Store.GetRoom(guid.RoomID)
		return room == nil || room.GetStateEvent(guid.Type, guid.StateKey) != nil
	})
	if len(keys) == 0 {
		return nil
	}
	resp, err := gc.API.GetSpecificRoomState(ctx, &jsoncmd.GetSpecificRoomStateParams{Keys: keys})
	if err != nil {
		return err
	}
	for _, evt := range resp {
		room := gc.Store.GetRoom(evt.RoomID)
		if room == nil {
			continue
		}
		room.ApplyState(evt)
	}
	return nil
}

func (gc *Client) LoadRoomState(ctx context.Context, roomID id.RoomID, includeMembers, refetch bool) error {
	room := gc.Store.GetRoom(roomID)
	if room == nil {
		return fmt.Errorf("room not found in store")
	}
	room.StateLoadLock.Lock()
	defer room.StateLoadLock.Unlock()
	if !refetch && (room.FullMembersLoaded.Load() || (!includeMembers && room.StateLoaded.Load())) {
		return nil
	}
	resp, err := gc.API.GetRoomState(ctx, &jsoncmd.GetRoomStateParams{
		RoomID:         roomID,
		Refetch:        refetch,
		FetchMembers:   !room.Meta.Current().HasMemberList,
		IncludeMembers: includeMembers,
	})
	if err != nil {
		return err
	}
	room.Meta.Current().HasMemberList = true
	room.ApplyFullState(resp, !includeMembers)
	return nil
}

func (gc *Client) LoadMoreHistory(ctx context.Context, roomID id.RoomID) error {
	room := gc.Store.GetRoom(roomID)
	if room == nil {
		return fmt.Errorf("room not found in store")
	} else if !room.Paginating.CompareAndSwap(false, true) {
		return fmt.Errorf("already paginating room")
	}
	defer room.Paginating.Store(false)
	oldestRowID, count := room.GetPaginationParams()
	resp, err := gc.API.Paginate(ctx, &jsoncmd.PaginateParams{
		RoomID:        room.ID,
		MaxTimelineID: oldestRowID,
		Limit:         count,
		Reset:         false,
	})
	if err != nil {
		return err
	}
	room.ApplyPagination(resp)
	return nil
}

func (gc *Client) GetDownloadURL(mxc id.ContentURI, encrypted, preauthed bool) string {
	query := url.Values{
		"encrypted": {strconv.FormatBool(encrypted)},
	}
	if preauthed {
		query.Set("image_auth", gc.Store.ImageAuthToken)
	}
	return gc.Backend.MediaURL(mxc, query)
}

func (gc *Client) Download(mxc id.ContentURI, encrypted bool) ([]byte, error) {
	resp, err := gc.Backend.DownloadMedia(context.TODO(), rpc.DownloadMediaParams{
		MXC:       mxc,
		Encrypted: encrypted,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

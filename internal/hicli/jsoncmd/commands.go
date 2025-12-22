// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package jsoncmd

import (
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/database"
)

type Container[T any] struct {
	Command   Name  `json:"command"`
	RequestID int64 `json:"request_id"`
	Data      T     `json:"data"`
}

type Name string

func (n Name) String() string {
	return string(n)
}

const (
	ReqGetState                 Name = "get_state"
	ReqCancel                   Name = "cancel"
	ReqSendMessage              Name = "send_message"
	ReqSendEvent                Name = "send_event"
	ReqResendEvent              Name = "resend_event"
	ReqReportEvent              Name = "report_event"
	ReqRedactEvent              Name = "redact_event"
	ReqSetState                 Name = "set_state"
	ReqUpdateDelayedEvent       Name = "update_delayed_event"
	ReqSetMembership            Name = "set_membership"
	ReqSetAccountData           Name = "set_account_data"
	ReqMarkRead                 Name = "mark_read"
	ReqSetTyping                Name = "set_typing"
	ReqGetProfile               Name = "get_profile"
	ReqSetProfileField          Name = "set_profile_field"
	ReqGetMutualRooms           Name = "get_mutual_rooms"
	ReqTrackUserDevices         Name = "track_user_devices"
	ReqGetProfileEncryptionInfo Name = "get_profile_encryption_info"
	ReqGetEvent                 Name = "get_event"
	ReqGetEventContext          Name = "get_event_context"
	ReqPaginateManual           Name = "paginate_manual"
	ReqGetRelatedEvents         Name = "get_related_events"
	ReqGetRoomState             Name = "get_room_state"
	ReqGetSpecificRoomState     Name = "get_specific_room_state"
	ReqGetReceipts              Name = "get_receipts"
	ReqPaginate                 Name = "paginate"
	ReqGetRoomSummary           Name = "get_room_summary"
	ReqGetSpaceHierarchy        Name = "get_space_hierarchy"
	ReqJoinRoom                 Name = "join_room"
	ReqKnockRoom                Name = "knock_room"
	ReqLeaveRoom                Name = "leave_room"
	ReqCreateRoom               Name = "create_room"
	ReqMuteRoom                 Name = "mute_room"
	ReqEnsureGroupSessionShared Name = "ensure_group_session_shared"
	ReqSendToDevice             Name = "send_to_device"
	ReqResolveAlias             Name = "resolve_alias"
	ReqRequestOpenIDToken       Name = "request_openid_token"
	ReqLogout                   Name = "logout"
	ReqLogin                    Name = "login"
	ReqLoginCustom              Name = "login_custom"
	ReqVerify                   Name = "verify"
	ReqDiscoverHomeserver       Name = "discover_homeserver"
	ReqGetLoginFlows            Name = "get_login_flows"
	ReqRegisterPush             Name = "register_push"
	ReqListenToDevice           Name = "listen_to_device"
	ReqGetTurnServers           Name = "get_turn_servers"
	ReqGetMediaConfig           Name = "get_media_config"
	ReqCalculateRoomID          Name = "calculate_room_id"

	RespError   Name = "error"
	RespSuccess Name = "response"

	ReqPing  Name = "ping"
	RespPong Name = "pong"

	EventSyncComplete    Name = "sync_complete"
	EventSyncStatus      Name = "sync_status"
	EventEventsDecrypted Name = "events_decrypted"
	EventTyping          Name = "typing"
	EventSendComplete    Name = "send_complete"
	EventClientState     Name = "client_state"
	EventImageAuthToken  Name = "image_auth_token"
	EventInitComplete    Name = "init_complete"
	EventRunID           Name = "run_id"
)

var (
	GetState                 = &CommandSpecWithoutRequest[*ClientState]{Name: ReqGetState}
	Cancel                   = &CommandSpec[*CancelRequestParams, bool]{Name: ReqCancel}
	SendMessage              = &CommandSpec[*SendMessageParams, *database.Event]{Name: ReqSendMessage}
	SendEvent                = &CommandSpec[*SendEventParams, *database.Event]{Name: ReqSendEvent}
	ResendEvent              = &CommandSpec[*ResendEventParams, *database.Event]{Name: ReqResendEvent}
	ReportEvent              = &CommandSpecWithoutResponse[*ReportEventParams]{Name: ReqReportEvent}
	RedactEvent              = &CommandSpec[*RedactEventParams, *mautrix.RespSendEvent]{Name: ReqRedactEvent}
	SetState                 = &CommandSpec[*SendStateEventParams, id.EventID]{Name: ReqSetState}
	UpdateDelayedEvent       = &CommandSpec[*UpdateDelayedEventParams, *mautrix.RespUpdateDelayedEvent]{Name: ReqUpdateDelayedEvent}
	SetMembership            = &CommandSpecWithoutResponse[*SetMembershipParams]{Name: ReqSetMembership}
	SetAccountData           = &CommandSpecWithoutResponse[*SetAccountDataParams]{Name: ReqSetAccountData}
	MarkRead                 = &CommandSpecWithoutResponse[*MarkReadParams]{Name: ReqMarkRead}
	SetTyping                = &CommandSpecWithoutResponse[*SetTypingParams]{Name: ReqSetTyping}
	GetProfile               = &CommandSpec[*GetProfileParams, *mautrix.RespUserProfile]{Name: ReqGetProfile}
	SetProfileField          = &CommandSpecWithoutResponse[*SetProfileFieldParams]{Name: ReqSetProfileField}
	GetMutualRooms           = &CommandSpec[*GetProfileParams, []id.RoomID]{Name: ReqGetMutualRooms}
	TrackUserDevices         = &CommandSpec[*GetProfileParams, *ProfileEncryptionInfo]{Name: ReqTrackUserDevices}
	GetProfileEncryptionInfo = &CommandSpec[*GetProfileParams, *ProfileEncryptionInfo]{Name: ReqGetProfileEncryptionInfo}
	GetEvent                 = &CommandSpec[*GetEventParams, *database.Event]{Name: ReqGetEvent}
	GetEventContext          = &CommandSpec[*GetEventContextParams, *EventContextResponse]{Name: ReqGetEventContext}
	PaginateManual           = &CommandSpec[*PaginateManualParams, *ManualPaginationResponse]{Name: ReqPaginateManual}
	GetRelatedEvents         = &CommandSpec[*GetRelatedEventsParams, []*database.Event]{Name: ReqGetRelatedEvents}
	GetRoomState             = &CommandSpec[*GetRoomStateParams, []*database.Event]{Name: ReqGetRoomState}
	GetSpecificRoomState     = &CommandSpec[*GetSpecificRoomStateParams, []*database.Event]{Name: ReqGetSpecificRoomState}
	GetReceipts              = &CommandSpec[*GetReceiptsParams, map[id.EventID][]*database.Receipt]{Name: ReqGetReceipts}
	Paginate                 = &CommandSpec[*PaginateParams, *PaginationResponse]{Name: ReqPaginate}
	GetRoomSummary           = &CommandSpec[*GetRoomSummaryParams, *mautrix.RespRoomSummary]{Name: ReqGetRoomSummary}
	GetSpaceHierarchy        = &CommandSpec[*GetHierarchyParams, *mautrix.RespHierarchy]{Name: ReqGetSpaceHierarchy}
	JoinRoom                 = &CommandSpec[*JoinRoomParams, *mautrix.RespJoinRoom]{Name: ReqJoinRoom}
	KnockRoom                = &CommandSpec[*JoinRoomParams, *mautrix.RespKnockRoom]{Name: ReqKnockRoom}
	LeaveRoom                = &CommandSpec[*LeaveRoomParams, *mautrix.RespLeaveRoom]{Name: ReqLeaveRoom}
	CreateRoom               = &CommandSpec[*mautrix.ReqCreateRoom, *mautrix.RespCreateRoom]{Name: ReqCreateRoom}
	MuteRoom                 = &CommandSpec[*MuteRoomParams, bool]{Name: ReqMuteRoom}
	EnsureGroupSessionShared = &CommandSpecWithoutResponse[*EnsureGroupSessionSharedParams]{Name: ReqEnsureGroupSessionShared}
	SendToDevice             = &CommandSpec[*SendToDeviceParams, *mautrix.RespSendToDevice]{Name: ReqSendToDevice}
	ResolveAlias             = &CommandSpec[*ResolveAliasParams, *mautrix.RespAliasResolve]{Name: ReqResolveAlias}
	RequestOpenIDToken       = &CommandSpecWithoutRequest[*mautrix.RespOpenIDToken]{Name: ReqRequestOpenIDToken}
	Logout                   = &CommandSpecWithoutData{Name: ReqLogout}
	Login                    = &CommandSpecWithoutResponse[*LoginParams]{Name: ReqLogin}
	LoginCustom              = &CommandSpecWithoutResponse[*LoginCustomParams]{Name: ReqLoginCustom}
	Verify                   = &CommandSpecWithoutResponse[*VerifyParams]{Name: ReqVerify}
	DiscoverHomeserver       = &CommandSpec[*DiscoverHomeserverParams, *mautrix.ClientWellKnown]{Name: ReqDiscoverHomeserver}
	GetLoginFlows            = &CommandSpec[*GetLoginFlowsParams, *mautrix.RespLoginFlows]{Name: ReqGetLoginFlows}
	RegisterPush             = &CommandSpecWithoutResponse[*database.PushRegistration]{Name: ReqRegisterPush}
	ListenToDevice           = &CommandSpec[bool, bool]{Name: ReqListenToDevice}
	GetTurnServers           = &CommandSpecWithoutRequest[*mautrix.RespTurnServer]{Name: ReqGetTurnServers}
	GetMediaConfig           = &CommandSpecWithoutRequest[*mautrix.RespMediaConfig]{Name: ReqGetMediaConfig}
	CalculateRoomID          = &CommandSpec[*CalculateRoomIDParams, id.RoomID]{Name: ReqCalculateRoomID}

	SpecSyncComplete    = &EventSpec[*SyncComplete]{Name: EventSyncComplete}
	SpecSyncStatus      = &EventSpec[*SyncStatus]{Name: EventSyncStatus}
	SpecEventsDecrypted = &EventSpec[*EventsDecrypted]{Name: EventEventsDecrypted}
	SpecTyping          = &EventSpec[*Typing]{Name: EventTyping}
	SpecSendComplete    = &EventSpec[*SendComplete]{Name: EventSendComplete}
	SpecClientState     = &EventSpec[*ClientState]{Name: EventClientState}

	SpecImageAuthToken = &EventSpec[ImageAuthToken]{Name: EventImageAuthToken}
	SpecInitComplete   = &EventSpec[Empty]{Name: EventInitComplete}
	SpecRunID          = &EventSpec[*RunData]{Name: EventRunID}
)

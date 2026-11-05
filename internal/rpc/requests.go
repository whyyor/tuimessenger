// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rpc

import (
	"context"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
	"maunium.net/go/mautrix/oauth"

	"github.com/whyyor/tuimessenger/internal/hicli/database"
	"github.com/whyyor/tuimessenger/internal/hicli/jsoncmd"
)

func (c *Commands) GetState(ctx context.Context) (*jsoncmd.ClientState, error) {
	return executeRequest(c, ctx, jsoncmd.GetState, nil)
}

func (c *Commands) SendMessage(ctx context.Context, params *jsoncmd.SendMessageParams) (*database.Event, error) {
	return executeRequest(c, ctx, jsoncmd.SendMessage, params)
}

func (c *Commands) SendEvent(ctx context.Context, params *jsoncmd.SendEventParams) (*database.Event, error) {
	return executeRequest(c, ctx, jsoncmd.SendEvent, params)
}

func (c *Commands) SendStickyEvent(ctx context.Context, params *jsoncmd.SendStickyEventParams) (id.EventID, error) {
	return executeRequest(c, ctx, jsoncmd.SendStickyEvent, params)
}

func (c *Commands) ResendEvent(ctx context.Context, params *jsoncmd.ResendEventParams) (*database.Event, error) {
	return executeRequest(c, ctx, jsoncmd.ResendEvent, params)
}

func (c *Commands) ReportEvent(ctx context.Context, params *jsoncmd.ReportEventParams) error {
	return executeRequestNoResponse(c, ctx, jsoncmd.ReportEvent, params)
}

func (c *Commands) RedactEvent(ctx context.Context, params *jsoncmd.RedactEventParams) (*mautrix.RespSendEvent, error) {
	return executeRequest(c, ctx, jsoncmd.RedactEvent, params)
}

func (c *Commands) SetState(ctx context.Context, params *jsoncmd.SendStateEventParams) (id.EventID, error) {
	return executeRequest(c, ctx, jsoncmd.SetState, params)
}

func (c *Commands) UpdateDelayedEvent(ctx context.Context, params *jsoncmd.UpdateDelayedEventParams) (*mautrix.RespUpdateDelayedEvent, error) {
	return executeRequest(c, ctx, jsoncmd.UpdateDelayedEvent, params)
}

func (c *Commands) SetMembership(ctx context.Context, params *jsoncmd.SetMembershipParams) error {
	return executeRequestNoResponse(c, ctx, jsoncmd.SetMembership, params)
}

func (c *Commands) SetAccountData(ctx context.Context, params *jsoncmd.SetAccountDataParams) error {
	return executeRequestNoResponse(c, ctx, jsoncmd.SetAccountData, params)
}

func (c *Commands) MarkRead(ctx context.Context, params *jsoncmd.MarkReadParams) error {
	return executeRequestNoResponse(c, ctx, jsoncmd.MarkRead, params)
}

func (c *Commands) SetTyping(ctx context.Context, params *jsoncmd.SetTypingParams) error {
	return executeRequestNoResponse(c, ctx, jsoncmd.SetTyping, params)
}

func (c *Commands) SetViewingRoom(ctx context.Context, params *jsoncmd.SetViewingRoomParams) error {
	return executeRequestNoResponse(c, ctx, jsoncmd.SetViewingRoom, params)
}

func (c *Commands) GetProfile(ctx context.Context, params *jsoncmd.GetProfileParams) (*jsoncmd.GetProfileResponse, error) {
	return executeRequest(c, ctx, jsoncmd.GetProfile, params)
}

func (c *Commands) SetProfileField(ctx context.Context, params *jsoncmd.SetProfileFieldParams) error {
	return executeRequestNoResponse(c, ctx, jsoncmd.SetProfileField, params)
}

func (c *Commands) GetMutualRooms(ctx context.Context, params *jsoncmd.GetMutualRoomsParams) (*mautrix.RespMutualRooms, error) {
	return executeRequest(c, ctx, jsoncmd.GetMutualRooms, params)
}

func (c *Commands) TrackUserDevices(ctx context.Context, params *jsoncmd.GetProfileParams) (*jsoncmd.ProfileEncryptionInfo, error) {
	return executeRequest(c, ctx, jsoncmd.TrackUserDevices, params)
}

func (c *Commands) ResetMasterKeyTOFU(ctx context.Context, params *jsoncmd.ResetMasterKeyTOFUParams) (*jsoncmd.ProfileEncryptionInfo, error) {
	return executeRequest(c, ctx, jsoncmd.ResetMasterKeyTOFU, params)
}

func (c *Commands) GetProfileEncryptionInfo(ctx context.Context, params *jsoncmd.GetProfileParams) (*jsoncmd.ProfileEncryptionInfo, error) {
	return executeRequest(c, ctx, jsoncmd.GetProfileEncryptionInfo, params)
}

func (c *Commands) GetOwnDevices(ctx context.Context) (*jsoncmd.GetOwnDevicesResponse, error) {
	return executeRequest(c, ctx, jsoncmd.GetOwnDevices, nil)
}

func (c *Commands) GetEvent(ctx context.Context, params *jsoncmd.GetEventParams) (*database.Event, error) {
	return executeRequest(c, ctx, jsoncmd.GetEvent, params)
}

func (c *Commands) GetEventByRowID(ctx context.Context, params *jsoncmd.GetEventByRowIDParams) (*database.Event, error) {
	return executeRequest(c, ctx, jsoncmd.GetEventByRowID, params)
}

func (c *Commands) GetRelatedEvents(ctx context.Context, params *jsoncmd.GetRelatedEventsParams) ([]*database.Event, error) {
	return executeRequest(c, ctx, jsoncmd.GetRelatedEvents, params)
}

func (c *Commands) GetStickyEvents(ctx context.Context, params *jsoncmd.GetStickyEventsParams) ([]*database.Event, error) {
	return executeRequest(c, ctx, jsoncmd.GetStickyEvents, params)
}

func (c *Commands) GetEventContext(ctx context.Context, params *jsoncmd.GetEventContextParams) (*jsoncmd.EventContextResponse, error) {
	return executeRequest(c, ctx, jsoncmd.GetEventContext, params)
}

func (c *Commands) GetRoomState(ctx context.Context, params *jsoncmd.GetRoomStateParams) ([]*database.Event, error) {
	return executeRequest(c, ctx, jsoncmd.GetRoomState, params)
}

func (c *Commands) GetSpecificRoomState(ctx context.Context, params *jsoncmd.GetSpecificRoomStateParams) ([]*database.Event, error) {
	return executeRequest(c, ctx, jsoncmd.GetSpecificRoomState, params)
}

func (c *Commands) GetReceipts(ctx context.Context, params *jsoncmd.GetReceiptsParams) (map[id.EventID][]*database.Receipt, error) {
	return executeRequest(c, ctx, jsoncmd.GetReceipts, params)
}

func (c *Commands) Paginate(ctx context.Context, params *jsoncmd.PaginateParams) (*jsoncmd.PaginationResponse, error) {
	return executeRequest(c, ctx, jsoncmd.Paginate, params)
}

func (c *Commands) PaginateManual(ctx context.Context, params *jsoncmd.PaginateManualParams) (*jsoncmd.ManualPaginationResponse, error) {
	return executeRequest(c, ctx, jsoncmd.PaginateManual, params)
}

func (c *Commands) SearchLocal(ctx context.Context, params *jsoncmd.SearchParams) (*jsoncmd.ManualPaginationResponse, error) {
	return executeRequest(c, ctx, jsoncmd.SearchLocal, params)
}

func (c *Commands) SearchServer(ctx context.Context, params *jsoncmd.SearchServerParams) (*jsoncmd.ManualPaginationResponse, error) {
	return executeRequest(c, ctx, jsoncmd.SearchServer, params)
}

func (c *Commands) GetMentions(ctx context.Context, params *jsoncmd.GetMentionsParams) ([]*database.Event, error) {
	return executeRequest(c, ctx, jsoncmd.GetMentions, params)
}

func (c *Commands) GetRoomSummary(ctx context.Context, params *jsoncmd.GetRoomSummaryParams) (*mautrix.RespRoomSummary, error) {
	return executeRequest(c, ctx, jsoncmd.GetRoomSummary, params)
}

func (c *Commands) GetSpaceHierarchy(ctx context.Context, params *jsoncmd.GetHierarchyParams) (*mautrix.RespHierarchy, error) {
	return executeRequest(c, ctx, jsoncmd.GetSpaceHierarchy, params)
}

func (c *Commands) JoinRoom(ctx context.Context, params *jsoncmd.JoinRoomParams) (*mautrix.RespJoinRoom, error) {
	return executeRequest(c, ctx, jsoncmd.JoinRoom, params)
}

func (c *Commands) KnockRoom(ctx context.Context, params *jsoncmd.JoinRoomParams) (*mautrix.RespKnockRoom, error) {
	return executeRequest(c, ctx, jsoncmd.KnockRoom, params)
}

func (c *Commands) LeaveRoom(ctx context.Context, params *jsoncmd.LeaveRoomParams) (*mautrix.RespLeaveRoom, error) {
	return executeRequest(c, ctx, jsoncmd.LeaveRoom, params)
}

func (c *Commands) CreateRoom(ctx context.Context, params *mautrix.ReqCreateRoom) (*mautrix.RespCreateRoom, error) {
	return executeRequest(c, ctx, jsoncmd.CreateRoom, params)
}

func (c *Commands) MuteRoom(ctx context.Context, params *jsoncmd.MuteRoomParams) (bool, error) {
	return executeRequest(c, ctx, jsoncmd.MuteRoom, params)
}

func (c *Commands) UpdatePushRule(ctx context.Context, params *jsoncmd.UpdatePushRuleParams) error {
	return executeRequestNoResponse(c, ctx, jsoncmd.UpdatePushRule, params)
}

func (c *Commands) EnsureGroupSessionShared(ctx context.Context, params *jsoncmd.EnsureGroupSessionSharedParams) error {
	return executeRequestNoResponse(c, ctx, jsoncmd.EnsureGroupSessionShared, params)
}

func (c *Commands) SendToDevice(ctx context.Context, params *jsoncmd.SendToDeviceParams) (*mautrix.RespSendToDevice, error) {
	return executeRequest(c, ctx, jsoncmd.SendToDevice, params)
}

func (c *Commands) ResolveAlias(ctx context.Context, params *jsoncmd.ResolveAliasParams) (*mautrix.RespAliasResolve, error) {
	return executeRequest(c, ctx, jsoncmd.ResolveAlias, params)
}

func (c *Commands) RequestOpenIDToken(ctx context.Context) (*mautrix.RespOpenIDToken, error) {
	return executeRequest(c, ctx, jsoncmd.RequestOpenIDToken, nil)
}

func (c *Commands) Logout(ctx context.Context) error {
	return executeRequestNoResponse(c, ctx, jsoncmd.Logout, nil)
}

func (c *Commands) Login(ctx context.Context, params *jsoncmd.LoginParams) error {
	return executeRequestNoResponse(c, ctx, jsoncmd.Login, params)
}

func (c *Commands) LoginCustom(ctx context.Context, params *jsoncmd.LoginCustomParams) error {
	return executeRequestNoResponse(c, ctx, jsoncmd.LoginCustom, params)
}

func (c *Commands) Verify(ctx context.Context, params *jsoncmd.VerifyParams) error {
	return executeRequestNoResponse(c, ctx, jsoncmd.Verify, params)
}

func (c *Commands) DiscoverHomeserver(ctx context.Context, params *jsoncmd.DiscoverHomeserverParams) (*mautrix.ClientWellKnown, error) {
	return executeRequest(c, ctx, jsoncmd.DiscoverHomeserver, params)
}

func (c *Commands) GetLoginFlows(ctx context.Context, params *jsoncmd.GetLoginFlowsParams) (*jsoncmd.LoginFlowsResponse, error) {
	return executeRequest(c, ctx, jsoncmd.GetLoginFlows, params)
}

func (c *Commands) GetVersions(ctx context.Context) (*mautrix.RespVersions, error) {
	return executeRequest(c, ctx, jsoncmd.GetVersions, nil)
}

func (c *Commands) OAuthRegisterClient(ctx context.Context, params *jsoncmd.OAuthRegisterClientParams) (*oauth.ClientMetadata, error) {
	return executeRequest(c, ctx, jsoncmd.OAuthRegisterClient, params)
}

func (c *Commands) OAuthGetAuthorizationURL(ctx context.Context, params *jsoncmd.OAuthGetAuthorizationURLParams) (*oauth.AuthorizationCodeResponse, error) {
	return executeRequest(c, ctx, jsoncmd.OAuthGetAuthorizationURL, params)
}

func (c *Commands) OAuthExchangeToken(ctx context.Context, params *jsoncmd.OAuthExchangeTokenParams) error {
	return executeRequestNoResponse(c, ctx, jsoncmd.OAuthExchangeToken, params)
}

func (c *Commands) OAuthGenerateDeviceCode(ctx context.Context, params *jsoncmd.OAuthGenerateDeviceCodeParams) (*oauth.DeviceCodeResponse, error) {
	return executeRequest(c, ctx, jsoncmd.OAuthGenerateDeviceCode, params)
}

func (c *Commands) OAuthSimpleDeviceCode(ctx context.Context, params *jsoncmd.OAuthSimpleDeviceCodeParams) (*oauth.DeviceCodeResponse, error) {
	return executeRequest(c, ctx, jsoncmd.OAuthSimpleDeviceCode, params)
}

func (c *Commands) OAuthPollDeviceCode(ctx context.Context, params *jsoncmd.OAuthPollDeviceCodeParams) error {
	return executeRequestNoResponse(c, ctx, jsoncmd.OAuthPollDeviceCode, params)
}

func (c *Commands) RegisterPush(ctx context.Context, params *database.PushRegistration) error {
	return executeRequestNoResponse(c, ctx, jsoncmd.RegisterPush, params)
}

func (c *Commands) ListenToDevice(ctx context.Context, listen bool) (bool, error) {
	return executeRequest(c, ctx, jsoncmd.ListenToDevice, listen)
}

func (c *Commands) GetTurnServers(ctx context.Context) (*mautrix.RespTurnServer, error) {
	return executeRequest(c, ctx, jsoncmd.GetTurnServers, nil)
}

func (c *Commands) GetRTCTransports(ctx context.Context) (*mautrix.RespRTCTransports, error) {
	return executeRequest(c, ctx, jsoncmd.GetRTCTransports, nil)
}

func (c *Commands) GetMediaConfig(ctx context.Context) (*mautrix.RespMediaConfig, error) {
	return executeRequest(c, ctx, jsoncmd.GetMediaConfig, nil)
}

func (c *Commands) CalculateRoomID(ctx context.Context, params *jsoncmd.CalculateRoomIDParams) (id.RoomID, error) {
	return executeRequest(c, ctx, jsoncmd.CalculateRoomID, params)
}

func (c *Commands) RerequestSession(ctx context.Context, params *jsoncmd.RerequestSessionParams) error {
	return executeRequestNoResponse(c, ctx, jsoncmd.RerequestSession, params)
}

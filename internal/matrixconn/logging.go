// Package matrixconn contains our replacement for a Matrix homeserver.
package matrixconn

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/status"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// LoggingConnector pretends to be a homeserver and only prints what the bridge asks of it.
type LoggingConnector struct {
	br         *bridgev2.Bridge
	log        zerolog.Logger
	serverName string
	owner      id.UserID
}

// Compile-time check that we implement every method bridgev2 needs.
var _ bridgev2.MatrixConnector = (*LoggingConnector)(nil)

func NewLogging(log zerolog.Logger, bridgeID string, owner id.UserID) *LoggingConnector {
	return &LoggingConnector{
		log:        log.With().Str("component", "matrix").Logger(),
		serverName: bridgeID + ".localhost",
		owner:      owner,
	}
}

func (c *LoggingConnector) Init(br *bridgev2.Bridge)        { c.br = br }
func (c *LoggingConnector) Start(ctx context.Context) error { c.log.Info().Msg("Start"); return nil }
func (c *LoggingConnector) PreStop()                        {}
func (c *LoggingConnector) Stop()                           { c.log.Info().Msg("Stop") }
func (c *LoggingConnector) ServerName() string              { return c.serverName }

func (c *LoggingConnector) GetCapabilities() *bridgev2.MatrixCapabilities {
	// BatchSending must be true, otherwise bridgev2 skips history backfill.
	return &bridgev2.MatrixCapabilities{AutoJoinInvites: true, BatchSending: true}
}

func (c *LoggingConnector) FormatGhostMXID(userID networkid.UserID) id.UserID {
	return id.NewUserID(id.EncodeUserLocalpart(string(userID)), c.serverName)
}

func (c *LoggingConnector) ParseGhostMXID(userID id.UserID) (networkid.UserID, bool) {
	localpart, server, err := userID.Parse()
	if err != nil || server != c.serverName {
		return "", false
	}
	decoded, err := id.DecodeUserLocalpart(localpart)
	if err != nil {
		return "", false
	}
	return networkid.UserID(decoded), true
}

func (c *LoggingConnector) GhostIntent(userID networkid.UserID) bridgev2.MatrixAPI {
	return &LoggingIntent{c: c, mxid: c.FormatGhostMXID(userID)}
}

func (c *LoggingConnector) BotIntent() bridgev2.MatrixAPI {
	return &LoggingIntent{c: c, mxid: id.NewUserID("bot", c.serverName)}
}

func (c *LoggingConnector) NewUserIntent(ctx context.Context, userID id.UserID, accessToken string) (bridgev2.MatrixAPI, string, error) {
	return &LoggingIntent{c: c, mxid: userID, owner: true}, accessToken, nil
}

func (c *LoggingConnector) SendBridgeStatus(ctx context.Context, state *status.BridgeState) error {
	c.log.Info().Str("state", string(state.StateEvent)).Str("error", string(state.Error)).Msg("SendBridgeStatus")
	return nil
}

func (c *LoggingConnector) SendMessageStatus(ctx context.Context, st *bridgev2.MessageStatus, evt *bridgev2.MessageStatusEventInfo) {
	c.log.Info().Str("event_id", evt.SourceEventID.String()).Str("txn_id", evt.TransactionID).Str("status", string(st.Status)).Msg("SendMessageStatus")
}

func (c *LoggingConnector) GenerateContentURI(ctx context.Context, mediaID networkid.MediaID) (id.ContentURIString, error) {
	return id.ContentURI{Homeserver: c.serverName, FileID: base64.RawURLEncoding.EncodeToString(mediaID)}.CUString(), nil
}

func (c *LoggingConnector) ParseContentURI(ctx context.Context, uri id.ContentURIString) (networkid.MediaID, error) {
	parsed, err := uri.Parse()
	if err != nil {
		return nil, err
	}
	return base64.RawURLEncoding.DecodeString(parsed.FileID)
}

// Nothing is stored yet, so these return "unknown"; bridgev2 handles nil safely.
func (c *LoggingConnector) GetPowerLevels(ctx context.Context, roomID id.RoomID) (*event.PowerLevelsEventContent, error) {
	return nil, nil
}

func (c *LoggingConnector) GetMembers(ctx context.Context, roomID id.RoomID) (map[id.UserID]*event.MemberEventContent, error) {
	return map[id.UserID]*event.MemberEventContent{}, nil
}

func (c *LoggingConnector) GetMemberInfo(ctx context.Context, roomID id.RoomID, userID id.UserID) (*event.MemberEventContent, error) {
	return nil, nil
}

func (c *LoggingConnector) BatchSend(ctx context.Context, roomID id.RoomID, req *mautrix.ReqBeeperBatchSend, extras []*bridgev2.MatrixSendExtra) (*mautrix.RespBeeperBatchSend, error) {
	c.log.Info().Str("room_id", roomID.String()).Int("events", len(req.Events)).Bool("forward", req.Forward).Msg("BatchSend")
	ids := make([]id.EventID, len(req.Events))
	for i, evt := range req.Events {
		c.logEvent("BatchSend event", roomID, evt.Sender, evt.Type, &evt.Content)
		ids[i] = c.randomEventID()
	}
	return &mautrix.RespBeeperBatchSend{EventIDs: ids}, nil
}

func (c *LoggingConnector) GenerateDeterministicRoomID(key networkid.PortalKey) id.RoomID {
	return id.RoomID("!" + hashID(string(key.ID), string(key.Receiver)) + ":" + c.serverName)
}

func (c *LoggingConnector) GenerateDeterministicEventID(roomID id.RoomID, key networkid.PortalKey, messageID networkid.MessageID, partID networkid.PartID) id.EventID {
	return id.EventID("$" + hashID(string(key.ID), string(key.Receiver), string(messageID), string(partID)) + ":" + c.serverName)
}

func (c *LoggingConnector) GenerateReactionEventID(roomID id.RoomID, target *database.Message, sender networkid.UserID, emojiID networkid.EmojiID) id.EventID {
	if target == nil {
		return c.randomEventID()
	}
	return id.EventID("$" + hashID("reaction", string(target.ID), string(target.PartID), string(sender), string(emojiID)) + ":" + c.serverName)
}

func (c *LoggingConnector) randomEventID() id.EventID {
	buf := make([]byte, 18)
	_, _ = rand.Read(buf)
	return id.EventID("$" + base64.RawURLEncoding.EncodeToString(buf) + ":" + c.serverName)
}

func (c *LoggingConnector) logEvent(what string, roomID id.RoomID, sender id.UserID, evtType event.Type, content *event.Content) {
	c.log.Info().
		Str("room_id", roomID.String()).
		Str("sender", sender.String()).
		Str("type", evtType.Type).
		Any("content", content.Raw).
		Any("parsed", content.Parsed).
		Msg(what)
}

// hashID turns several parts into one stable, URL-safe ID.
func hashID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return base64.RawURLEncoding.EncodeToString(sum[:18])
}

// LoggingIntent is "a user acting in rooms" (a contact, the bridge bot, or you).
type LoggingIntent struct {
	c     *LoggingConnector
	mxid  id.UserID
	owner bool
}

var _ bridgev2.MatrixAPI = (*LoggingIntent)(nil)

var errNotImplemented = errors.New("not implemented in logging connector")

func (i *LoggingIntent) GetMXID() id.UserID   { return i.mxid }
func (i *LoggingIntent) IsDoublePuppet() bool { return i.owner }

func (i *LoggingIntent) SendMessage(ctx context.Context, roomID id.RoomID, evtType event.Type, content *event.Content, extra *bridgev2.MatrixSendExtra) (*mautrix.RespSendEvent, error) {
	i.c.logEvent("SendMessage", roomID, i.mxid, evtType, content)
	return &mautrix.RespSendEvent{EventID: i.c.randomEventID()}, nil
}

func (i *LoggingIntent) SendState(ctx context.Context, roomID id.RoomID, evtType event.Type, stateKey string, content *event.Content, ts time.Time) (*mautrix.RespSendEvent, error) {
	i.c.logEvent("SendState "+stateKey, roomID, i.mxid, evtType, content)
	return &mautrix.RespSendEvent{EventID: i.c.randomEventID()}, nil
}

func (i *LoggingIntent) MarkRead(ctx context.Context, roomID id.RoomID, eventID id.EventID, ts time.Time) error {
	i.c.log.Info().Str("room_id", roomID.String()).Str("user", i.mxid.String()).Str("event_id", eventID.String()).Msg("MarkRead")
	return nil
}

func (i *LoggingIntent) MarkUnread(ctx context.Context, roomID id.RoomID, unread bool) error {
	return nil
}

func (i *LoggingIntent) MarkTyping(ctx context.Context, roomID id.RoomID, typingType bridgev2.TypingType, timeout time.Duration) error {
	i.c.log.Info().Str("room_id", roomID.String()).Str("user", i.mxid.String()).Dur("timeout", timeout).Msg("MarkTyping")
	return nil
}

func (i *LoggingIntent) DownloadMedia(ctx context.Context, uri id.ContentURIString, file *event.EncryptedFileInfo) ([]byte, error) {
	return nil, errNotImplemented
}

func (i *LoggingIntent) DownloadMediaToFile(ctx context.Context, uri id.ContentURIString, file *event.EncryptedFileInfo, writable bool, callback func(*os.File) error) error {
	return errNotImplemented
}

func (i *LoggingIntent) UploadMedia(ctx context.Context, roomID id.RoomID, data []byte, fileName, mimeType string) (id.ContentURIString, *event.EncryptedFileInfo, error) {
	i.c.log.Info().Str("room_id", roomID.String()).Str("file", fileName).Int("bytes", len(data)).Msg("UploadMedia")
	return i.fakeMediaURI(), nil, nil
}

func (i *LoggingIntent) UploadMediaStream(ctx context.Context, roomID id.RoomID, size int64, requireFile bool, cb bridgev2.FileStreamCallback) (id.ContentURIString, *event.EncryptedFileInfo, error) {
	// The callback writes the media into a file we give it; we discard it for now.
	tmp, err := os.CreateTemp("", "tuimessenger-upload-*")
	if err != nil {
		return "", nil, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	res, err := cb(tmp)
	if err != nil {
		return "", nil, bridgev2.CallbackError{Type: "write", Wrapped: err}
	}
	i.c.log.Info().Str("room_id", roomID.String()).Str("file", res.FileName).Int64("bytes", size).Msg("UploadMediaStream")
	return i.fakeMediaURI(), nil, nil
}

func (i *LoggingIntent) fakeMediaURI() id.ContentURIString {
	return id.ContentURI{Homeserver: i.c.serverName, FileID: hashID(time.Now().String())}.CUString()
}

func (i *LoggingIntent) SetDisplayName(ctx context.Context, name string) error {
	i.c.log.Info().Str("user", i.mxid.String()).Str("name", name).Msg("SetDisplayName")
	return nil
}

func (i *LoggingIntent) SetAvatarURL(ctx context.Context, avatarURL id.ContentURIString) error {
	return nil
}

func (i *LoggingIntent) SetExtraProfileMeta(ctx context.Context, data any) error { return nil }
func (i *LoggingIntent) SetProfile(ctx context.Context, data any) error          { return nil }

func (i *LoggingIntent) CreateRoom(ctx context.Context, req *mautrix.ReqCreateRoom) (id.RoomID, error) {
	roomID := req.BeeperLocalRoomID
	if roomID == "" {
		roomID = id.RoomID("!" + hashID(time.Now().String()) + ":" + i.c.serverName)
	}
	i.c.log.Info().Str("room_id", roomID.String()).Str("name", req.Name).Int("initial_state", len(req.InitialState)).Msg("CreateRoom")
	return roomID, nil
}

func (i *LoggingIntent) DeleteRoom(ctx context.Context, roomID id.RoomID, puppetsOnly bool) error {
	i.c.log.Info().Str("room_id", roomID.String()).Msg("DeleteRoom")
	return nil
}

func (i *LoggingIntent) EnsureJoined(ctx context.Context, roomID id.RoomID, params ...bridgev2.EnsureJoinedParams) error {
	return nil
}

func (i *LoggingIntent) EnsureInvited(ctx context.Context, roomID id.RoomID, userID id.UserID) error {
	return nil
}

func (i *LoggingIntent) TagRoom(ctx context.Context, roomID id.RoomID, tag event.RoomTag, isTagged bool) error {
	return nil
}

func (i *LoggingIntent) MuteRoom(ctx context.Context, roomID id.RoomID, until time.Time) error {
	return nil
}

func (i *LoggingIntent) GetEvent(ctx context.Context, roomID id.RoomID, eventID id.EventID) (*event.Event, error) {
	return nil, errNotImplemented
}

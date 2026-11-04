package matrixconn

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// Ghost IDs look like "@whatsapp_123:server" and the bot like "@whatsappbot:server";
// the tuimessenger client reads the part before "_" to pick the network icon.

func (c *Connector) FormatGhostMXID(userID networkid.UserID) id.UserID {
	return id.NewUserID(c.bridgeKey+"_"+id.EncodeUserLocalpart(string(userID)), c.serverName)
}

func (c *Connector) ParseGhostMXID(userID id.UserID) (networkid.UserID, bool) {
	localpart, server, err := userID.Parse()
	if err != nil || server != c.serverName {
		return "", false
	}
	encoded, ok := strings.CutPrefix(localpart, c.bridgeKey+"_")
	if !ok {
		return "", false
	}
	decoded, err := id.DecodeUserLocalpart(encoded)
	if err != nil {
		return "", false
	}
	return networkid.UserID(decoded), true
}

func (c *Connector) GenerateDeterministicRoomID(key networkid.PortalKey) id.RoomID {
	return id.RoomID("!" + c.hashID("room", string(key.ID), string(key.Receiver)) + ":" + c.serverName)
}

func (c *Connector) GenerateDeterministicEventID(roomID id.RoomID, key networkid.PortalKey, messageID networkid.MessageID, partID networkid.PartID) id.EventID {
	return id.EventID("$" + c.hashID("message", string(key.ID), string(key.Receiver), string(messageID), string(partID)) + ":" + c.serverName)
}

func (c *Connector) GenerateReactionEventID(roomID id.RoomID, target *database.Message, sender networkid.UserID, emojiID networkid.EmojiID) id.EventID {
	if target == nil {
		return c.randomEventID()
	}
	return id.EventID("$" + c.hashID("reaction", string(target.Room.ID), string(target.Room.Receiver),
		string(target.ID), string(target.PartID), string(sender), string(emojiID)) + ":" + c.serverName)
}

// eventIDFor gives new messages and reactions a stable ID, so a resent one is recognised as a duplicate.
func (c *Connector) eventIDFor(roomID id.RoomID, evtType event.Type, content *event.Content, extra *bridgev2.MatrixSendExtra) id.EventID {
	switch {
	case extra == nil, evtType == event.EventRedaction, isEdit(content):
		// Edits and redactions carry the metadata of the message they change, so they need their own ID.
		return c.randomEventID()
	case extra.MessageMeta != nil:
		msg := extra.MessageMeta
		return c.GenerateDeterministicEventID(roomID, msg.Room, msg.ID, msg.PartID)
	case extra.ReactionMeta != nil:
		r := extra.ReactionMeta
		target := &database.Message{Room: r.Room, ID: r.MessageID, PartID: r.MessagePartID}
		return c.GenerateReactionEventID(roomID, target, r.SenderID, r.EmojiID)
	}
	return c.randomEventID()
}

func isEdit(content *event.Content) bool {
	msg, ok := content.Parsed.(*event.MessageEventContent)
	return ok && msg.RelatesTo != nil && msg.RelatesTo.Type == event.RelReplace
}

func (c *Connector) randomEventID() id.EventID {
	return id.EventID("$" + randomString() + ":" + c.serverName)
}

func (c *Connector) randomRoomID() id.RoomID {
	return id.RoomID("!" + randomString() + ":" + c.serverName)
}

// hashID turns several parts into one stable, URL-safe ID that differs per network.
func (c *Connector) hashID(parts ...string) string {
	sum := sha256.Sum256([]byte(c.bridgeKey + "\x00" + strings.Join(parts, "\x00")))
	return base64.RawURLEncoding.EncodeToString(sum[:18])
}

func randomString() string {
	buf := make([]byte, 18)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)
}

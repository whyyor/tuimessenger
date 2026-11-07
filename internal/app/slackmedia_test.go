package app

import (
	"bytes"
	"testing"

	"go.mau.fi/mautrix-slack/pkg/slackid"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

func TestSlackLegacyFileID(t *testing.T) {
	logins := []networkid.UserLoginID{"T0000000001-U0000000009", "T0000000000-U0000000001"}
	// The old fork's layout: type 2, the workspace, the file.
	legacy := networkid.MediaID(append(append([]byte{2, 11}, "T0000000000"...), append([]byte{11}, "F0000000042"...)...))
	got := slackLegacyFileID(legacy, logins)
	parsed, err := slackid.ParseMediaID(got)
	file, ok := parsed.(*slackid.DirectMediaFile)
	if err != nil || !ok || file.UserLoginID != "T0000000000-U0000000001" || file.FileID != "F0000000042" {
		t.Fatalf("old ID should point at that workspace's login: %#v %v", parsed, err)
	}

	current := (&slackid.DirectMediaFile{UserLoginID: "T0000000000-U0000000001", FileID: "F1"}).MediaID()
	if !bytes.Equal(slackLegacyFileID(current, logins), current) {
		t.Error("a current ID must stay as it is")
	}
	if !bytes.Equal(slackLegacyFileID(legacy, nil), legacy) {
		t.Error("no matching login: unchanged, so the bridge reports it")
	}
	emoji := networkid.MediaID(append([]byte{1, 2}, "T1"...))
	if !bytes.Equal(slackLegacyFileID(emoji, logins), emoji) {
		t.Error("custom emoji IDs are left alone")
	}
}

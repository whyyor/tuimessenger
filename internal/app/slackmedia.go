package app

import (
	"context"
	"strings"

	"go.mau.fi/mautrix-slack/pkg/slackid"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

// slackLegacyFileID upgrades a Slack file ID saved by tuimessenger's old Slack fork, which stored the file's workspace
// where the upstream bridge stores the login ("WORKSPACE-USER"), so files bridged before the switch still open.
// Anything else comes back unchanged.
func slackLegacyFileID(mediaID networkid.MediaID, logins []networkid.UserLoginID) networkid.MediaID {
	parsed, err := slackid.ParseMediaID(mediaID)
	file, ok := parsed.(*slackid.DirectMediaFile)
	if err != nil || !ok || strings.Contains(string(file.UserLoginID), "-") {
		return mediaID
	}
	for _, login := range logins {
		if team, _ := slackid.ParseUserLoginID(login); team != "" && strings.EqualFold(team, string(file.UserLoginID)) {
			upgraded := &slackid.DirectMediaFile{UserLoginID: login, FileID: file.FileID}
			if id := upgraded.MediaID(); id != nil {
				return id
			}
		}
	}
	return mediaID
}

// slackLogins lists the owner's Slack logins, for slackLegacyFileID.
func (a *App) slackLogins(ctx context.Context, br *bridgev2.Bridge) []networkid.UserLoginID {
	user, err := br.GetUserByMXID(ctx, a.opts.Owner)
	if err != nil || user == nil {
		return nil
	}
	return user.GetUserLoginIDs()
}

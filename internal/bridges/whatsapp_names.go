package bridges

import (
	"context"
	"strings"
	"time"

	"github.com/rs/zerolog"
	whatsapp "go.mau.fi/mautrix-whatsapp/pkg/connector"
	"go.mau.fi/mautrix-whatsapp/pkg/waid"
	"go.mau.fi/whatsmeow/types"
	"maunium.net/go/mautrix/bridgev2"
)

// A fresh WhatsApp link knows few names: business names ("Myntra") only come from a user-info lookup, which the
// bridge does every 4 hours and only for contacts last checked over a week ago. These settings make opening a chat
// fix its name within minutes, and fixNumberNames looks up the DMs still shown as a phone number once at startup.

const (
	nameLookupBatch   = 50
	nameLookupWaitMax = 2 * time.Minute
)

func init() {
	whatsapp.ResyncLoopInterval = 10 * time.Minute
	whatsapp.ResyncJitterSeconds = 120
}

// fixNumberNames looks up WhatsApp DM contacts whose name is still their phone number.
func fixNumberNames(ctx context.Context, br *bridgev2.Bridge, owner *bridgev2.User) {
	log := br.Log.With().Str("action", "fix number names").Logger()
	for _, login := range owner.GetUserLogins() {
		wa, ok := login.Client.(*whatsapp.WhatsAppClient)
		if !ok || !waitConnected(ctx, wa) {
			continue
		}
		ghosts, jids := numberNamedDMs(ctx, br, log)
		if len(ghosts) == 0 {
			return
		}
		log.Info().Int("contacts", len(ghosts)).Msg("Looking up names for DMs shown as a phone number")
		fixed := 0
		for start := 0; start < len(jids); start += nameLookupBatch {
			end := min(start+nameLookupBatch, len(jids))
			// Asking WhatsApp about these users stores any business names it returns in the contact store.
			if _, err := wa.Client.GetUserInfo(ctx, jids[start:end]); err != nil {
				log.Warn().Err(err).Msg("Name lookup failed")
				return
			}
			for _, ghost := range ghosts[start:end] {
				// A phone number counts as "name set", so mark it unset to make the bridge work the name out again.
				ghost.NameSet = false
				info, err := wa.GetUserInfo(ctx, ghost)
				if err != nil || info == nil {
					continue
				}
				ghost.UpdateInfo(ctx, info)
				if !strings.HasPrefix(ghost.Name, "+") {
					fixed++
				}
			}
		}
		log.Info().Int("fixed", fixed).Int("still_numbers", len(ghosts)-fixed).Msg("Finished name lookup")
	}
}

func waitConnected(ctx context.Context, wa *whatsapp.WhatsAppClient) bool {
	deadline := time.Now().Add(nameLookupWaitMax)
	for time.Now().Before(deadline) {
		if wa.Client != nil && wa.Client.IsConnected() && wa.IsLoggedIn() {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(2 * time.Second):
		}
	}
	return false
}

func numberNamedDMs(ctx context.Context, br *bridgev2.Bridge, log zerolog.Logger) ([]*bridgev2.Ghost, []types.JID) {
	portals, err := br.GetAllPortalsWithMXID(ctx)
	if err != nil {
		log.Err(err).Msg("Failed to list chats")
		return nil, nil
	}
	var ghosts []*bridgev2.Ghost
	var jids []types.JID
	seen := make(map[string]bool)
	for _, portal := range portals {
		if portal.OtherUserID == "" || seen[string(portal.OtherUserID)] {
			continue
		}
		seen[string(portal.OtherUserID)] = true
		ghost, err := br.GetExistingGhostByID(ctx, portal.OtherUserID)
		if err != nil || ghost == nil || !strings.HasPrefix(ghost.Name, "+") {
			continue
		}
		ghosts = append(ghosts, ghost)
		jids = append(jids, waid.ParseUserID(ghost.ID))
	}
	return ghosts, jids
}

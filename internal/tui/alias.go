package tui

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"go.mau.fi/mauview"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/gomuks/pkg/hicli/jsoncmd"
	"go.mau.fi/gomuks/pkg/rpc/client"
	"go.mau.fi/gomuks/pkg/rpc/store"
	"go.mau.fi/gomuks/tui/debug"
	"go.mau.fi/gomuks/tui/widget"
)

// AccountDataAlias is your private nickname for a chat ("sis"); room account
// data, so it syncs and the other side never sees it.
var AccountDataAlias = event.Type{Type: "com.whyyor.gomuks.alias", Class: event.AccountDataEventType}

type chatAliasContent struct {
	Alias string `json:"alias,omitempty"`
}

func chatAlias(room *store.RoomStore) string {
	if room == nil {
		return ""
	}
	var c chatAliasContent
	if ad := room.GetAccountData(AccountDataAlias); ad != nil {
		_ = json.Unmarshal(ad.Content, &c)
	}
	return strings.TrimSpace(c.Alias)
}

func aliasOf(gmx *client.GomuksClient, roomID id.RoomID) string {
	if gmx == nil {
		return ""
	}
	return chatAlias(gmx.GetRoom(roomID))
}

// SetAlias saves this chat's alias; an empty one removes it.
func (view *RoomView) SetAlias(alias string) {
	defer debug.Recover()
	alias = strings.TrimSpace(alias)
	raw, _ := json.Marshal(&chatAliasContent{Alias: alias})
	err := view.parent.matrix.SetAccountData(context.TODO(), &jsoncmd.SetAccountDataParams{
		RoomID: view.Room.ID, Type: AccountDataAlias.Type, Content: raw,
	})
	switch {
	case err != nil:
		view.AddServiceMessage("Failed to save alias: %v", err)
	case alias == "":
		view.AddServiceMessage("Alias removed")
	default:
		view.AddServiceMessage("Alias set to %q: find this chat by it in Ctrl+k and Ctrl+p", alias)
	}
	RequestRepaint()
}

// showAlias is the alias to display, or "" when aliases are hidden.
func (view *MainView) showAlias(roomID id.RoomID) string {
	if view == nil || view.config == nil || view.config.Preferences.HideAliases {
		return ""
	}
	return aliasOf(view.matrix, roomID)
}

// aliasDim is the faint alias style on the row's own background.
func aliasDim(row tcell.Style) tcell.Style {
	_, bg, _ := row.Decompose()
	return tcell.StyleDefault.Background(bg).Foreground(ColorStatusText)
}

// writeNameAlias draws "Name · alias"; the name wins when space is short,
// and the alias is dropped under five cells.
func writeNameAlias(screen mauview.Screen, x, y, maxW int, name, alias string, style, aliasStyle tcell.Style) {
	name = runewidth.Truncate(name, maxW, "…")
	widget.WriteLine(screen, mauview.AlignLeft, name, x, y, maxW, style)
	left := maxW - runewidth.StringWidth(name)
	if alias == "" || left < 5 {
		return
	}
	text := runewidth.Truncate(" · "+alias, left, "…")
	widget.WriteLine(screen, mauview.AlignLeft, text, x+maxW-left, y, left, aliasStyle)
}

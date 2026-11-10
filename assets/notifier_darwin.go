//go:build darwin

package assets

import (
	"embed"
	"io/fs"
)

// NotifierZip is terminal-notifier.app (MIT; built by scripts/build-notifier.sh), which shows tuimessenger's
// notifications on macOS under its own name and icon.
//
//go:embed terminal-notifier.zip
var NotifierZip []byte

// networkIcons are the per-network notification icons, <App name>.icns (see THIRD_PARTY_NOTICES.md).
//
//go:embed notifiers/*.icns
var networkIcons embed.FS

// NetworkIcon is the notification icon of the network app with this name (e.g. "WhatsApp"), or nil if there is none.
func NetworkIcon(name string) []byte {
	icon, err := fs.ReadFile(networkIcons, "notifiers/"+name+".icns")
	if err != nil {
		return nil
	}
	return icon
}

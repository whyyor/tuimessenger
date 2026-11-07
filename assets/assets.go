// Package assets holds files built into the binary.
package assets

import _ "embed"

// IconICNS is the app icon for macOS bundles (the notifier app), rendered from icon.svg.
//
//go:embed icon.icns
var IconICNS []byte

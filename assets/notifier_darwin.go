//go:build darwin

package assets

import _ "embed"

// NotifierZip is terminal-notifier.app (MIT; built by scripts/build-notifier.sh), which shows tuimessenger's
// notifications on macOS under its own name and icon.
//
//go:embed terminal-notifier.zip
var NotifierZip []byte

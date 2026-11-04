package media

import (
	"encoding/base64"
	"strings"
)

// Direct media: files that stay on the network until someone opens them. Their mxc:// ID says which network to ask
// and what to ask for: "dm-<network>-<base64url of the network's media ID>", e.g. "dm-whatsapp-AbC...".

const directPrefix = "dm-"

// DirectID builds the mxc file ID for a network's media ID.
func DirectID(network string, mediaID []byte) string {
	return directPrefix + network + "-" + base64.RawURLEncoding.EncodeToString(mediaID)
}

// IsDirect says whether a file ID points to direct media rather than a stored file.
func IsDirect(fileID string) bool {
	return strings.HasPrefix(fileID, directPrefix)
}

// ParseDirectID splits a direct media file ID into the network and its media ID.
func ParseDirectID(fileID string) (network string, mediaID []byte, ok bool) {
	rest, ok := strings.CutPrefix(fileID, directPrefix)
	if !ok {
		return "", nil, false
	}
	network, encoded, ok := strings.Cut(rest, "-")
	if !ok || network == "" {
		return "", nil, false
	}
	mediaID, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(mediaID) == 0 {
		return "", nil, false
	}
	return network, mediaID, true
}

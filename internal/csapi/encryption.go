package csapi

import (
	"net/http"

	"maunium.net/go/mautrix"
)

// Bridged chats aren't end-to-end encrypted between the client and this server (both run on your own machines),
// so the encryption endpoints only give harmless answers. Clients learn this from the flag below in /versions.
const noE2EEFeature = "com.tuimessenger.no_e2ee"

// fullKeyCount tells clients they already uploaded plenty of one-time keys, so they don't keep uploading more.
var fullKeyCount = mautrix.OTKCount{SignedCurve25519: 50}

func (s *Server) encryptionRoutes() {
	s.route("POST "+clientV3+"/keys/upload", true, s.keysUpload)
	s.route("POST "+clientV3+"/keys/query", true, s.keysQuery)
	s.route("POST "+clientV3+"/keys/claim", true, s.keysClaim)
	s.route("GET "+clientV3+"/keys/changes", true, s.keysChanges)
	s.route("PUT "+clientV3+"/sendToDevice/{type}/{txnID}", true, s.sendToDevice)
	s.route("GET "+clientV3+"/room_keys/version", true, s.noKeyBackup)
}

func (s *Server) keysUpload(r *http.Request) (any, error) {
	return map[string]any{"one_time_key_counts": fullKeyCount}, nil
}

func (s *Server) keysQuery(r *http.Request) (any, error) {
	return map[string]any{"device_keys": map[string]any{}, "failures": map[string]any{}}, nil
}

func (s *Server) keysClaim(r *http.Request) (any, error) {
	return map[string]any{"one_time_keys": map[string]any{}, "failures": map[string]any{}}, nil
}

func (s *Server) keysChanges(r *http.Request) (any, error) {
	return map[string]any{"changed": []string{}, "left": []string{}}, nil
}

func (s *Server) sendToDevice(r *http.Request) (any, error) {
	return nil, nil
}

func (s *Server) noKeyBackup(r *http.Request) (any, error) {
	return nil, mautrix.MNotFound.WithMessage("No key backup")
}

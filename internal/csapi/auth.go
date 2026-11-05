package csapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

const passwordSetting = "password_bcrypt"

// SetPassword sets the password clients log in with. Only a bcrypt hash is stored.
func (s *Server) SetPassword(ctx context.Context, password string) error {
	if len(password) < 8 {
		return errors.New("the password needs at least 8 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.Store.SetSetting(ctx, passwordSetting, string(hash))
}

// HasPassword reports whether a password was set, i.e. whether clients can log in at all.
func (s *Server) HasPassword(ctx context.Context) (bool, error) {
	hash, err := s.Store.GetSetting(ctx, passwordSetting)
	return hash != "", err
}

func (s *Server) versions(r *http.Request) (any, error) {
	return &mautrix.RespVersions{
		Versions: []mautrix.SpecVersion{mautrix.SpecV11, mautrix.SpecV12, mautrix.SpecV13, mautrix.SpecV14,
			mautrix.SpecV15, mautrix.SpecV16, mautrix.SpecV17, mautrix.SpecV18, mautrix.SpecV19, mautrix.SpecV110, mautrix.SpecV111},
		UnstableFeatures: map[string]bool{noE2EEFeature: true, viewingFeature: true, reconnectFeature: true},
	}, nil
}

func (s *Server) loginFlows(r *http.Request) (any, error) {
	return &mautrix.RespLoginFlows{Flows: []mautrix.LoginFlow{{Type: mautrix.AuthTypePassword}}}, nil
}

func (s *Server) login(r *http.Request) (any, error) {
	var req mautrix.ReqLogin
	if err := readJSON(r, &req); err != nil {
		return nil, err
	}
	if req.Type != mautrix.AuthTypePassword {
		return nil, mautrix.MUnknown.WithMessage("Only password login is supported")
	}
	user := req.Identifier.User
	if user != s.Owner.String() && user != s.Owner.Localpart() {
		return nil, s.loginFailed()
	}
	hash, err := s.Store.GetSetting(r.Context(), passwordSetting)
	if err != nil {
		return nil, err
	} else if hash == "" || bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		return nil, s.loginFailed()
	}

	deviceID := req.DeviceID
	if deviceID == "" {
		deviceID = id.DeviceID("TM" + strings.ToUpper(randomString(8)))
	}
	token := randomString(32)
	if err = s.Store.AddAccessToken(r.Context(), token, deviceID); err != nil {
		return nil, err
	}
	s.Log.Info().Stringer("device_id", deviceID).Msg("Client logged in")
	return &mautrix.RespLogin{AccessToken: token, DeviceID: deviceID, UserID: s.Owner}, nil
}

// loginFailed waits a moment before answering, so guessing passwords is slow.
func (s *Server) loginFailed() error {
	time.Sleep(time.Second)
	return mautrix.MForbidden.WithMessage("Wrong username or password")
}

func (s *Server) logout(r *http.Request) (any, error) {
	return nil, s.Store.DeleteAccessToken(r.Context(), accessToken(r))
}

func (s *Server) whoami(r *http.Request) (any, error) {
	deviceID, _ := r.Context().Value(deviceKey).(id.DeviceID)
	return &mautrix.RespWhoami{UserID: s.Owner, DeviceID: deviceID}, nil
}

func (s *Server) capabilities(r *http.Request) (any, error) {
	return map[string]any{"capabilities": map[string]any{
		"m.change_password": map[string]bool{"enabled": false},
		"m.room_versions":   map[string]any{"default": "11", "available": map[string]string{"11": "stable"}},
	}}, nil
}

func randomString(n int) string {
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)[:n]
}

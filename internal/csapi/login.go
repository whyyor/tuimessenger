package csapi

import (
	"net/http"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

// Clients log in to chat networks step by step here, like mautrix's provisioning API: start a flow, then answer each
// step (form values, browser cookies, or nothing to wait for a QR scan) until the step is "complete".
const (
	loginFeature = "com.tuimessenger.login"
	loginPath    = "/_matrix/client/unstable/com.tuimessenger/login"
)

// LoginStep is a login in progress and what it needs next.
type LoginStep struct {
	LoginID string              `json:"login_id"`
	Step    *bridgev2.LoginStep `json:"step"`
}

func (s *Server) startLogin(r *http.Request) (any, error) {
	return s.Bridges.StartLogin(r.Context(), r.PathValue("network"), r.PathValue("flowID"))
}

// submitLogin takes the step's values as a JSON object of strings ({} for waiting).
func (s *Server) submitLogin(r *http.Request) (any, error) {
	input := map[string]string{}
	if r.ContentLength != 0 {
		if err := readJSON(r, &input); err != nil {
			return nil, err
		}
	}
	return s.Bridges.SubmitLogin(r.Context(), r.PathValue("loginID"), r.PathValue("stepID"),
		bridgev2.LoginStepType(r.PathValue("stepType")), input)
}

func (s *Server) cancelLogin(r *http.Request) (any, error) {
	return struct{}{}, s.Bridges.CancelLogin(r.Context(), r.PathValue("loginID"))
}

func (s *Server) networkLogout(r *http.Request) (any, error) {
	network, loginID := r.PathValue("network"), networkid.UserLoginID(r.PathValue("loginID"))
	if err := s.Bridges.Logout(r.Context(), network, loginID); err != nil {
		return nil, err
	}
	s.Log.Info().Str("network", network).Msg("Client logged a network out")
	return struct{}{}, nil
}

// setTelegramKeys takes {"api_id": 123, "api_hash": "…"} from https://my.telegram.org.
func (s *Server) setTelegramKeys(r *http.Request) (any, error) {
	var req struct {
		APIID   int    `json:"api_id"`
		APIHash string `json:"api_hash"`
	}
	if err := readJSON(r, &req); err != nil {
		return nil, err
	}
	if req.APIID <= 0 || len(req.APIHash) != 32 {
		return nil, mautrix.MInvalidParam.WithMessage("api_id is a number and api_hash is 32 characters, both from my.telegram.org")
	}
	if err := s.Bridges.SetTelegramKeys(r.Context(), req.APIID, req.APIHash); err != nil {
		return nil, err
	}
	s.Log.Info().Msg("Client saved Telegram app keys")
	return struct{}{}, nil
}

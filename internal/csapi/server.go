// Package csapi is a small Matrix client-server API over our store, enough for the tuimessenger client to log in, sync and chat.
package csapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/media"
	"github.com/whyyor/tuimessenger/internal/store"
	"github.com/whyyor/tuimessenger/internal/typing"
)

// Bridges hands what you do in the client to the network each room belongs to.
type Bridges interface {
	// Queue sends an event from the owner to the room's network and returns once the bridge has taken it.
	Queue(ctx context.Context, evt *event.Event) error
	// LoadOlder fetches one more page of a room's history from its network; false means nothing more is there.
	LoadOlder(ctx context.Context, roomID id.RoomID) (bool, error)
	// SetViewing tells the networks which chat is open on screen; "" means none.
	SetViewing(ctx context.Context, roomID id.RoomID)
	// FetchMedia opens direct media (a file still on its network), downloading it the first time. It also
	// returns the content type, which may be "" if the network didn't say.
	FetchMedia(ctx context.Context, fileID string, params map[string]string) (*os.File, string, error)
}

type Options struct {
	Log        zerolog.Logger
	Store      *store.Store
	Media      *media.Store
	Typing     *typing.Tracker
	Bridges    Bridges
	ServerName string
	Owner      id.UserID
}

type Server struct {
	Options
	mux *http.ServeMux

	filterLock sync.Mutex
	filters    []int // timeline limit per uploaded filter; the index is the filter ID

	viewingLock  sync.Mutex
	viewingRoom  id.RoomID
	viewingTimer *time.Timer
}

const (
	clientV3 = "/_matrix/client/v3"
	clientV1 = "/_matrix/client/v1"
	mediaV3  = "/_matrix/media/v3"
)

func New(opts Options) *Server {
	s := &Server{Options: opts, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.route("GET /_matrix/client/versions", false, s.versions)
	s.route("GET "+clientV3+"/login", false, s.loginFlows)
	s.route("POST "+clientV3+"/login", false, s.login)
	s.route("POST "+clientV3+"/logout", true, s.logout)
	s.route("GET "+clientV3+"/account/whoami", true, s.whoami)
	s.route("GET "+clientV3+"/capabilities", true, s.capabilities)

	s.route("GET "+clientV3+"/sync", true, s.sync)
	s.route("POST "+clientV3+"/user/{userID}/filter", true, s.uploadFilter)
	s.route("GET "+clientV3+"/joined_rooms", true, s.joinedRooms)

	s.route("GET "+clientV3+"/user/{userID}/account_data/{type}", true, s.getAccountData)
	s.route("PUT "+clientV3+"/user/{userID}/account_data/{type}", true, s.putAccountData)
	s.route("GET "+clientV3+"/user/{userID}/rooms/{roomID}/account_data/{type}", true, s.getAccountData)
	s.route("PUT "+clientV3+"/user/{userID}/rooms/{roomID}/account_data/{type}", true, s.putAccountData)
	s.route("GET "+clientV3+"/profile/{userID}", true, s.getProfile)
	s.route("GET "+clientV3+"/profile/{userID}/{field}", true, s.getProfile)
	s.route("PUT "+clientV3+"/profile/{userID}/{field}", true, s.putProfile)

	s.route("GET "+clientV3+"/pushrules/", true, s.getPushRules)
	s.route("GET "+clientV3+"/pushrules/global/", true, s.getPushRules)
	s.route("PUT "+clientV3+"/pushrules/global/{kind}/{ruleID}", true, s.putPushRule)
	s.route("DELETE "+clientV3+"/pushrules/global/{kind}/{ruleID}", true, s.deletePushRule)
	s.route("PUT "+clientV3+"/pushrules/global/{kind}/{ruleID}/enabled", true, s.putPushRuleEnabled)
	s.route("PUT "+clientV3+"/pushrules/global/{kind}/{ruleID}/actions", true, s.putPushRuleActions)

	s.route("PUT "+clientV3+"/rooms/{roomID}/send/{type}/{txnID}", true, s.send)
	s.route("PUT "+clientV3+"/rooms/{roomID}/state/{type}", true, s.putState)
	s.route("PUT "+clientV3+"/rooms/{roomID}/state/{type}/{stateKey...}", true, s.putState)
	s.route("PUT "+clientV3+"/rooms/{roomID}/redact/{eventID}/{txnID}", true, s.redact)
	s.route("GET "+clientV3+"/rooms/{roomID}/messages", true, s.messages)
	s.route("GET "+clientV3+"/rooms/{roomID}/event/{eventID}", true, s.getEvent)
	s.route("GET "+clientV3+"/rooms/{roomID}/state", true, s.getRoomState)
	s.route("GET "+clientV3+"/rooms/{roomID}/state/{type}", true, s.getStateEvent)
	s.route("GET "+clientV3+"/rooms/{roomID}/state/{type}/{stateKey...}", true, s.getStateEvent)
	s.route("GET "+clientV3+"/rooms/{roomID}/members", true, s.members)
	s.route("GET "+clientV3+"/rooms/{roomID}/joined_members", true, s.joinedMembers)
	s.route("POST "+clientV3+"/rooms/{roomID}/receipt/{type}/{eventID}", true, s.receipt)
	s.route("POST "+clientV3+"/rooms/{roomID}/read_markers", true, s.readMarkers)
	s.route("PUT "+clientV3+"/rooms/{roomID}/typing/{userID}", true, s.typingNotice)
	s.route("POST "+clientV3+"/rooms/{roomID}/leave", true, s.leave)
	s.route("POST "+clientV3+"/rooms/{roomID}/join", true, s.join)
	s.route("POST "+clientV3+"/join/{roomID}", true, s.join)

	s.route("GET "+clientV1+"/media/config", true, s.mediaConfig)
	s.route("GET "+mediaV3+"/config", true, s.mediaConfig)
	s.route("POST "+mediaV3+"/upload", true, s.upload)
	s.mux.HandleFunc("GET "+clientV1+"/media/download/{server}/{mediaID}", s.authed(s.download))
	s.mux.HandleFunc("GET "+clientV1+"/media/download/{server}/{mediaID}/{fileName}", s.authed(s.download))
	s.mux.HandleFunc("GET "+clientV1+"/media/thumbnail/{server}/{mediaID}", s.authed(s.download))
	s.mux.HandleFunc("GET "+mediaV3+"/download/{server}/{mediaID}", s.authed(s.download))
	s.mux.HandleFunc("GET "+mediaV3+"/download/{server}/{mediaID}/{fileName}", s.authed(s.download))
	s.mux.HandleFunc("GET "+mediaV3+"/thumbnail/{server}/{mediaID}", s.authed(s.download))

	s.route("PUT "+viewingPath, true, s.viewing)
	s.encryptionRoutes()

	// Everything else (search, room directory, ...) isn't supported (yet).
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, mautrix.MUnrecognized.WithMessage("%s %s is not supported", r.Method, r.URL.Path))
	})
}

type handler func(r *http.Request) (any, error)

// route registers a JSON endpoint: fn's result is sent as JSON, and its error as a Matrix error.
func (s *Server) route(pattern string, auth bool, fn handler) {
	h := func(w http.ResponseWriter, r *http.Request) {
		resp, err := fn(r)
		if err != nil {
			s.writeErr(w, r, err)
			return
		}
		if resp == nil {
			resp = struct{}{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
	if auth {
		h = s.authed(h)
	}
	s.mux.HandleFunc(pattern, h)
}

type ctxKey int

const deviceKey ctxKey = iota

// authed rejects requests without a valid access token, and remembers the token's device for the handler.
func (s *Server) authed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := accessToken(r)
		if token == "" {
			writeError(w, mautrix.MMissingToken.WithMessage("Missing access token"))
			return
		}
		deviceID, err := s.Store.CheckAccessToken(r.Context(), token)
		if err != nil {
			s.writeErr(w, r, err)
			return
		} else if deviceID == "" {
			writeError(w, mautrix.MUnknownToken.WithMessage("Unknown access token"))
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), deviceKey, deviceID)))
	}
}

func accessToken(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:]
	}
	return r.URL.Query().Get("access_token")
}

func (s *Server) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	var respErr mautrix.RespError
	if errors.As(err, &respErr) {
		writeError(w, respErr)
		return
	}
	s.Log.Err(err).Str("method", r.Method).Str("path", r.URL.Path).Msg("Request failed")
	writeError(w, mautrix.MUnknown.WithMessage("Internal error"))
}

func writeError(w http.ResponseWriter, err mautrix.RespError) {
	status := err.StatusCode
	if status == 0 {
		status = http.StatusBadRequest
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// MarshalJSON is defined on *RespError, so encode a pointer to get {"errcode":...,"error":...}.
	_ = json.NewEncoder(w).Encode(&err)
}

// readJSON decodes a request body of at most 1 MiB.
func readJSON(r *http.Request, into any) error {
	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	return parseJSON(data, into)
}

func parseJSON(data []byte, into any) error {
	if err := json.Unmarshal(data, into); err != nil {
		return mautrix.MNotJSON.WithMessage("Request body is not valid JSON")
	}
	return nil
}

// checkOwnUser rejects requests about another user's private data, e.g. their account data.
func (s *Server) checkOwnUser(r *http.Request) error {
	if id.UserID(r.PathValue("userID")) != s.Owner {
		return mautrix.MForbidden.WithMessage("You can only access your own data")
	}
	return nil
}

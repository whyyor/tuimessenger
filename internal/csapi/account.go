package csapi

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"net/http"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
	"maunium.net/go/mautrix/pushrules"

	"github.com/whyyor/tuimessenger/internal/store"
)

// Settings that bridges act on: marking unread, tags (favourites, archive) and muting reach the network too.
var bridgedAccountData = map[string]bool{
	event.AccountDataMarkedUnread.Type: true,
	event.AccountDataRoomTags.Type:     true,
	event.AccountDataBeeperMute.Type:   true,
}

func (s *Server) getAccountData(r *http.Request) (any, error) {
	if err := s.checkOwnUser(r); err != nil {
		return nil, err
	}
	evtType := event.Type{Type: r.PathValue("type"), Class: event.AccountDataEventType}
	raw, err := s.Store.GetAccountData(r.Context(), id.RoomID(r.PathValue("roomID")), evtType)
	if err != nil {
		return nil, err
	} else if raw == nil {
		return nil, mautrix.MNotFound.WithMessage("Account data not found")
	}
	return raw, nil
}

func (s *Server) putAccountData(r *http.Request) (any, error) {
	if err := s.checkOwnUser(r); err != nil {
		return nil, err
	}
	body, err := readBody(r)
	if err != nil {
		return nil, err
	}
	var content json.RawMessage
	if err = parseJSON(body, &content); err != nil {
		return nil, err
	}
	roomID := id.RoomID(r.PathValue("roomID"))
	evtType := event.Type{Type: r.PathValue("type"), Class: event.AccountDataEventType}
	err = s.Store.Write(r.Context(), func(ctx context.Context, w *store.Writer) error {
		return w.SetAccountData(ctx, roomID, evtType, content)
	})
	if err != nil {
		return nil, err
	}
	if roomID != "" && bridgedAccountData[evtType.Type] {
		evt := &event.Event{Type: evtType, RoomID: roomID, Sender: s.Owner, Content: event.Content{VeryRaw: body}}
		go s.forward(evt)
	}
	return nil, nil
}

func (s *Server) getProfile(r *http.Request) (any, error) {
	userID := id.UserID(r.PathValue("userID"))
	profile, err := s.Store.GetProfile(r.Context(), userID)
	if err != nil {
		return nil, err
	}
	resp := map[string]any{}
	if profile.Displayname != "" {
		resp["displayname"] = profile.Displayname
	}
	if profile.AvatarURL != "" {
		resp["avatar_url"] = profile.AvatarURL
	}
	if field := r.PathValue("field"); field != "" {
		value, ok := resp[field]
		if !ok {
			return nil, mautrix.MNotFound.WithMessage("Profile field not set")
		}
		return map[string]any{field: value}, nil
	}
	return resp, nil
}

// putProfile only changes how you appear here, not on the networks.
func (s *Server) putProfile(r *http.Request) (any, error) {
	if err := s.checkOwnUser(r); err != nil {
		return nil, err
	}
	var req map[string]string
	if err := readJSON(r, &req); err != nil {
		return nil, err
	}
	field := r.PathValue("field")
	return nil, s.Store.Write(r.Context(), func(ctx context.Context, w *store.Writer) error {
		profile, err := s.Store.GetProfile(ctx, s.Owner)
		if err != nil {
			return err
		}
		switch field {
		case "displayname":
			profile.Displayname = req[field]
		case "avatar_url":
			profile.AvatarURL = id.ContentURIString(req[field])
		default:
			return mautrix.MInvalidParam.WithMessage("Unsupported profile field %s", field)
		}
		_, err = w.SetProfile(ctx, s.Owner, profile)
		return err
	})
}

//go:embed default_pushrules.json
var defaultPushRules []byte

// ruleset is the push rules exactly as they look in JSON: five lists, highest priority first.
type ruleset struct {
	Override  pushrules.PushRuleArray `json:"override"`
	Content   pushrules.PushRuleArray `json:"content"`
	Room      pushrules.PushRuleArray `json:"room"`
	Sender    pushrules.PushRuleArray `json:"sender"`
	Underride pushrules.PushRuleArray `json:"underride"`
}

// Push rules decide which messages notify. the tuimessenger client evaluates them itself; we only store them (as m.push_rules).
func (s *Server) loadPushRules(ctx context.Context) (*ruleset, error) {
	raw, err := s.Store.GetAccountData(ctx, "", event.AccountDataPushRules)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		raw = bytes.ReplaceAll(defaultPushRules, []byte("@OWNER"), []byte(s.Owner))
	}
	var wrapper struct {
		Global *ruleset `json:"global"`
	}
	if err = json.Unmarshal(raw, &wrapper); err != nil || wrapper.Global == nil {
		return nil, errors.New("stored push rules are invalid")
	}
	return wrapper.Global, nil
}

func (s *Server) savePushRules(ctx context.Context, rules *ruleset) error {
	return s.Store.Write(ctx, func(ctx context.Context, w *store.Writer) error {
		return w.SetAccountData(ctx, "", event.AccountDataPushRules, map[string]any{"global": rules})
	})
}

func (s *Server) getPushRules(r *http.Request) (any, error) {
	rules, err := s.loadPushRules(r.Context())
	if err != nil {
		return nil, err
	}
	if r.URL.Path == clientV3+"/pushrules/global/" {
		return rules, nil
	}
	return map[string]any{"global": rules}, nil
}

// changePushRule loads the rules, lets change edit the list of one kind, and saves the result.
func (s *Server) changePushRule(r *http.Request, change func(rules *pushrules.PushRuleArray, ruleID string) error) (any, error) {
	ctx := r.Context()
	rules, err := s.loadPushRules(ctx)
	if err != nil {
		return nil, err
	}
	var list *pushrules.PushRuleArray
	switch r.PathValue("kind") {
	case "override":
		list = &rules.Override
	case "content":
		list = &rules.Content
	case "room":
		list = &rules.Room
	case "sender":
		list = &rules.Sender
	case "underride":
		list = &rules.Underride
	default:
		return nil, mautrix.MInvalidParam.WithMessage("Unknown push rule kind")
	}
	if err = change(list, r.PathValue("ruleID")); err != nil {
		return nil, err
	}
	return nil, s.savePushRules(ctx, rules)
}

func findRule(list pushrules.PushRuleArray, ruleID string) int {
	for i, rule := range list {
		if rule.RuleID == ruleID {
			return i
		}
	}
	return -1
}

func (s *Server) putPushRule(r *http.Request) (any, error) {
	var rule pushrules.PushRule
	if err := readJSON(r, &rule); err != nil {
		return nil, err
	}
	return s.changePushRule(r, func(list *pushrules.PushRuleArray, ruleID string) error {
		rule.RuleID, rule.Enabled, rule.Default = ruleID, true, false
		if i := findRule(*list, ruleID); i >= 0 {
			(*list)[i] = &rule
		} else {
			// New rules come first, so they win over the defaults.
			*list = append(pushrules.PushRuleArray{&rule}, *list...)
		}
		return nil
	})
}

func (s *Server) deletePushRule(r *http.Request) (any, error) {
	return s.changePushRule(r, func(list *pushrules.PushRuleArray, ruleID string) error {
		i := findRule(*list, ruleID)
		if i < 0 {
			return mautrix.MNotFound.WithMessage("Push rule not found")
		}
		*list = append((*list)[:i], (*list)[i+1:]...)
		return nil
	})
}

func (s *Server) putPushRuleEnabled(r *http.Request) (any, error) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := readJSON(r, &req); err != nil {
		return nil, err
	}
	return s.changePushRule(r, func(list *pushrules.PushRuleArray, ruleID string) error {
		i := findRule(*list, ruleID)
		if i < 0 {
			return mautrix.MNotFound.WithMessage("Push rule not found")
		}
		(*list)[i].Enabled = req.Enabled
		return nil
	})
}

func (s *Server) putPushRuleActions(r *http.Request) (any, error) {
	var req struct {
		Actions pushrules.PushActionArray `json:"actions"`
	}
	if err := readJSON(r, &req); err != nil {
		return nil, err
	}
	return s.changePushRule(r, func(list *pushrules.PushRuleArray, ruleID string) error {
		i := findRule(*list, ruleID)
		if i < 0 {
			return mautrix.MNotFound.WithMessage("Push rule not found")
		}
		(*list)[i].Actions = req.Actions
		return nil
	})
}

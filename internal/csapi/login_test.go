package csapi_test

import (
	"context"
	"errors"
	"testing"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
)

func TestNetworkLoginAPI(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	unstable := mautrix.BaseURLPath{"_matrix", "client", "unstable", "com.tuimessenger"}
	var resp struct {
		LoginID string              `json:"login_id"`
		Step    *bridgev2.LoginStep `json:"step"`
	}
	call := func(method string, path []any, body, out any) error {
		t.Helper()
		_, err := e.cli.MakeRequest(ctx, method, e.cli.BuildURL(append(append(mautrix.BaseURLPath{}, unstable...), path...)), body, out)
		return err
	}

	if err := call("POST", []any{"networks", "slack", "login", "email"}, struct{}{}, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.LoginID != "L1" || resp.Step.Type != bridgev2.LoginStepTypeUserInput || len(resp.Step.UserInputParams.Fields) != 1 {
		t.Fatalf("start: %+v %+v", resp, resp.Step)
	}
	if err := call("POST", []any{"login", "L1", "step", "pw", "user_input"}, map[string]string{"password": "hunter2"}, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Step.Type != bridgev2.LoginStepTypeComplete || resp.Step.CompleteParams.UserLoginID != "T1-U1" {
		t.Errorf("submit: %+v", resp.Step)
	}
	if e.bridges.loginInput["password"] != "hunter2" {
		t.Errorf("the form values must reach the network: %v", e.bridges.loginInput)
	}
	if err := call("POST", []any{"networks", "nope", "login", "x"}, struct{}{}, &resp); !errors.Is(err, mautrix.MNotFound) {
		t.Errorf("unknown network: %v", err)
	}
	if err := call("POST", []any{"login", "L1", "cancel"}, struct{}{}, nil); err != nil || len(e.bridges.cancelled) != 1 {
		t.Errorf("cancel: %v %v", err, e.bridges.cancelled)
	}
	if err := call("POST", []any{"networks", "slack", "logout", "T1-U1"}, struct{}{}, nil); err != nil || len(e.bridges.loggedOut) != 1 || e.bridges.loggedOut[0] != "slack/T1-U1" {
		t.Errorf("logout: %v %v", err, e.bridges.loggedOut)
	}

	// Telegram keys are checked before they're saved.
	if err := call("PUT", []any{"networks", "telegram", "keys"}, map[string]any{"api_id": 0, "api_hash": "short"}, nil); !errors.Is(err, mautrix.MInvalidParam) {
		t.Errorf("bad keys: %v", err)
	}
	if err := call("PUT", []any{"networks", "telegram", "keys"}, map[string]any{"api_id": 123, "api_hash": "0123456789abcdef0123456789abcdef"}, nil); err != nil || e.bridges.telegramKeys != "123:0123456789abcdef0123456789abcdef" {
		t.Errorf("keys: %v %q", err, e.bridges.telegramKeys)
	}
}

package rpc

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/whyyor/tuimessenger/internal/hicli/jsoncmd"
)

func TestTransport(t *testing.T) {
	var got *jsoncmd.Container[json.RawMessage]
	gr := NewWithTransport(func(_ context.Context, cmd *jsoncmd.Container[json.RawMessage]) *jsoncmd.Container[json.RawMessage] {
		got = cmd
		if cmd.Command != jsoncmd.ReqGetState {
			return &jsoncmd.Container[json.RawMessage]{Command: jsoncmd.RespError, Data: json.RawMessage(`"nope"`)}
		}
		return &jsoncmd.Container[json.RawMessage]{Command: jsoncmd.RespSuccess, Data: json.RawMessage(`{"is_logged_in":true,"user_id":"@me:x"}`)}
	})

	state, err := gr.GetState(context.Background())
	if err != nil {
		t.Fatal(err)
	} else if !state.IsLoggedIn || state.UserID != "@me:x" {
		t.Errorf("response wasn't decoded: %+v", state)
	} else if got.Command != jsoncmd.ReqGetState || got.RequestID == 0 {
		t.Errorf("command didn't reach the transport: %+v", got)
	}

	if err = gr.Logout(context.Background()); err == nil || err.Error() != "nope" {
		t.Errorf("want the transport's error, got %v", err)
	}
}

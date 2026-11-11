package agent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/upload"
)

// DeviceID is the agent's login in the backend, separate from the TUI's so it can be revoked on its own.
const DeviceID = id.DeviceID("TMAGENT")

// ErrNotLoggedIn means `tuimessenger agent login` hasn't been run.
var ErrNotLoggedIn = errors.New("the agent isn't logged in: run `tuimessenger agent login` once")

// ErrBackendDown means nothing answered at the backend's address.
var ErrBackendDown = errors.New("the tuimessenger backend isn't running: open tuimessenger (or start its service) first")

// TokenPath is where the agent's access token is kept (readable only by you).
func TokenPath(dataDir string) string { return filepath.Join(dataDir, "agent-token") }

// Backend talks to the tuimessenger backend the TUI is logged in to.
type Backend struct {
	cli     *mautrix.Client
	dataDir string
}

// NewBackend uses the backend address and user from the TUI's login, and the agent token if there is one.
func NewBackend(r *Reader, dataDir string) (*Backend, error) {
	cli, err := mautrix.NewClient(r.Account.HomeserverURL, r.Account.UserID, "")
	if err != nil {
		return nil, err
	}
	cli.DefaultHTTPRetries = 0
	if token, err := os.ReadFile(TokenPath(dataDir)); err == nil {
		cli.AccessToken = strings.TrimSpace(string(token))
		cli.DeviceID = DeviceID
	}
	return &Backend{cli: cli, dataDir: dataDir}, nil
}

func (b *Backend) LoggedIn() bool { return b.cli.AccessToken != "" }

// Login makes the agent's own login with the backend password and saves its token.
func (b *Backend) Login(ctx context.Context, password string) error {
	resp, err := b.cli.Login(ctx, &mautrix.ReqLogin{
		Type:                     mautrix.AuthTypePassword,
		Identifier:               mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: b.cli.UserID.Localpart()},
		Password:                 password,
		DeviceID:                 DeviceID,
		InitialDeviceDisplayName: "tuimessenger agent",
	})
	if errors.Is(err, mautrix.MForbidden) {
		return errors.New("wrong password: use the one the TUI logs in with")
	} else if err != nil {
		return friendly(err)
	}
	return os.WriteFile(TokenPath(b.dataDir), []byte(resp.AccessToken+"\n"), 0o600)
}

// Logout revokes the agent's login in the backend and deletes the token file.
func (b *Backend) Logout(ctx context.Context) error {
	if !b.LoggedIn() {
		return ErrNotLoggedIn
	}
	if _, err := b.cli.Logout(ctx); err != nil && !errors.Is(err, mautrix.MUnknownToken) {
		return friendly(err)
	}
	return os.Remove(TokenPath(b.dataDir))
}

// SendResult is what happened to a sent message.
type SendResult struct {
	ID     id.EventID `json:"id"`
	Status string     `json:"status"` // delivered, failed, sent (no answer from the network yet)
	Error  string     `json:"error,omitempty"`
}

// SendText sends a text message, then waits up to wait for the network to confirm it.
func (b *Backend) SendText(ctx context.Context, roomID id.RoomID, text string, wait time.Duration) (*SendResult, error) {
	if !b.LoggedIn() {
		return nil, ErrNotLoggedIn
	}
	return b.send(ctx, roomID, &event.MessageEventContent{MsgType: event.MsgText, Body: text}, wait)
}

// SendFile uploads a file read with upload.Read and sends it (photo, video, audio or file, from its bytes) with an
// optional caption, then waits up to wait for the network to confirm it.
func (b *Backend) SendFile(ctx context.Context, roomID id.RoomID, name string, raw []byte, caption string, wait time.Duration) (*SendResult, error) {
	if !b.LoggedIn() {
		return nil, ErrNotLoggedIn
	}
	content := upload.Content(name, raw)
	resp, err := b.cli.UploadMedia(ctx, mautrix.ReqUploadMedia{ContentBytes: raw, ContentType: content.Info.MimeType, FileName: name})
	if err != nil {
		return nil, friendly(err)
	}
	content.URL = resp.ContentURI.CUString()
	if caption != "" {
		// A body that differs from the file name is the caption (the bridges send it with the file).
		content.Body = caption
	}
	return b.send(ctx, roomID, content, wait)
}

// send sends a message, then waits up to wait for the network's delivery report.
func (b *Backend) send(ctx context.Context, roomID id.RoomID, content *event.MessageEventContent, wait time.Duration) (*SendResult, error) {
	txnID := "agent-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	resp, err := b.cli.SendMessageEvent(ctx, roomID, event.EventMessage, content, mautrix.ReqSendEvent{TransactionID: txnID})
	if err != nil {
		return nil, friendly(err)
	}
	result := &SendResult{ID: resp.EventID, Status: "sent"}
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return result, nil
		case <-time.After(500 * time.Millisecond):
		}
		status, err := b.status(ctx, roomID, resp.EventID)
		if err != nil || status == nil {
			continue
		}
		if status.Status == event.MessageStatusSuccess {
			result.Status = "delivered"
		} else {
			result.Status, result.Error = "failed", strings.TrimSpace(status.Message+" "+string(status.Reason))
		}
		return result, nil
	}
	return result, nil
}

// status finds the network's delivery report for a sent message among the chat's newest events.
func (b *Backend) status(ctx context.Context, roomID id.RoomID, eventID id.EventID) (*event.BeeperMessageStatusEventContent, error) {
	resp, err := b.cli.Messages(ctx, roomID, "", "", mautrix.DirectionBackward, nil, 50)
	if err != nil {
		return nil, err
	}
	for _, evt := range resp.Chunk {
		if evt.Type != event.BeeperMessageStatus {
			continue
		}
		if err := evt.Content.ParseRaw(evt.Type); err != nil {
			continue
		}
		if st, ok := evt.Content.Parsed.(*event.BeeperMessageStatusEventContent); ok && st.RelatesTo.EventID == eventID && st.Status != event.MessageStatusPending {
			return st, nil
		}
	}
	return nil, nil
}

// friendly turns connection and auth errors into what to do about them.
func friendly(err error) error {
	var netErr *net.OpError
	switch {
	case errors.As(err, &netErr):
		return ErrBackendDown
	case errors.Is(err, mautrix.MUnknownToken):
		return fmt.Errorf("the agent's login was revoked: run `tuimessenger agent login` again")
	case errors.Is(err, mautrix.MForbidden):
		return fmt.Errorf("the backend refused: %w", err)
	}
	return err
}

// Running reports whether the backend answers.
func (b *Backend) Running(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err := b.cli.Versions(ctx)
	return err == nil
}

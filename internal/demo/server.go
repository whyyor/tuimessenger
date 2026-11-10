package demo

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/csapi"
	"github.com/whyyor/tuimessenger/internal/matrixconn"
	"github.com/whyyor/tuimessenger/internal/media"
	"github.com/whyyor/tuimessenger/internal/store"
	"github.com/whyyor/tuimessenger/internal/typing"
)

const (
	serverName = "tuimessenger.localhost"
	owner      = id.UserID("@me:" + serverName)
)

// Password is what the demo TUI logs in with.
const Password = "demo-password"

var networks = []string{"whatsapp", "telegram", "slack", "instagram"}

var networkNames = map[string]string{"whatsapp": "WhatsApp", "telegram": "Telegram", "slack": "Slack", "instagram": "Instagram"}

// accounts are the made-up logged-in accounts the Networks panel shows.
var accounts = map[string]string{
	"whatsapp": "+1 555 010 0001", "telegram": "demo", "slack": "Acme - you@example.com", "instagram": "you.demo",
}

type Options struct {
	Log      zerolog.Logger
	DataDir  string
	MediaDir string
	Listen   string
}

// Serve seeds the demo data and runs the client API until ctx ends. POST /_demo/say {"chat":"maya","text":"hi"}
// makes a contact write, e.g. to show a notification on cue.
func Serve(ctx context.Context, opts Options) error {
	if err := os.MkdirAll(opts.DataDir, 0o700); err != nil {
		return err
	}
	st, err := store.Open(ctx, filepath.Join(opts.DataDir, "store.db"), opts.Log)
	if err != nil {
		return err
	}
	defer st.Close()
	med, err := media.New(filepath.Join(opts.DataDir, "media"))
	if err != nil {
		return err
	}
	typ := typing.New()
	conns := make(map[string]*matrixconn.Connector, len(networks))
	for _, name := range networks {
		conns[name] = matrixconn.New(opts.Log, st, med, typ, name, serverName, owner)
	}
	rooms, err := seed(ctx, conns, owner, opts.MediaDir)
	if err != nil {
		return err
	}
	fake := &fakeNetworks{rooms: rooms, typing: typ, log: opts.Log}
	api := csapi.New(csapi.Options{Log: opts.Log, Store: st, Media: med, Typing: typ, Bridges: fake, ServerName: serverName, Owner: owner})
	if err = api.SetPassword(ctx, Password); err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("/", api)
	mux.HandleFunc("POST /_demo/say", fake.say)
	listener, err := net.Listen("tcp", opts.Listen)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		_ = server.Close()
	}()
	opts.Log.Info().Msgf("Demo backend on http://%s: log in as me / %s", listener.Addr(), Password)
	if err = server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// fakeNetworks plays the networks: what you send is delivered and read, and the other side sometimes answers.
type fakeNetworks struct {
	mu       sync.Mutex
	rooms    map[id.RoomID]*room
	typing   *typing.Tracker
	log      zerolog.Logger
	loggedIn map[string]bool
}

var _ csapi.Bridges = (*fakeNetworks)(nil)

func (f *fakeNetworks) Queue(ctx context.Context, evt *event.Event) error {
	f.mu.Lock()
	r := f.rooms[evt.RoomID]
	f.mu.Unlock()
	if r == nil {
		return errors.New("not a demo chat")
	}
	go f.deliver(r, evt)
	return nil
}

// deliver acts like a network: delivered, then read by the other side, then (for messages) a typed answer.
func (f *fakeNetworks) deliver(r *room, evt *event.Event) {
	ctx := f.log.WithContext(context.Background())
	time.Sleep(400 * time.Millisecond)
	r.conn.SendMessageStatus(ctx, &bridgev2.MessageStatus{Status: event.MessageStatusSuccess, DeliveredTo: []id.UserID{r.ghosts[0].GetMXID()}},
		&bridgev2.MessageStatusEventInfo{RoomID: r.id, SourceEventID: evt.ID, EventType: evt.Type, Sender: owner})
	if evt.Type != event.EventMessage {
		return
	}
	time.Sleep(1200 * time.Millisecond)
	for _, g := range r.ghosts {
		_ = g.MarkRead(ctx, r.id, evt.ID, time.Now())
	}
	f.mu.Lock()
	if r.next >= len(r.replies) {
		f.mu.Unlock()
		return
	}
	answer := r.replies[r.next]
	r.next++
	f.mu.Unlock()
	f.write(ctx, r, 0, answer)
}

// write makes a contact type for a moment and send text.
func (f *fakeNetworks) write(ctx context.Context, r *room, who int, text string) {
	ghost := r.ghosts[who]
	_ = ghost.MarkTyping(ctx, r.id, bridgev2.TypingTypeText, 3*time.Second)
	time.Sleep(1500 * time.Millisecond)
	_ = ghost.MarkTyping(ctx, r.id, bridgev2.TypingTypeText, 0)
	content := &event.Content{Parsed: &event.MessageEventContent{MsgType: event.MsgText, Body: text}}
	if _, err := ghost.SendMessage(ctx, r.id, event.EventMessage, content, nil); err != nil {
		f.log.Err(err).Msg("Demo message failed")
	}
}

// say is POST /_demo/say {"chat": "maya", "text": "…", "from": 0}.
func (f *fakeNetworks) say(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Chat string `json:"chat"`
		Text string `json:"text"`
		From int    `json:"from"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	var target *room
	for _, r := range f.rooms {
		if r.key == body.Chat {
			target = r
		}
	}
	f.mu.Unlock()
	if target == nil || body.From < 0 || body.From >= len(target.ghosts) {
		http.Error(w, "no such chat or person", http.StatusNotFound)
		return
	}
	go f.write(f.log.WithContext(context.Background()), target, body.From, body.Text)
	w.WriteHeader(http.StatusNoContent)
}

func (f *fakeNetworks) LoadOlder(context.Context, id.RoomID) (bool, error) { return false, nil }
func (f *fakeNetworks) SetViewing(context.Context, id.RoomID)              {}
func (f *fakeNetworks) ReconnectUnhealthy(context.Context) []string        { return nil }
func (f *fakeNetworks) FetchMedia(context.Context, string, map[string]string) (*os.File, string, error) {
	return nil, "", mautrix.MNotFound.WithMessage("Demo media is stored locally")
}
func (f *fakeNetworks) SetNetworkSetting(context.Context, string, string, bool) error { return nil }
func (f *fakeNetworks) SetTelegramKeys(context.Context, int, string) error            { return nil }
func (f *fakeNetworks) CancelLogin(context.Context, string) error                     { return nil }

func (f *fakeNetworks) Networks(context.Context) []csapi.Network {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []csapi.Network
	for _, name := range networks {
		n := csapi.Network{Name: name, Running: true, State: "CONNECTED", Settings: map[string]bool{},
			Flows: []bridgev2.LoginFlow{{ID: "qr", Name: "QR code", Description: "Scan a QR code with your phone"}}}
		if name == "slack" {
			n.Settings["include_channels"] = true
		}
		if f.loggedIn == nil || f.loggedIn[name] {
			n.Logins = []csapi.NetworkLogin{{ID: networkid.UserLoginID(name + "-demo"), Name: accounts[name], State: "CONNECTED"}}
		} else {
			n.State = ""
		}
		out = append(out, n)
	}
	return out
}

// The demo login shows a QR code and "scans" itself after a few seconds.
func (f *fakeNetworks) StartLogin(_ context.Context, network, _ string) (*csapi.LoginStep, error) {
	return &csapi.LoginStep{LoginID: "demo-" + network, Step: &bridgev2.LoginStep{
		Type: bridgev2.LoginStepTypeDisplayAndWait, StepID: "demo.qr",
		Instructions: "Open " + networkNames[network] + " on your phone → Linked devices → Link a device, and scan this code.",
		DisplayAndWaitParams: &bridgev2.LoginDisplayAndWaitParams{Type: bridgev2.LoginDisplayTypeQR,
			Data: "https://github.com/whyyor/tuimessenger#demo-" + network},
	}}, nil
}

func (f *fakeNetworks) SubmitLogin(ctx context.Context, loginID, _ string, _ bridgev2.LoginStepType, _ map[string]string) (*csapi.LoginStep, error) {
	select {
	case <-time.After(6 * time.Second):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	network := loginID[len("demo-"):]
	f.mu.Lock()
	if f.loggedIn != nil {
		f.loggedIn[network] = true
	}
	f.mu.Unlock()
	return &csapi.LoginStep{LoginID: loginID, Step: &bridgev2.LoginStep{Type: bridgev2.LoginStepTypeComplete, StepID: "demo.done",
		CompleteParams: &bridgev2.LoginCompleteParams{UserLoginID: networkid.UserLoginID(network + "-demo")}}}, nil
}

func (f *fakeNetworks) Logout(_ context.Context, network string, _ networkid.UserLoginID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.loggedIn == nil {
		f.loggedIn = map[string]bool{}
		for _, n := range networks {
			f.loggedIn[n] = true
		}
	}
	f.loggedIn[network] = false
	return nil
}

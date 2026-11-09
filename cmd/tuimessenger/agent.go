package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"maunium.net/go/mautrix/id"

	"github.com/whyyor/tuimessenger/internal/agent"
	"github.com/whyyor/tuimessenger/internal/login"
)

const agentUsage = `tuimessenger agent: your chats for scripts and AI agents. Output is JSON.

  tuimessenger agent status                          logged in? backend running? how fresh is the data?
  tuimessenger agent chats [-unread] [-limit N]      chats, newest first
  tuimessenger agent messages [-limit N] CHAT        a chat's newest messages, oldest first
  tuimessenger agent search [-chat CHAT] [-limit N] TEXT
  tuimessenger agent send CHAT TEXT -yes [-wait 15s] send a text message (without -yes: only shows what would be sent)
  tuimessenger agent login                           the agent's own login (asks for the backend password once)
  tuimessenger agent logout                          revoke that login

CHAT is a chat id (!…), a name or alias ("Maya"), or network:name ("whatsapp:Maya").
Every command takes -data DIR (default: tuimessenger's data folder). Reading works with the TUI closed;
sending needs the backend running. Data is as fresh as the TUI's last sync ("synced_at").`

// runAgent is `tuimessenger agent …`.
func runAgent(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Println(agentUsage)
		return 0
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	cmd := args[0]
	fs := flag.NewFlagSet("tuimessenger agent "+cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dataDir := fs.String("data", defaultDataDir(), "tuimessenger's data folder")
	limit := fs.Int("limit", 0, "how many")
	unread := fs.Bool("unread", false, "only chats with unread messages")
	chatFlag := fs.String("chat", "", "only this chat")
	yes := fs.Bool("yes", false, "really send")
	wait := fs.Duration("wait", 15*time.Second, "how long to wait for the network to confirm a send")
	pos, err := parseAnywhere(fs, args[1:])
	if err != nil {
		return agentFail(fmt.Errorf("%w (see tuimessenger agent help)", err))
	}
	r, err := agent.Open(ctx, *dataDir)
	if err != nil {
		return agentFail(err)
	}
	defer r.Close()
	switch cmd {
	case "status":
		b, err := agent.NewBackend(r, *dataDir)
		if err != nil {
			return agentFail(err)
		}
		return agentOut(map[string]any{
			"backend": r.Account.HomeserverURL, "user": r.Account.UserID, "logged_in": b.LoggedIn(),
			"backend_running": b.Running(ctx), "synced_at": r.SyncedAt(),
		})
	case "chats":
		chats, err := r.Chats(ctx, *unread, *limit)
		if err != nil {
			return agentFail(err)
		}
		return agentOut(map[string]any{"chats": nonNil(chats), "synced_at": r.SyncedAt()})
	case "messages":
		if len(pos) != 1 {
			return agentFail(errors.New("usage: tuimessenger agent messages [-limit N] CHAT"))
		}
		chat, err := r.Resolve(ctx, pos[0])
		if err != nil {
			return agentFail(err)
		}
		msgs, err := r.Messages(ctx, chat.ID, orDefault(*limit, 30))
		if err != nil {
			return agentFail(err)
		}
		chat.LastMessage = nil
		return agentOut(map[string]any{"chat": chat, "messages": nonNil(msgs), "synced_at": r.SyncedAt()})
	case "search":
		if len(pos) == 0 {
			return agentFail(errors.New("usage: tuimessenger agent search [-chat CHAT] [-limit N] TEXT"))
		}
		var rooms []id.RoomID
		if *chatFlag != "" {
			chat, err := r.Resolve(ctx, *chatFlag)
			if err != nil {
				return agentFail(err)
			}
			rooms = []id.RoomID{chat.ID}
		}
		msgs, err := r.Search(ctx, strings.Join(pos, " "), rooms, orDefault(*limit, 30))
		if err != nil {
			return agentFail(err)
		}
		return agentOut(map[string]any{"messages": nonNil(msgs), "synced_at": r.SyncedAt()})
	case "send":
		if len(pos) < 2 {
			return agentFail(errors.New("usage: tuimessenger agent send CHAT TEXT -yes"))
		}
		chat, err := r.Resolve(ctx, pos[0])
		if err != nil {
			return agentFail(err)
		}
		text := strings.Join(pos[1:], " ")
		chat.LastMessage = nil
		if !*yes {
			return agentOut(map[string]any{"sent": false, "chat": chat, "text": text,
				"note": "nothing was sent: show this to the user, and add -yes once they agree"})
		}
		b, err := agent.NewBackend(r, *dataDir)
		if err != nil {
			return agentFail(err)
		}
		res, err := b.SendText(ctx, chat.ID, text, *wait)
		if err != nil {
			return agentFail(err)
		}
		out := agentOut(map[string]any{"sent": true, "chat": chat, "text": text, "result": res})
		if res.Status == "failed" {
			return 1
		}
		return out
	case "login":
		b, err := agent.NewBackend(r, *dataDir)
		if err != nil {
			return agentFail(err)
		}
		if !isTerminal() {
			return agentFail(errors.New("run `tuimessenger agent login` yourself in a terminal: it asks for the backend password"))
		}
		password, err := login.NewTerminal().AskSecret("Backend password (the one the TUI logs in with): ")
		if err != nil {
			return agentFail(err)
		}
		if err = b.Login(ctx, password); err != nil {
			return agentFail(err)
		}
		return agentOut(map[string]any{"logged_in": true, "device": agent.DeviceID, "token_file": agent.TokenPath(*dataDir)})
	case "logout":
		b, err := agent.NewBackend(r, *dataDir)
		if err != nil {
			return agentFail(err)
		}
		if err = b.Logout(ctx); err != nil {
			return agentFail(err)
		}
		return agentOut(map[string]any{"logged_in": false})
	}
	return agentFail(fmt.Errorf("unknown command %q (see tuimessenger agent help)", cmd))
}

// parseAnywhere lets flags come before or after the other arguments; everything after -- is text.
func parseAnywhere(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for len(args) > 0 {
		if args[0] == "--" {
			return append(pos, args[1:]...), nil
		}
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) > 0 && len(args) > len(rest) && args[len(args)-len(rest)-1] == "--" {
			return append(pos, rest...), nil
		}
		if len(rest) == 0 {
			break
		}
		pos, args = append(pos, rest[0]), rest[1:]
	}
	return pos, nil
}

func agentOut(v any) int {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return 0
}

// agentFail prints {"error": …} (with the candidates when a name matched several chats) and returns exit code 1.
func agentFail(err error) int {
	out := map[string]any{"error": err.Error()}
	var amb *agent.AmbiguousError
	if errors.As(err, &amb) {
		for _, c := range amb.Matches {
			c.LastMessage = nil
		}
		out["matches"] = amb.Matches
	}
	agentOut(out)
	return 1
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func orDefault(n, def int) int {
	if n <= 0 {
		return def
	}
	return n
}

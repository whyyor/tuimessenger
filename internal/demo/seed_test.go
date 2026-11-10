package demo

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/event"

	"github.com/whyyor/tuimessenger/internal/matrixconn"
	"github.com/whyyor/tuimessenger/internal/media"
	"github.com/whyyor/tuimessenger/internal/store"
	"github.com/whyyor/tuimessenger/internal/typing"
)

// Seeding creates every chat once (members: the people, you and the network bot); a second run (a restarted demo) finds them and adds nothing.
func TestSeedTwice(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(dir, "store.db"), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	med, err := media.New(filepath.Join(dir, "media"))
	if err != nil {
		t.Fatal(err)
	}
	conns := map[string]*matrixconn.Connector{}
	for _, name := range networks {
		conns[name] = matrixconn.New(zerolog.Nop(), st, med, typing.New(), name, serverName, owner)
	}
	for run := 1; run <= 2; run++ {
		rooms, err := seed(ctx, conns, owner, filepath.Join(dir, "no-media"))
		if err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		if len(rooms) != len(chats()) {
			t.Fatalf("run %d: %d rooms, want %d", run, len(rooms), len(chats()))
		}
		for _, r := range rooms {
			members, err := st.GetStateOfType(ctx, r.id, event.StateMember)
			if err != nil || len(members) != len(r.people)+2 {
				t.Fatalf("run %d, %s: %d members (%v), want %d", run, r.key, len(members), err, len(r.people)+2)
			}
		}
	}
}

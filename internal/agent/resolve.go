package agent

import (
	"context"
	"fmt"
	"strings"

	"maunium.net/go/mautrix/id"
)

// AmbiguousError lists the chats a name matched when it matched more than one.
type AmbiguousError struct {
	Query   string
	Matches []*Chat
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("%q matches %d chats; use one of their ids, or network:name", e.Query, len(e.Matches))
}

// Resolve finds one chat by room ID, alias, or name ("maya", "whatsapp:maya"). Exact names win over partial ones.
func (r *Reader) Resolve(ctx context.Context, query string) (*Chat, error) {
	query = strings.TrimSpace(query)
	chats, err := r.Chats(ctx, false, 0)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(query, "!") {
		for _, c := range chats {
			if c.ID == id.RoomID(query) {
				return c, nil
			}
		}
		return nil, fmt.Errorf("no chat with id %s", query)
	}
	network, name := "", query
	if before, after, ok := strings.Cut(query, ":"); ok && before != "" && !strings.Contains(before, " ") {
		network, name = strings.ToLower(before), after
	}
	name = strings.ToLower(strings.TrimSpace(name))
	var exact, partial []*Chat
	for _, c := range chats {
		if network != "" && c.Network != network {
			continue
		}
		names := []string{strings.ToLower(c.Name), strings.ToLower(c.Alias)}
		switch {
		case names[0] == name || (names[1] != "" && names[1] == name):
			exact = append(exact, c)
		case strings.Contains(names[0], name) || (names[1] != "" && strings.Contains(names[1], name)):
			partial = append(partial, c)
		}
	}
	matches := exact
	if len(matches) == 0 {
		matches = partial
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no chat matches %q", query)
	case 1:
		return matches[0], nil
	}
	return nil, &AmbiguousError{Query: query, Matches: matches}
}

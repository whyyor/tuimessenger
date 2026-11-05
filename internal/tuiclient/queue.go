package tuiclient

import (
	"context"
	"sync"
)

// queue hands events from hicli to the TUI in order. Pushing never blocks, so a busy TUI can't stall syncing,
// and the TUI can safely run commands while it handles an event.
type queue struct {
	mu    sync.Mutex
	items []any
	wake  chan struct{}
}

func newQueue() *queue {
	return &queue{wake: make(chan struct{}, 1)}
}

func (q *queue) push(items ...any) {
	q.mu.Lock()
	q.items = append(q.items, items...)
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// pop waits for the next event; ok is false once ctx is done.
func (q *queue) pop(ctx context.Context) (item any, ok bool) {
	for {
		q.mu.Lock()
		if len(q.items) > 0 {
			item = q.items[0]
			q.items[0] = nil
			q.items = q.items[1:]
			q.mu.Unlock()
			return item, true
		}
		q.mu.Unlock()
		select {
		case <-q.wake:
		case <-ctx.Done():
			return nil, false
		}
	}
}

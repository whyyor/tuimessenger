package tuiclient

import (
	"context"
	"testing"
	"time"
)

func TestQueue(t *testing.T) {
	q := newQueue()
	q.push(1, 2)
	q.push(3)
	ctx := context.Background()
	for want := 1; want <= 3; want++ {
		if got, ok := q.pop(ctx); !ok || got != want {
			t.Fatalf("got %v, want %d", got, want)
		}
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		q.push(4)
	}()
	if got, ok := q.pop(ctx); !ok || got != 4 {
		t.Fatalf("pop didn't wait for the next push: %v", got)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, ok := q.pop(cancelled); ok {
		t.Error("pop should stop once the context is cancelled")
	}
}

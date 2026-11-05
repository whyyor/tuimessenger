// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package rpc

import (
	"context"
	"testing"
	"time"
)

func newTestRPC() *GomuksRPC {
	return &GomuksRPC{resyncCh: make(chan struct{}, 1)}
}

// The whole point of the wake channel: a resync requested while the retry loop
// is sleeping must not wait out the backoff, which can be 30 seconds.
func TestWaitBeforeRetryWakesOnResync(t *testing.T) {
	gr := newTestRPC()
	gr.resyncCh <- struct{}{}

	start := time.Now()
	forced, alive := gr.waitBeforeRetry(context.Background(), time.Minute)
	elapsed := time.Since(start)

	if !forced {
		t.Error("expected forced=true when a resync is pending")
	}
	if !alive {
		t.Error("expected alive=true when the context is live")
	}
	if elapsed > 5*time.Second {
		t.Errorf("waited %v; should have returned immediately", elapsed)
	}
}

func TestWaitBeforeRetryTimesOut(t *testing.T) {
	gr := newTestRPC()
	forced, alive := gr.waitBeforeRetry(context.Background(), time.Millisecond)
	if forced {
		t.Error("expected forced=false when the timer fires")
	}
	if !alive {
		t.Error("expected alive=true when the context is live")
	}
}

func TestWaitBeforeRetryReportsShutdown(t *testing.T) {
	gr := newTestRPC()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, alive := gr.waitBeforeRetry(ctx, time.Minute); alive {
		t.Error("expected alive=false once the client context is cancelled")
	}
}

// Resync must clear the resume state so the server falls back to sending full
// initial data instead of replaying from its event buffer.
func TestResyncClearsResumeState(t *testing.T) {
	gr := newTestRPC()
	gr.setRunID("some-run-id")
	gr.lastReqID.Store(1234)

	gr.Resync()

	if runID := gr.getRunID(); runID != "" {
		t.Errorf("run ID should be cleared, got %q", runID)
	}
	if last := gr.lastReqID.Load(); last != 0 {
		t.Errorf("last received event should be cleared, got %d", last)
	}
	select {
	case <-gr.resyncCh:
	default:
		t.Error("Resync should have queued a wake token")
	}
}

// Repeated presses must not block or queue up multiple redials.
func TestResyncIsIdempotentWhileQueued(t *testing.T) {
	gr := newTestRPC()
	done := make(chan struct{})
	go func() {
		for range 5 {
			gr.Resync()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Resync blocked when a wake token was already queued")
	}
	if len(gr.resyncCh) != 1 {
		t.Errorf("expected exactly 1 queued token, got %d", len(gr.resyncCh))
	}
}

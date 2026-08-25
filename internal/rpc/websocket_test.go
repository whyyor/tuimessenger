// Copyright (c) 2025 Tulir Asokan
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

func TestReconnectDelayStaysWithinBounds(t *testing.T) {
	for attempt := 0; attempt <= 20; attempt++ {
		for range 100 {
			delay := reconnectDelay(attempt)
			if delay <= 0 {
				t.Fatalf("attempt %d produced non-positive delay %v", attempt, delay)
			}
			if delay > reconnectMaxDelay {
				t.Fatalf("attempt %d produced delay %v above cap %v", attempt, delay, reconnectMaxDelay)
			}
		}
	}
}

func TestReconnectDelayReachesCap(t *testing.T) {
	// Past the shift cap, half jitter should keep the delay in the top half
	// of the maximum rather than collapsing back towards zero.
	for range 100 {
		delay := reconnectDelay(20)
		if delay < reconnectMaxDelay/2 {
			t.Fatalf("expected capped delay of at least %v, got %v", reconnectMaxDelay/2, delay)
		}
	}
}

func TestSleepContextReturnsFalseWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if sleepContext(ctx, time.Minute) {
		t.Fatal("sleepContext should return false when the context is already cancelled")
	}
}

func TestSleepContextReturnsTrueOnTimeout(t *testing.T) {
	if !sleepContext(context.Background(), time.Millisecond) {
		t.Fatal("sleepContext should return true when the timer fires")
	}
}

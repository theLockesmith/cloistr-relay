package startup

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryConnect_UnreachableThenReachable(t *testing.T) {
	const failCount = 3
	var attempts atomic.Int32

	connect := func() error {
		n := attempts.Add(1)
		if n <= failCount {
			return errors.New("connection refused")
		}
		return nil
	}

	start := time.Now()
	err := RetryConnect(context.Background(), "test-db", connect, BackoffOpts{
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     50 * time.Millisecond,
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	got := int(attempts.Load())
	if got != failCount+1 {
		t.Fatalf("expected %d attempts, got %d", failCount+1, got)
	}
	// 3 waits: 10ms + 20ms + 40ms = 70ms minimum
	if elapsed < 70*time.Millisecond {
		t.Fatalf("backoff too fast: %v (expected >= 70ms)", elapsed)
	}
}

func TestRetryConnect_SucceedsFirstTry(t *testing.T) {
	var attempts atomic.Int32
	err := RetryConnect(context.Background(), "test-db", func() error {
		attempts.Add(1)
		return nil
	}, DefaultBackoff)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := int(attempts.Load()); got != 1 {
		t.Fatalf("expected 1 attempt, got %d", got)
	}
}

func TestRetryConnect_CancelledDuringRetry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := RetryConnect(ctx, "test-db", func() error {
		return errors.New("connection refused")
	}, BackoffOpts{
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
	})

	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got: %v", err)
	}
}

func TestRetryConnect_ExponentialBackoffCaps(t *testing.T) {
	const maxDelay = 40 * time.Millisecond
	var timestamps []time.Time

	connect := func() error {
		timestamps = append(timestamps, time.Now())
		if len(timestamps) < 6 {
			return errors.New("fail")
		}
		return nil
	}

	err := RetryConnect(context.Background(), "test-db", connect, BackoffOpts{
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     maxDelay,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Delays: 10, 20, 40, 40, 40 (capped at 40)
	// Check that later gaps don't exceed max + tolerance
	for i := 1; i < len(timestamps); i++ {
		gap := timestamps[i].Sub(timestamps[i-1])
		if gap > maxDelay+maxDelay/4+20*time.Millisecond {
			t.Errorf("gap %d->%d = %v exceeds max %v + jitter + tolerance", i-1, i, gap, maxDelay)
		}
	}
}

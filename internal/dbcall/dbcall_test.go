package dbcall

import (
	"context"
	"errors"
	"testing"
	"time"
)

// A call that ignores its context entirely -- what a query on a dead lib/pq
// connection does -- must still return at the deadline.
func TestWithDeadline_ReturnsAtDeadlineWhenCallNeverReturns(t *testing.T) {
	release := make(chan struct{})
	defer close(release)

	start := time.Now()
	_, err := WithDeadline(context.Background(), 100*time.Millisecond, func(context.Context) (int, error) {
		<-release // blocks until the test ends, ignoring ctx
		return 1, nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("returned after %v, want about 100ms", elapsed)
	}
}

func TestWithDeadline_PassesThroughResultAndError(t *testing.T) {
	v, err := WithDeadline(context.Background(), time.Second, func(context.Context) (string, error) {
		return "ok", nil
	})
	if v != "ok" || err != nil {
		t.Fatalf("got (%q, %v), want (ok, nil)", v, err)
	}
	boom := errors.New("boom")
	if _, err := WithDeadline(context.Background(), time.Second, func(context.Context) (int, error) {
		return 0, boom
	}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
}

// A parent that is already cancelled (client gone) wins over the deadline.
func TestWithDeadline_HonoursParentCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	release := make(chan struct{})
	defer close(release)
	_, err := WithDeadline(parent, time.Minute, func(context.Context) (int, error) {
		<-release
		return 0, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want Canceled", err)
	}
}

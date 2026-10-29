package search

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// Audit of !59: a setup error that never changes (a permanent schema problem)
// was retried every 30s forever, logging the same error twice a minute. The
// same error again must double the wait, up to the cap.
func TestRetrySetup_BacksOffOnRepeatedIdenticalError(t *testing.T) {
	var waits []time.Duration
	_, err := retrySetup(context.Background(), time.Second, 8*time.Second,
		func(context.Context) error { return errors.New("permission denied for table event") },
		func(_ context.Context, d time.Duration) error {
			waits = append(waits, d)
			if len(waits) == 6 {
				return context.Canceled
			}
			return nil
		})
	if err == nil {
		t.Fatal("retrySetup returned nil with a failing attempt")
	}
	want := []time.Duration{1, 2, 4, 8, 8, 8}
	for i := range want {
		want[i] *= time.Second
	}
	if fmt.Sprint(waits) != fmt.Sprint(want) {
		t.Errorf("waits = %v, want %v", waits, want)
	}
}

// A different error means something changed; go back to the base interval.
func TestRetrySetup_ResetsWaitWhenErrorChanges(t *testing.T) {
	errs := []string{"lock timeout", "lock timeout", "lock timeout", "connection refused", "connection refused"}
	var waits []time.Duration
	n := 0
	_, _ = retrySetup(context.Background(), time.Second, time.Minute,
		func(context.Context) error { e := errors.New(errs[n]); n++; return e },
		func(_ context.Context, d time.Duration) error {
			waits = append(waits, d)
			if len(waits) == len(errs) {
				return context.Canceled
			}
			return nil
		})
	want := []time.Duration{1, 2, 4, 1, 2}
	for i := range want {
		want[i] *= time.Second
	}
	if fmt.Sprint(waits) != fmt.Sprint(want) {
		t.Errorf("waits = %v, want %v", waits, want)
	}
}

func TestRetrySetup_ReturnsOnSuccess(t *testing.T) {
	n := 0
	attempts, err := retrySetup(context.Background(), time.Second, time.Minute,
		func(context.Context) error {
			n++
			if n < 3 {
				return errors.New("lock timeout")
			}
			return nil
		},
		func(context.Context, time.Duration) error { return nil })
	if err != nil || attempts != 3 {
		t.Errorf("attempts, err = %d, %v; want 3, nil", attempts, err)
	}
}

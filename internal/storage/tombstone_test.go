package storage

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

// These assert ADMISSION as well as refusal. A guard that refused every write would
// satisfy a refusal-only suite while taking the relay offline for everyone.

func TestRejectRepublishedDeletion_AdmitsAnEventNeverDeleted(t *testing.T) {
	reject, msg := RejectRepublishedDeletion(func(context.Context, string) (bool, error) {
		return false, nil
	})(context.Background(), "abc")
	if reject {
		t.Errorf("refused an event that was never deleted: %s", msg)
	}
}

func TestRejectRepublishedDeletion_RefusesARepublish(t *testing.T) {
	reject, msg := RejectRepublishedDeletion(func(context.Context, string) (bool, error) {
		return true, nil
	})(context.Background(), "abc")
	if !reject {
		t.Error("admitted a republish of a deleted event")
	}
	if msg == "" {
		t.Error("refusal carried no reason")
	}
}

// THE DISCRIMINATOR: an unhealthy database must not read as "never deleted".
func TestRejectRepublishedDeletion_LookupErrorRefuses(t *testing.T) {
	reject, msg := RejectRepublishedDeletion(func(context.Context, string) (bool, error) {
		return false, errors.New("connection refused")
	})(context.Background(), "abc")
	if !reject {
		t.Error("a failed lookup admitted the write; a broken database would resurrect deleted events")
	}
	if msg == "" {
		t.Error("refusal carried no reason")
	}
}

func TestRejectRepublishedDeletion_RetriesOnConnError(t *testing.T) {
	var calls atomic.Int32
	reject, _ := RejectRepublishedDeletion(func(context.Context, string) (bool, error) {
		if calls.Add(1) == 1 {
			return false, errors.New("server closed the connection unexpectedly")
		}
		return false, nil // second call succeeds, event not tombstoned
	})(context.Background(), "abc")
	if reject {
		t.Error("refused after a successful retry; a Patroni switchover would reject all writes")
	}
	if calls.Load() != 2 {
		t.Errorf("expected 2 calls (original + retry), got %d", calls.Load())
	}
}

func TestRejectRepublishedDeletion_NoRetryOnNonConnError(t *testing.T) {
	var calls atomic.Int32
	reject, _ := RejectRepublishedDeletion(func(context.Context, string) (bool, error) {
		calls.Add(1)
		return false, errors.New("permission denied for table deleted_event")
	})(context.Background(), "abc")
	if !reject {
		t.Error("admitted the write on a non-connection error")
	}
	if calls.Load() != 1 {
		t.Errorf("expected 1 call (no retry for non-connection errors), got %d", calls.Load())
	}
}

func TestIsConnError(t *testing.T) {
	for _, tc := range []struct {
		msg  string
		want bool
	}{
		{"server closed the connection unexpectedly", true},
		{"the database system is shutting down", true},
		{"connection reset by peer", true},
		{"broken pipe", true},
		{"bad connection", true},
		{"terminating connection due to administrator command", true},
		{"SSL connection has been closed unexpectedly", true},
		{"permission denied for table deleted_event", false},
		{"relation \"deleted_event\" does not exist", false},
		{"syntax error at or near \"SELECT\"", false},
	} {
		got := isConnError(errors.New(tc.msg))
		if got != tc.want {
			t.Errorf("isConnError(%q) = %v, want %v", tc.msg, got, tc.want)
		}
	}
}

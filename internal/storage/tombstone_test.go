package storage

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"testing"

	"github.com/lib/pq"
	"github.com/lib/pq/pqerror"
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
		return false, errors.New("something broke")
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
			return false, &pq.Error{Code: pqerror.AdminShutdown, Message: "terminating connection due to administrator command"}
		}
		return false, nil
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
		return false, &pq.Error{Code: pqerror.InsufficientPrivilege, Message: "permission denied for table deleted_event"}
	})(context.Background(), "abc")
	if !reject {
		t.Error("admitted the write on a non-connection error")
	}
	if calls.Load() != 1 {
		t.Errorf("expected 1 call (no retry for non-connection errors), got %d", calls.Load())
	}
}

// A mutation that bumps the retry count (e.g. a loop of 5) must be caught.
// An always-failing connection error must produce exactly 2 lookup calls
// (1 original + 1 retry) and the write must be refused.
func TestRejectRepublishedDeletion_ExactlyOneRetry(t *testing.T) {
	var calls atomic.Int32
	reject, _ := RejectRepublishedDeletion(func(context.Context, string) (bool, error) {
		calls.Add(1)
		return false, &pq.Error{Code: pqerror.AdminShutdown, Message: "terminating connection due to administrator command"}
	})(context.Background(), "abc")
	if !reject {
		t.Error("admitted the write after all retries failed")
	}
	if calls.Load() != 2 {
		t.Errorf("expected exactly 2 calls (1 original + 1 retry), got %d", calls.Load())
	}
}

func TestIsConnError(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		// SQLSTATE class 08 -- connection exception
		{
			name: "pq class 08 connection_exception",
			err:  &pq.Error{Code: pqerror.ConnectionException},
			want: true,
		},
		{
			name: "pq class 08 connection_failure",
			err:  &pq.Error{Code: pqerror.ConnectionFailure},
			want: true,
		},
		// SQLSTATE 57P01/57P02/57P03
		{
			name: "pq 57P01 admin_shutdown",
			err:  &pq.Error{Code: pqerror.AdminShutdown, Message: "terminating connection due to administrator command"},
			want: true,
		},
		{
			name: "pq 57P02 crash_shutdown",
			err:  &pq.Error{Code: pqerror.CrashShutdown, Message: "terminating connection due to crash of another server process"},
			want: true,
		},
		{
			name: "pq 57P03 cannot_connect_now (shutting down)",
			err:  &pq.Error{Code: pqerror.CannotConnectNow, Message: "the database system is shutting down"},
			want: true,
		},
		{
			name: "pq 57P03 cannot_connect_now (starting up)",
			err:  &pq.Error{Code: pqerror.CannotConnectNow, Message: "the database system is starting up"},
			want: true,
		},
		// Go-level transport errors
		{
			name: "driver.ErrBadConn",
			err:  driver.ErrBadConn,
			want: true,
		},
		{
			name: "wrapped driver.ErrBadConn",
			err:  fmt.Errorf("tombstone lookup abc: %w", driver.ErrBadConn),
			want: true,
		},
		{
			name: "io.EOF",
			err:  io.EOF,
			want: true,
		},
		{
			name: "io.ErrUnexpectedEOF",
			err:  io.ErrUnexpectedEOF,
			want: true,
		},
		{
			name: "net.OpError",
			err:  &net.OpError{Op: "read", Err: errors.New("connection reset by peer")},
			want: true,
		},
		// Non-connection errors
		{
			name: "pq 42501 insufficient_privilege",
			err:  &pq.Error{Code: pqerror.InsufficientPrivilege, Message: "permission denied for table deleted_event"},
			want: false,
		},
		{
			name: "pq 42P01 undefined_table",
			err:  &pq.Error{Code: pqerror.UndefinedTable, Message: "relation \"deleted_event\" does not exist"},
			want: false,
		},
		{
			name: "pq 42601 syntax_error",
			err:  &pq.Error{Code: pqerror.SyntaxError, Message: "syntax error at or near \"SELECT\""},
			want: false,
		},
		{
			name: "generic error (not pq, not transport)",
			err:  errors.New("something unrelated"),
			want: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := isConnError(tc.err)
			if got != tc.want {
				t.Errorf("isConnError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

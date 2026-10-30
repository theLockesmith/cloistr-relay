// Package dbcall bounds how long a request can wait on one database call.
//
// lib/pq's context cancellation sends a CancelRequest on a NEW socket but never
// unblocks a read already blocked on the original one, so a query on a dead
// connection hangs until TCP gives up despite context.WithTimeout. That is how
// tombstone lookups held writes for up to ~15 minutes in October 2026
// (storage.lookupWithDeadline is the original fix; this is the same pattern,
// shared). WithDeadline runs the call in a goroutine and returns
// context.DeadlineExceeded when the deadline fires, abandoning the goroutine.
// pq marks the connection bad and the pool discards it when the read finally
// returns (bounded by TCP_USER_TIMEOUT on the tuned connector).
//
// Abandoning a WRITE means it may still commit after the caller was told it
// failed. Callers that use this for writes must make a retry safe.
package dbcall

import (
	"context"
	"time"
)

// WithDeadline calls fn with a context that expires after d, and returns no
// later than that deadline even if fn ignores its context.
func WithDeadline[T any](parent context.Context, d time.Duration, fn func(ctx context.Context) (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(parent, d)
	defer cancel()

	type result struct {
		v   T
		err error
	}
	ch := make(chan result, 1) // buffered: an abandoned call must not block forever on send
	go func() {
		v, err := fn(ctx)
		ch <- result{v, err}
	}()

	select {
	case r := <-ch:
		return r.v, r.err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}

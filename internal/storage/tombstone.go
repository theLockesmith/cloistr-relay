package storage

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"github.com/lib/pq"
	"github.com/lib/pq/pqerror"
	"github.com/nbd-wtf/go-nostr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Tombstones record the ids of events this relay has deleted, so a saved copy
// cannot simply be republished.
//
// WHY THIS MATTERS MORE FOR THREADS THAN FOR ORDINARY EVENTS. A thread message is
// wrapped under a one-time key that is discarded the moment it is used. Only that
// key could ever sign a NIP-09 deletion for the wrap, so the author of a thread
// message can NEVER delete it. Deletion is therefore an operator action on their
// own relay, and without a tombstone it is undone by anyone who kept a copy and
// posts it back.
//
// The table is created with a HARD failure, unlike the warn-and-continue used for
// index creation a few files over. An index that fails to build costs query time.
// A tombstone table that fails to create makes deletion look permanent while being
// reversible by anyone, with the relay reporting healthy throughout, which is worse
// than not having the feature at all.

const tombstoneDDL = `
CREATE TABLE IF NOT EXISTS deleted_event (
	id          TEXT PRIMARY KEY,
	kind        INTEGER NOT NULL,
	deleted_at  BIGINT  NOT NULL
)`

var tombstoneAttemptTimeout = 5 * time.Second

const maxInflightTombstoneLookups = 8

var tombstoneLookupSem = make(chan struct{}, maxInflightTombstoneLookups)

var tombstoneSemWaitSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
	Name:    "nostr_relay_tombstone_sem_wait_seconds",
	Help:    "Time spent waiting for a tombstone lookup semaphore slot",
	Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
})

// tombstoneLookupSeconds times each lookup attempt, from the semaphore slot to
// the answer or the attempt deadline, so the slow tail is visible before it
// reaches the timeout.
var tombstoneLookupSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
	Name:    "nostr_relay_tombstone_lookup_seconds",
	Help:    "Duration of each tombstone lookup attempt, including attempts abandoned at the deadline.",
	Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
})

var tombstoneLookupFailures = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "nostr_relay_tombstone_lookup_failures_total",
	Help: "Tombstone lookup failures by cause",
}, []string{"cause"})

// EnsureTombstoneTable creates the tombstone table. It returns an error rather
// than logging one: see the note above.
func EnsureTombstoneTable(db *sql.DB) error {
	if _, err := db.Exec(tombstoneDDL); err != nil {
		return fmt.Errorf("create deleted_event table: %w", err)
	}
	return nil
}

// RecordTombstone notes that an event id has been deleted here.
func RecordTombstone(ctx context.Context, db *sql.DB, id string, kind int, now int64) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO deleted_event (id, kind, deleted_at) VALUES ($1, $2, $3)
		 ON CONFLICT (id) DO NOTHING`, id, kind, now)
	if err != nil {
		return fmt.Errorf("record tombstone %s: %w", id, err)
	}
	return nil
}

// IsTombstoned reports whether an event id has previously been deleted here.
func IsTombstoned(ctx context.Context, db *sql.DB, id string) (bool, error) {
	var one int
	err := db.QueryRowContext(ctx, `SELECT 1 FROM deleted_event WHERE id = $1`, id).Scan(&one)
	switch {
	case err == sql.ErrNoRows:
		return false, nil
	case err != nil:
		return false, fmt.Errorf("tombstone lookup %s: %w", id, err)
	}
	return true, nil
}

// TombstoneLookup is the only thing the reject decision needs, taken as a function
// so it is testable without a database.
type TombstoneLookup func(ctx context.Context, id string) (bool, error)

// connErrorSQLSTATEs lists specific SQLSTATE codes that indicate a retriable
// connection problem outside class 08 (which is checked by prefix).
var connErrorSQLSTATEs = map[pqerror.Code]bool{
	pqerror.AdminShutdown:    true, // 57P01: "terminating connection due to administrator command"
	pqerror.CrashShutdown:    true, // 57P02: "terminating connection due to crash of another server process"
	pqerror.CannotConnectNow: true, // 57P03: "the database system is shutting down" / "starting up"
}

// isConnError reports whether err indicates a closed or broken connection that
// the pool handed out. A retry on a fresh connection is likely to succeed.
//
// Classification is by type and SQLSTATE, not by message substring:
//  1. driver.ErrBadConn -- the pool already knows the conn is dead
//  2. io.EOF / io.ErrUnexpectedEOF -- TCP stream severed
//  3. net.OpError -- OS-level network failure
//  4. pq.Error with SQLSTATE class 08 (connection exception)
//  5. pq.Error with SQLSTATE 57P01/57P02/57P03 (admin/crash shutdown, cannot connect)
func isConnError(err error) bool {
	if errors.Is(err, driver.ErrBadConn) {
		return true
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return true
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		if pqErr.Code.Class() == pqerror.ClassConnectionException {
			return true
		}
		if connErrorSQLSTATEs[pqErr.Code] {
			return true
		}
	}
	return false
}

type lookupResult struct {
	deleted bool
	err     error
}

// lookupWithDeadline runs the lookup in a goroutine and abandons it if the
// deadline fires. lib/pq's ctx cancel sends a CancelRequest on a new socket
// but never unblocks the blocked read on the original one, so a dead
// connection hangs indefinitely despite context.WithTimeout. The goroutine +
// select pattern returns ctx.Err() immediately on deadline, abandoning the
// blocked goroutine. The goroutine's connection is marked bad by pq and will
// be discarded by the pool when (if) the read eventually returns.
func lookupWithDeadline(parent context.Context, lookup TombstoneLookup, id string) (bool, error) {
	ctx, cancel := context.WithTimeout(parent, tombstoneAttemptTimeout)
	defer cancel()

	// Each abandoned goroutine holds a pool connection until the socket dies
	// (~30s with TCP_USER_TIMEOUT). Cap in-flight lookups so a DB blip
	// cannot exhaust the pool and stall every other rawDB user. Wait for a
	// slot rather than failing instantly so healthy-server bursts are not
	// rejected.
	waitStart := time.Now()
	select {
	case tombstoneLookupSem <- struct{}{}:
	case <-ctx.Done():
		tombstoneSemWaitSeconds.Observe(time.Since(waitStart).Seconds())
		return false, ctx.Err()
	}
	tombstoneSemWaitSeconds.Observe(time.Since(waitStart).Seconds())

	lookupStart := time.Now()
	defer func() { tombstoneLookupSeconds.Observe(time.Since(lookupStart).Seconds()) }()

	ch := make(chan lookupResult, 1)
	go func() {
		defer func() { <-tombstoneLookupSem }()
		deleted, err := lookup(ctx, id)
		ch <- lookupResult{deleted, err}
	}()
	select {
	case r := <-ch:
		return r.deleted, r.err
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func classifyTombstoneError(parentCtx context.Context, err error) string {
	if parentCtx.Err() == context.Canceled {
		return "client_gone"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	// 57014 = query_canceled. Sent by PG when our CancelRequest is processed.
	// The abandon wrapper normally returns ctx.Err() before this arrives, but
	// if pq wins the race, the cancel was still ours (attempt deadline) and
	// the right action is the same: retry.
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "57014" {
		return "timeout"
	}
	if isConnError(err) {
		return "conn_error"
	}
	return "other"
}

// RejectRepublishedDeletion refuses an event whose id is tombstoned.
//
// A lookup ERROR refuses the write rather than admitting it. That direction is
// deliberate: a relay that cannot tell whether something was deleted should not be
// the one that brings it back.
//
// Each attempt is capped at tombstoneAttemptTimeout so a dead connection fails
// fast instead of hanging for minutes. One retry on a connection error or timeout:
// during a Patroni switchover the pool can hand out a stale connection whose
// server-side session is gone. The first query fails, database/sql discards the
// connection, and the retry gets a fresh one to the new leader.
//
// Client disconnects (context.Canceled from the request context) are classified
// separately and not retried.
func RejectRepublishedDeletion(lookup TombstoneLookup) func(context.Context, string) (bool, string) {
	return func(ctx context.Context, id string) (reject bool, msg string) {
		deleted, err := lookupWithDeadline(ctx, lookup, id)
		if err == nil {
			if deleted {
				return true, "blocked: this event was deleted from this relay"
			}
			return false, ""
		}

		cause := classifyTombstoneError(ctx, err)

		if cause == "client_gone" {
			tombstoneLookupFailures.WithLabelValues("client_gone").Inc()
			return true, "client-gone: deletion check interrupted by client disconnect"
		}

		// Retryable: connection error or per-attempt timeout (hung connection).
		// Only retry if the request context is still alive.
		if ctx.Err() == nil && (cause == "conn_error" || cause == "timeout") {
			log.Printf("tombstone lookup failed for %s (retrying, cause=%s): %v", id, cause, err)
			deleted, err = lookupWithDeadline(ctx, lookup, id)
			if err == nil {
				if deleted {
					return true, "blocked: this event was deleted from this relay"
				}
				return false, ""
			}
			cause = classifyTombstoneError(ctx, err)
		}

		tombstoneLookupFailures.WithLabelValues(cause).Inc()
		log.Printf("tombstone lookup failed for %s (cause=%s): %v", id, cause, err)
		return true, fmt.Sprintf("error: could not check deletion status (%s), refusing", cause)
	}
}

// RejectRepublishedDeletionEvent is RejectRepublishedDeletion for a whole event.
// Ephemeral events (20000-29999) are never stored, so there is nothing to
// re-delete and no lookup is made. Those include NIP-46 signer messages, which
// a slow lookup would otherwise stall in the middle of a client's login.
func RejectRepublishedDeletionEvent(lookup TombstoneLookup) func(context.Context, *nostr.Event) (bool, string) {
	rejectID := RejectRepublishedDeletion(lookup)
	return func(ctx context.Context, evt *nostr.Event) (bool, string) {
		if nostr.IsEphemeralKind(evt.Kind) {
			return false, ""
		}
		return rejectID(ctx, evt.ID)
	}
}

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

	"github.com/lib/pq"
	"github.com/lib/pq/pqerror"
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

// RejectRepublishedDeletion refuses an event whose id is tombstoned.
//
// A lookup ERROR refuses the write rather than admitting it. That direction is
// deliberate: a relay that cannot tell whether something was deleted should not be
// the one that brings it back.
//
// One retry on a connection error: during a Patroni switchover the pool can hand
// out a stale connection whose server-side session is gone. The first query fails,
// database/sql discards the connection, and the retry gets a fresh one to the new
// leader. Without this, every write during and shortly after a failover is rejected.
func RejectRepublishedDeletion(lookup TombstoneLookup) func(context.Context, string) (bool, string) {
	return func(ctx context.Context, id string) (reject bool, msg string) {
		deleted, err := lookup(ctx, id)
		if err != nil {
			if isConnError(err) {
				log.Printf("tombstone lookup failed for %s (retrying): %v", id, err)
				deleted, err = lookup(ctx, id)
			}
			if err != nil {
				log.Printf("tombstone lookup failed for %s: %v", id, err)
				return true, "error: could not check deletion status, refusing"
			}
		}
		if deleted {
			return true, "blocked: this event was deleted from this relay"
		}
		return false, ""
	}
}

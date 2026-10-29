package search

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

// holdReader opens a transaction that reads the event table and keeps it open,
// holding ACCESS SHARE the way a long REQ or the expiry sweeper's scan does.
func holdReader(t *testing.T, db *sql.DB) *sql.Tx {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin reader: %v", err)
	}
	var n int
	if err := tx.QueryRow(`SELECT count(*) FROM event`).Scan(&n); err != nil {
		t.Fatalf("reader scan: %v", err)
	}
	return tx
}

// Production 2026-10-29: both relay pods logged "add search_tsv column: lock
// timeout (55P03)" at startup while the expiry sweeper scanned the table, and
// search stayed off until the next restart. A table that already has the
// column and trigger must not need an exclusive lock again.
func TestInitSchema_AlreadySetUpSucceedsUnderLongReader(t *testing.T) {
	db := testDB(t)
	defer func() { _ = db.Close() }()
	setupSearchTable(t, db)

	sb := NewSearchBackend(db, DefaultSearchKinds)
	if err := sb.InitSchema(context.Background()); err != nil {
		t.Fatalf("first InitSchema: %v", err)
	}

	reader := holdReader(t, db)
	defer func() { _ = reader.Rollback() }()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	start := time.Now()
	if err := sb.InitSchema(ctx); err != nil {
		t.Fatalf("second InitSchema with a reader open: %v (after %v)", err, time.Since(start).Round(time.Millisecond))
	}
}

// A first-time setup still needs the exclusive lock. One failed attempt must
// not leave search off until the next restart: retry until the reader is gone.
func TestInitSchemaRetry_WaitsOutABusyTable(t *testing.T) {
	db := testDB(t)
	defer func() { _ = db.Close() }()
	setupSearchTable(t, db)

	reader := holdReader(t, db)
	go func() {
		time.Sleep(7 * time.Second) // past one 5s lock_timeout
		_ = reader.Rollback()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	sb := NewSearchBackend(db, DefaultSearchKinds)
	attempts, err := sb.InitSchemaRetry(ctx, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("InitSchemaRetry: %v after %d attempts", err, attempts)
	}
	if attempts < 2 {
		t.Errorf("attempts = %d, want >= 2 (the first should have hit the lock)", attempts)
	}
}

func TestInitSchemaRetry_StopsWhenContextEnds(t *testing.T) {
	db := testDB(t)
	defer func() { _ = db.Close() }()
	setupSearchTable(t, db)

	reader := holdReader(t, db)
	defer func() { _ = reader.Rollback() }()

	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()
	sb := NewSearchBackend(db, DefaultSearchKinds)
	if _, err := sb.InitSchemaRetry(ctx, 500*time.Millisecond); err == nil {
		t.Fatal("InitSchemaRetry returned nil while the table stayed locked")
	}
}

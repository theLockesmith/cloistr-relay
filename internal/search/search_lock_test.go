package search

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
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

// Audit of !58: setup retries shared the 30-minute backfill deadline, so a
// first-time setup behind a long-held lock gave up and search stayed off
// until restart. Retries must outlast the backfill window.
func TestActivate_SetupRetriesOutliveBackfillWindow(t *testing.T) {
	db := testDB(t)
	defer func() { _ = db.Close() }()
	setupSearchTable(t, db)

	reader := holdReader(t, db)
	go func() {
		time.Sleep(7 * time.Second) // longer than the backfill window below
		_ = reader.Rollback()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	var stored *SearchBackend
	sb := NewSearchBackend(db, DefaultSearchKinds)
	if err := sb.Activate(ctx, 500*time.Millisecond, 3*time.Second, func(b *SearchBackend) { stored = b }); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if stored != sb {
		t.Fatal("Activate returned nil but never stored the backend")
	}
}

// Audit of !58: the column check read information_schema across every
// schema, so an event table with search_tsv elsewhere made setup skip adding
// the column here, and the index build then failed.
func TestInitSchema_IgnoresEventTableInOtherSchema(t *testing.T) {
	db := testDB(t)
	defer func() { _ = db.Close() }()
	setupSearchTable(t, db)

	if _, err := db.Exec(`
		DROP SCHEMA IF EXISTS search_other CASCADE;
		CREATE SCHEMA search_other;
		CREATE TABLE search_other.event (id text, search_tsv tsvector);
	`); err != nil {
		t.Fatalf("create other schema: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DROP SCHEMA IF EXISTS search_other CASCADE`) })

	sb := NewSearchBackend(db, DefaultSearchKinds)
	if err := sb.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	var has bool
	if err := db.QueryRow(`
		SELECT EXISTS (SELECT 1 FROM information_schema.columns
		               WHERE table_schema = current_schema()
		                 AND table_name = 'event' AND column_name = 'search_tsv')
	`).Scan(&has); err != nil {
		t.Fatalf("check column: %v", err)
	}
	if !has {
		t.Fatal("search_tsv was not added to the event table in the current schema")
	}
}

// Audit of !61: the setup-failure alert used a 1h increase window, so after
// early failures and a recovery it kept firing for up to an hour saying search
// was off. SearchActive lets it tell the two apart: 0 while setup is blocked,
// 1 once the backend is stored.
func TestActivate_SearchActiveGaugeTracksServing(t *testing.T) {
	db := testDB(t)
	defer func() { _ = db.Close() }()
	setupSearchTable(t, db)

	SearchActive.Set(1) // a stale 1 must not survive into a new setup
	reader := holdReader(t, db)
	release := make(chan struct{})
	go func() {
		<-release
		_ = reader.Rollback()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	done := make(chan error, 1)
	sb := NewSearchBackend(db, DefaultSearchKinds)
	go func() { done <- sb.Activate(ctx, 500*time.Millisecond, 10*time.Second, func(*SearchBackend) {}) }()

	time.Sleep(2 * time.Second) // setup is retrying behind the reader's lock
	if got := testutil.ToFloat64(SearchActive); got != 0 {
		t.Errorf("SearchActive while setup is blocked = %v, want 0", got)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if got := testutil.ToFloat64(SearchActive); got != 1 {
		t.Errorf("SearchActive after Activate = %v, want 1", got)
	}
}

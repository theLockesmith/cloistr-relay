package expiry

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/nostr_test?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("cannot open test database: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		t.Skipf("cannot reach test database: %v", err)
	}
	return db
}

func setupEventTable(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`
		CREATE OR REPLACE FUNCTION tags_to_tagvalues(jsonb) RETURNS text[]
		AS 'SELECT COALESCE(array_agg((t->>0) || '':'' || (t->>1)), ''{}''::text[])
		    FROM (SELECT jsonb_array_elements($1) AS t)s
		    WHERE length(t->>0) = 1;'
		LANGUAGE SQL IMMUTABLE RETURNS NULL ON NULL INPUT;

		DROP TABLE IF EXISTS event;
		CREATE TABLE event (
			id         text NOT NULL,
			pubkey     text NOT NULL,
			created_at integer NOT NULL,
			kind       integer NOT NULL,
			tags       jsonb NOT NULL,
			content    text NOT NULL,
			sig        text NOT NULL,
			tagvalues  text[] GENERATED ALWAYS AS (tags_to_tagvalues(tags)) STORED
		);
		CREATE UNIQUE INDEX IF NOT EXISTS ididx ON event USING btree (id text_pattern_ops);
	`)
	if err != nil {
		t.Fatalf("setup event table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec("DROP TABLE IF EXISTS event")
	})
}

func insertEvent(t *testing.T, db *sql.DB, id string, kind int, createdAt int64, expirationTag string) {
	t.Helper()
	tags := "[]"
	if expirationTag != "" {
		tags = fmt.Sprintf(`[["expiration","%s"]]`, expirationTag)
	}
	_, err := db.Exec(`
		INSERT INTO event (id, pubkey, created_at, kind, tags, content, sig)
		VALUES ($1, 'aabbccdd', $2, $3, $4::jsonb, '', 'deadbeef')
	`, id, createdAt, kind, tags)
	if err != nil {
		t.Fatalf("insert event %s: %v", id, err)
	}
}

func eventExists(t *testing.T, db *sql.DB, id string) bool {
	t.Helper()
	var n int
	err := db.QueryRow("SELECT 1 FROM event WHERE id = $1", id).Scan(&n)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		t.Fatalf("check event %s: %v", id, err)
	}
	return true
}

func eventCount(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRow("SELECT COUNT(*) FROM event").Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	return n
}

func TestSweep_DryRun_CountsByKind(t *testing.T) {
	db := testDB(t)
	defer func() { _ = db.Close() }()
	setupEventTable(t, db)

	now := time.Now().Unix()
	past := fmt.Sprintf("%d", now-3600)
	future := fmt.Sprintf("%d", now+3600)

	insertEvent(t, db, "expired-note-1", 1, now-7200, past)
	insertEvent(t, db, "expired-note-2", 1, now-7200, past)
	insertEvent(t, db, "expired-dm", 4, now-7200, past)
	insertEvent(t, db, "live-note", 1, now-100, future)
	insertEvent(t, db, "no-expiry", 1, now-100, "")

	sw := New(db, Config{
		DryRun:        true,
		GiftWrapGrace: 30 * 24 * time.Hour,
		BatchSize:     1000,
	})

	result, err := sw.Sweep(context.Background())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}

	if result.WouldDelete != 3 {
		t.Errorf("dry-run WouldDelete = %d, want 3", result.WouldDelete)
	}
	if result.Deleted != 0 {
		t.Errorf("dry-run Deleted = %d, want 0", result.Deleted)
	}

	// All 5 events should still exist (dry run deletes nothing)
	if n := eventCount(t, db); n != 5 {
		t.Errorf("event count after dry run = %d, want 5", n)
	}

	kindMap := make(map[int]int64)
	for _, kc := range result.Counts {
		kindMap[kc.Kind] = kc.Count
	}
	if kindMap[1] != 2 {
		t.Errorf("kind 1 count = %d, want 2", kindMap[1])
	}
	if kindMap[4] != 1 {
		t.Errorf("kind 4 count = %d, want 1", kindMap[4])
	}
}

func TestSweep_Live_DeletesExpired(t *testing.T) {
	db := testDB(t)
	defer func() { _ = db.Close() }()
	setupEventTable(t, db)

	now := time.Now().Unix()
	past := fmt.Sprintf("%d", now-3600)
	future := fmt.Sprintf("%d", now+3600)

	insertEvent(t, db, "expired-1", 1, now-7200, past)
	insertEvent(t, db, "expired-2", 7, now-7200, past)
	insertEvent(t, db, "live-1", 1, now-100, future)
	insertEvent(t, db, "no-expiry", 1, now-100, "")

	sw := New(db, Config{
		DryRun:        false,
		GiftWrapGrace: 30 * 24 * time.Hour,
		BatchSize:     1000,
		BatchPause:    time.Millisecond,
	})

	result, err := sw.Sweep(context.Background())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}

	if result.Deleted != 2 {
		t.Errorf("Deleted = %d, want 2", result.Deleted)
	}

	if eventExists(t, db, "expired-1") {
		t.Error("expired-1 should have been deleted")
	}
	if eventExists(t, db, "expired-2") {
		t.Error("expired-2 should have been deleted")
	}
	if !eventExists(t, db, "live-1") {
		t.Error("live-1 should still exist")
	}
	if !eventExists(t, db, "no-expiry") {
		t.Error("no-expiry should still exist")
	}
}

func TestSweep_GiftWrapGracePeriod(t *testing.T) {
	db := testDB(t)
	defer func() { _ = db.Close() }()
	setupEventTable(t, db)

	now := time.Now().Unix()
	graceDays := 30
	graceSeconds := int64(graceDays * 86400)

	// Expired 1 hour ago: within grace, should NOT be deleted
	recentExpiry := fmt.Sprintf("%d", now-3600)
	// Expired 31 days ago: past grace, should be deleted
	oldExpiry := fmt.Sprintf("%d", now-graceSeconds-86400)
	// Regular kind expired 1 hour ago: no grace, should be deleted
	regularExpiry := fmt.Sprintf("%d", now-3600)

	insertEvent(t, db, "wrap-recent", kindGiftWrap, now-7200, recentExpiry)
	insertEvent(t, db, "wrap-old", kindGiftWrap, now-7200, oldExpiry)
	insertEvent(t, db, "note-expired", 1, now-7200, regularExpiry)

	sw := New(db, Config{
		DryRun:        false,
		GiftWrapGrace: time.Duration(graceDays) * 24 * time.Hour,
		BatchSize:     1000,
		BatchPause:    time.Millisecond,
	})

	result, err := sw.Sweep(context.Background())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}

	if result.Deleted != 2 {
		t.Errorf("Deleted = %d, want 2 (wrap-old + note-expired)", result.Deleted)
	}

	if !eventExists(t, db, "wrap-recent") {
		t.Error("wrap-recent (kind 1059, within grace) should still exist")
	}
	if eventExists(t, db, "wrap-old") {
		t.Error("wrap-old (kind 1059, past grace) should have been deleted")
	}
	if eventExists(t, db, "note-expired") {
		t.Error("note-expired (kind 1, no grace) should have been deleted")
	}
}

func TestSweep_MalformedExpiration(t *testing.T) {
	db := testDB(t)
	defer func() { _ = db.Close() }()
	setupEventTable(t, db)

	now := time.Now().Unix()
	past := fmt.Sprintf("%d", now-3600)

	insertEvent(t, db, "good-expired", 1, now-7200, past)
	insertEvent(t, db, "bad-text", 1, now-7200, "not-a-number")
	insertEvent(t, db, "bad-negative", 1, now-7200, "-12345")

	// Insert an event with an empty expiration value directly (insertEvent
	// skips empty strings, so we insert the tag JSON ourselves).
	_, err := db.Exec(`
		INSERT INTO event (id, pubkey, created_at, kind, tags, content, sig)
		VALUES ('bad-empty', 'aabbccdd', $1, 1, '[["expiration",""]]'::jsonb, '', 'deadbeef')
	`, now-7200)
	if err != nil {
		t.Fatalf("insert bad-empty: %v", err)
	}

	sw := New(db, Config{
		DryRun:        true,
		GiftWrapGrace: 30 * 24 * time.Hour,
		BatchSize:     1000,
	})

	result, err := sw.Sweep(context.Background())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}

	if result.WouldDelete != 1 {
		t.Errorf("WouldDelete = %d, want 1 (only good-expired)", result.WouldDelete)
	}
	if result.Deleted != 0 {
		t.Errorf("dry-run Deleted = %d, want 0", result.Deleted)
	}
	if result.Malformed != 3 {
		t.Errorf("Malformed = %d, want 3", result.Malformed)
	}
}

func TestSweep_BatchSize(t *testing.T) {
	db := testDB(t)
	defer func() { _ = db.Close() }()
	setupEventTable(t, db)

	now := time.Now().Unix()
	past := fmt.Sprintf("%d", now-3600)

	for i := 0; i < 25; i++ {
		insertEvent(t, db, fmt.Sprintf("evt-%03d", i), 1, now-7200, past)
	}

	sw := New(db, Config{
		DryRun:        false,
		GiftWrapGrace: 30 * 24 * time.Hour,
		BatchSize:     10,
		BatchPause:    time.Millisecond,
	})

	result, err := sw.Sweep(context.Background())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}

	if result.Deleted != 25 {
		t.Errorf("Deleted = %d, want 25", result.Deleted)
	}
	if n := eventCount(t, db); n != 0 {
		t.Errorf("remaining events = %d, want 0", n)
	}
}

func TestSweep_ContextCancellation(t *testing.T) {
	db := testDB(t)
	defer func() { _ = db.Close() }()
	setupEventTable(t, db)

	now := time.Now().Unix()
	past := fmt.Sprintf("%d", now-3600)

	for i := 0; i < 50; i++ {
		insertEvent(t, db, fmt.Sprintf("evt-%03d", i), 1, now-7200, past)
	}

	ctx, cancel := context.WithCancel(context.Background())

	sw := New(db, Config{
		DryRun:        false,
		GiftWrapGrace: 30 * 24 * time.Hour,
		BatchSize:     10,
		BatchPause:    100 * time.Millisecond,
	})

	// Cancel after a short delay so at least one batch runs but not all
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	result, err := sw.Sweep(ctx)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}

	if result.Deleted >= 50 {
		t.Error("expected cancellation to stop before all 50 were deleted")
	}
	t.Logf("deleted %d of 50 before cancellation", result.Deleted)
}

func TestSweep_DryRun_SeparateGiftWrapCount(t *testing.T) {
	db := testDB(t)
	defer func() { _ = db.Close() }()
	setupEventTable(t, db)

	now := time.Now().Unix()
	graceSeconds := int64(30 * 86400)
	oldExpiry := fmt.Sprintf("%d", now-graceSeconds-86400)
	regularExpiry := fmt.Sprintf("%d", now-3600)

	insertEvent(t, db, "wrap-1", kindGiftWrap, now-7200, oldExpiry)
	insertEvent(t, db, "wrap-2", kindGiftWrap, now-7200, oldExpiry)
	insertEvent(t, db, "note-1", 1, now-7200, regularExpiry)
	insertEvent(t, db, "note-2", 1, now-7200, regularExpiry)
	insertEvent(t, db, "note-3", 1, now-7200, regularExpiry)

	sw := New(db, Config{
		DryRun:        true,
		GiftWrapGrace: 30 * 24 * time.Hour,
		BatchSize:     1000,
	})

	result, err := sw.Sweep(context.Background())
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}

	kindMap := make(map[int]int64)
	for _, kc := range result.Counts {
		kindMap[kc.Kind] = kc.Count
	}

	if kindMap[kindGiftWrap] != 2 {
		t.Errorf("kind 1059 count = %d, want 2", kindMap[kindGiftWrap])
	}
	if kindMap[1] != 3 {
		t.Errorf("kind 1 count = %d, want 3", kindMap[1])
	}
}

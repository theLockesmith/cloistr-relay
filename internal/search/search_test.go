package search

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	_ "github.com/lib/pq"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5450/nostr_test?sslmode=disable"
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

func setupSearchTable(t *testing.T, db *sql.DB) {
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

// setupRaceTable creates the event table with the search_tsv column
// pre-populated so that CREATE INDEX CONCURRENTLY takes measurable time,
// widening the race window between concurrent InitSchema calls.
func setupRaceTable(t *testing.T, db *sql.DB, rows int) {
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
			search_tsv tsvector,
			tagvalues  text[] GENERATED ALWAYS AS (tags_to_tagvalues(tags)) STORED
		);
		CREATE UNIQUE INDEX IF NOT EXISTS ididx ON event USING btree (id text_pattern_ops);
	`)
	if err != nil {
		t.Fatalf("setup race table: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO event (id, pubkey, created_at, kind, tags, content, sig, search_tsv)
		SELECT 'r-'||i::text, 'aa', 1, 1, '[]'::jsonb,
		       'searchable content number '||i::text, 'bb',
		       to_tsvector('english', 'searchable content number '||i::text)
		FROM generate_series(1, $1) AS s(i)
	`, rows)
	if err != nil {
		t.Fatalf("populate race table with %d rows: %v", rows, err)
	}

	t.Cleanup(func() {
		_, _ = db.Exec("DROP INDEX CONCURRENTLY IF EXISTS search_tsv_idx")
		_, _ = db.Exec("DROP TABLE IF EXISTS event")
	})
}

func insertSearchEvent(t *testing.T, db *sql.DB, id string, kind int, content string) {
	t.Helper()
	now := time.Now().Unix()
	_, err := db.Exec(`
		INSERT INTO event (id, pubkey, created_at, kind, tags, content, sig)
		VALUES ($1, 'aabbccdd', $2, $3, '[]'::jsonb, $4, 'deadbeef')
	`, id, now, kind, content)
	if err != nil {
		t.Fatalf("insert event %s: %v", id, err)
	}
}

func hasSearchVector(t *testing.T, db *sql.DB, id string) bool {
	t.Helper()
	var notNull bool
	err := db.QueryRow("SELECT search_tsv IS NOT NULL FROM event WHERE id = $1", id).Scan(&notNull)
	if err != nil {
		t.Fatalf("check search_tsv for %s: %v", id, err)
	}
	return notNull
}

// TestInitSchema_TwoReplicaRace runs two concurrent InitSchema calls against
// one database, simulating two relay replicas starting simultaneously.
//
// Without the advisory lock, both replicas reach the CIC (CREATE INDEX
// CONCURRENTLY) step at roughly the same time: one gets "relation already
// exists" and returns an error, leaving that replica with search silently OFF.
//
// With the advisory lock, the calls serialize: the first replica builds the
// index, the second sees it valid and skips. Both succeed.
func TestInitSchema_TwoReplicaRace(t *testing.T) {
	db := testDB(t)
	defer func() { _ = db.Close() }()

	setupRaceTable(t, db, 50000)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	sb1 := NewSearchBackend(db, DefaultSearchKinds)
	sb2 := NewSearchBackend(db, DefaultSearchKinds)

	var wg sync.WaitGroup
	errs := make([]error, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		errs[0] = sb1.InitSchema(ctx)
	}()
	go func() {
		defer wg.Done()
		errs[1] = sb2.InitSchema(ctx)
	}()
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("replica %d failed: %v", i, err)
		}
	}

	var valid bool
	if err := db.QueryRow(`
		SELECT COALESCE(
			(SELECT i.indisvalid FROM pg_class c
			 JOIN pg_index i ON i.indexrelid = c.oid
			 WHERE c.relname = 'search_tsv_idx'),
			false)
	`).Scan(&valid); err != nil {
		t.Fatalf("check index validity: %v", err)
	}
	if !valid {
		t.Error("search_tsv_idx must be valid after both replicas complete")
	}
}

func TestInitSchema_CreatesColumnAndTrigger(t *testing.T) {
	db := testDB(t)
	defer func() { _ = db.Close() }()
	setupSearchTable(t, db)

	sb := NewSearchBackend(db, DefaultSearchKinds)
	if err := sb.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	var colExists bool
	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'event' AND column_name = 'search_tsv'
		)
	`).Scan(&colExists)
	if err != nil {
		t.Fatalf("check column: %v", err)
	}
	if !colExists {
		t.Error("search_tsv column should exist after InitSchema")
	}

	var trigExists bool
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.triggers
			WHERE trigger_name = 'search_tsv_update' AND event_object_table = 'event'
		)
	`).Scan(&trigExists)
	if err != nil {
		t.Fatalf("check trigger: %v", err)
	}
	if !trigExists {
		t.Error("search_tsv_update trigger should exist after InitSchema")
	}
}

func TestEncryptedKindNeverIndexed(t *testing.T) {
	db := testDB(t)
	defer func() { _ = db.Close() }()
	setupSearchTable(t, db)

	sb := NewSearchBackend(db, DefaultSearchKinds)
	if err := sb.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	insertSearchEvent(t, db, "public-note", 1, "hello world searchable text")
	insertSearchEvent(t, db, "reply-11", 11, "a reply to a note")
	insertSearchEvent(t, db, "comment-1111", 1111, "a comment on something")
	insertSearchEvent(t, db, "giftwrap-1059", 1059, "encrypted ciphertext base64 stuff")
	insertSearchEvent(t, db, "dm-4", 4, "private direct message content")
	insertSearchEvent(t, db, "reaction-7", 7, "+")
	insertSearchEvent(t, db, "article-30023", 30023, "long form article about nostr")

	if !hasSearchVector(t, db, "public-note") {
		t.Error("kind 1 (public note) should have search_tsv")
	}
	if !hasSearchVector(t, db, "reply-11") {
		t.Error("kind 11 (reply) should have search_tsv")
	}
	if !hasSearchVector(t, db, "comment-1111") {
		t.Error("kind 1111 (comment) should have search_tsv")
	}
	if !hasSearchVector(t, db, "article-30023") {
		t.Error("kind 30023 (article) should have search_tsv")
	}
	if hasSearchVector(t, db, "giftwrap-1059") {
		t.Error("kind 1059 (gift wrap) must NOT have search_tsv")
	}
	if hasSearchVector(t, db, "dm-4") {
		t.Error("kind 4 (DM) must NOT have search_tsv")
	}
	if hasSearchVector(t, db, "reaction-7") {
		t.Error("kind 7 (reaction) must NOT have search_tsv")
	}
}

func TestQueryEvents_OnlySearchableKinds(t *testing.T) {
	db := testDB(t)
	defer func() { _ = db.Close() }()
	setupSearchTable(t, db)

	sb := NewSearchBackend(db, DefaultSearchKinds)
	if err := sb.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	insertSearchEvent(t, db, "note-hello", 1, "hello world from nostr")
	insertSearchEvent(t, db, "wrap-hello", 1059, "hello world from nostr")
	insertSearchEvent(t, db, "note-goodbye", 1, "goodbye from nostr")

	filter := nostr.Filter{
		Search: "hello",
		Limit:  100,
	}

	ch, err := sb.QueryEvents(context.Background(), filter)
	if err != nil {
		t.Fatalf("QueryEvents: %v", err)
	}

	var results []string
	for evt := range ch {
		results = append(results, evt.ID)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d: %v", len(results), results)
	}
	if len(results) == 1 && results[0] != "note-hello" {
		t.Errorf("expected note-hello, got %s", results[0])
	}
}

func TestBackfill_PopulatesExistingEvents(t *testing.T) {
	db := testDB(t)
	defer func() { _ = db.Close() }()
	setupSearchTable(t, db)

	insertSearchEvent(t, db, "pre-existing", 1, "pre-existing event content")
	insertSearchEvent(t, db, "pre-encrypted", 1059, "encrypted content here")

	sb := NewSearchBackend(db, DefaultSearchKinds)
	if err := sb.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	n, err := sb.Backfill(context.Background())
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}

	if n != 1 {
		t.Errorf("backfilled %d events, want 1 (only kind 1)", n)
	}

	if !hasSearchVector(t, db, "pre-existing") {
		t.Error("pre-existing (kind 1) should have search_tsv after backfill")
	}
	if hasSearchVector(t, db, "pre-encrypted") {
		t.Error("pre-encrypted (kind 1059) must NOT have search_tsv after backfill")
	}
}

func TestClearNonSearchable_RemovesVectors(t *testing.T) {
	db := testDB(t)
	defer func() { _ = db.Close() }()
	setupSearchTable(t, db)

	sb := NewSearchBackend(db, DefaultSearchKinds)
	if err := sb.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	insertSearchEvent(t, db, "note-ok", 1, "searchable note")
	insertSearchEvent(t, db, "reaction-stale", 7, "reaction text")

	// Disable the trigger temporarily to simulate a stale vector from before
	// the scoped trigger existed (migration scenario).
	_, err := db.Exec("ALTER TABLE event DISABLE TRIGGER search_tsv_update")
	if err != nil {
		t.Fatalf("disable trigger: %v", err)
	}
	_, err = db.Exec("UPDATE event SET search_tsv = to_tsvector('english', content) WHERE id = 'reaction-stale'")
	if err != nil {
		t.Fatalf("manually set vector: %v", err)
	}
	_, err = db.Exec("ALTER TABLE event ENABLE TRIGGER search_tsv_update")
	if err != nil {
		t.Fatalf("enable trigger: %v", err)
	}

	if !hasSearchVector(t, db, "reaction-stale") {
		t.Fatal("reaction-stale should have a vector before clear")
	}

	n, err := sb.ClearNonSearchable(context.Background())
	if err != nil {
		t.Fatalf("ClearNonSearchable: %v", err)
	}

	if n != 1 {
		t.Errorf("cleared %d events, want 1", n)
	}
	if hasSearchVector(t, db, "reaction-stale") {
		t.Error("reaction-stale should NOT have search_tsv after clear")
	}
	if !hasSearchVector(t, db, "note-ok") {
		t.Error("note-ok should still have search_tsv after clear")
	}
}

func TestDropsLegacyIndex(t *testing.T) {
	db := testDB(t)
	defer func() { _ = db.Close() }()
	setupSearchTable(t, db)

	_, err := db.Exec(`
		CREATE INDEX content_search_idx ON event USING GIN (to_tsvector('english', content))
	`)
	if err != nil {
		t.Fatalf("create legacy index: %v", err)
	}

	sb := NewSearchBackend(db, DefaultSearchKinds)
	if err := sb.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	var legacyExists bool
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes WHERE indexname = 'content_search_idx'
		)
	`).Scan(&legacyExists)
	if err != nil {
		t.Fatalf("check legacy index: %v", err)
	}
	if legacyExists {
		t.Error("content_search_idx should have been dropped by InitSchema")
	}
}

func TestToTSQuery(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello:*"},
		{"hello world", "hello:* | world:*"},
		{"", ""},
		{"hello! @world", "hello:* | world:*"},
		{"  spaces  ", "spaces:*"},
		{"!!! ???", ""},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.input), func(t *testing.T) {
			got := toTSQuery(tt.input)
			if got != tt.want {
				t.Errorf("toTSQuery(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestQueryEvents_AllSymbolSearchReturnsEmpty(t *testing.T) {
	db := testDB(t)
	defer func() { _ = db.Close() }()
	setupSearchTable(t, db)

	sb := NewSearchBackend(db, DefaultSearchKinds)
	if err := sb.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	insertSearchEvent(t, db, "note-1", 1, "hello world")

	filter := nostr.Filter{Search: "!!! ???", Limit: 100}
	ch, err := sb.QueryEvents(context.Background(), filter)
	if err != nil {
		t.Fatalf("QueryEvents: %v", err)
	}

	var count int
	for range ch {
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 results for all-symbol search, got %d", count)
	}
}

func TestTrigger_UpdatesVectorOnUpsert(t *testing.T) {
	db := testDB(t)
	defer func() { _ = db.Close() }()
	setupSearchTable(t, db)

	sb := NewSearchBackend(db, DefaultSearchKinds)
	if err := sb.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	insertSearchEvent(t, db, "article-1", 30023, "original article content")

	filter := nostr.Filter{Search: "original", Limit: 100}
	ch, _ := sb.QueryEvents(context.Background(), filter)
	var found bool
	for range ch {
		found = true
	}
	if !found {
		t.Fatal("article should be findable by 'original'")
	}

	_, err := db.Exec(`
		UPDATE event SET content = 'completely rewritten article' WHERE id = 'article-1'
	`)
	if err != nil {
		t.Fatalf("update event: %v", err)
	}

	ch, _ = sb.QueryEvents(context.Background(), nostr.Filter{Search: "original", Limit: 100})
	var staleCount int
	for range ch {
		staleCount++
	}
	if staleCount != 0 {
		t.Error("after update, 'original' should no longer match (stale vector)")
	}

	ch, _ = sb.QueryEvents(context.Background(), nostr.Filter{Search: "rewritten", Limit: 100})
	var newCount int
	for range ch {
		newCount++
	}
	if newCount != 1 {
		t.Errorf("after update, 'rewritten' should match, got %d results", newCount)
	}
}

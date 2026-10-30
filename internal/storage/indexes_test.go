package storage_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fiatjaf/eventstore/postgresql"

	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/storage"
)

// indexTestDB gives each test its own schema holding a freshly initialised
// eventstore event table, so index changes here never touch the event table
// other tests share. The returned DB has search_path set to that schema.
func indexTestDB(t *testing.T) *sql.DB {
	t.Helper()
	base := testDBURL(t)

	admin, err := sql.Open("postgres", base)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })

	schema := fmt.Sprintf("idxtest_%d", time.Now().UnixNano())
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`) })

	url := base + "&search_path=" + schema
	backend := &postgresql.PostgresBackend{DatabaseURL: url}
	if err := backend.Init(); err != nil {
		t.Fatalf("eventstore init: %v", err)
	}
	t.Cleanup(backend.Close)

	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatalf("open schema db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func indexExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var exists bool
	if err := db.QueryRow(`SELECT to_regclass($1) IS NOT NULL`, name).Scan(&exists); err != nil {
		t.Fatalf("check index %s: %v", name, err)
	}
	return exists
}

// createOldDuplicates recreates the two indexes earlier versions of
// OptimizeIndexes built, as production has them today.
func createOldDuplicates(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, stmt := range []string{
		`CREATE INDEX IF NOT EXISTS event_kind_created_at_idx ON event (kind, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS event_tagvalues_gin_idx ON event USING GIN (tagvalues)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create old duplicate: %v", err)
		}
	}
}

func TestOptimizeIndexes_DropsDuplicatesOfEventstoreIndexes(t *testing.T) {
	db := indexTestDB(t)
	createOldDuplicates(t, db)

	// Twice: the second run proves the duplicates are not recreated.
	for run := 1; run <= 2; run++ {
		if err := storage.OptimizeIndexes(db); err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		for _, dup := range []string{"event_kind_created_at_idx", "event_tagvalues_gin_idx"} {
			if indexExists(t, db, dup) {
				t.Errorf("run %d: duplicate index %s still exists", run, dup)
			}
		}
		for _, twin := range []string{"kindtimeidx", "arbitrarytagvalues"} {
			if !indexExists(t, db, twin) {
				t.Errorf("run %d: eventstore index %s is gone", run, twin)
			}
		}
	}
}

func TestOptimizeIndexes_KeepsDuplicateWhenTwinMissing(t *testing.T) {
	db := indexTestDB(t)
	createOldDuplicates(t, db)
	if _, err := db.Exec(`DROP INDEX arbitrarytagvalues`); err != nil {
		t.Fatalf("drop twin: %v", err)
	}

	if err := storage.OptimizeIndexes(db); err != nil {
		t.Fatal(err)
	}
	if !indexExists(t, db, "event_tagvalues_gin_idx") {
		t.Error("event_tagvalues_gin_idx was dropped although arbitrarytagvalues is missing: no tag index left")
	}
	if indexExists(t, db, "event_kind_created_at_idx") {
		t.Error("event_kind_created_at_idx kept although kindtimeidx is present")
	}
}

func TestOptimizeIndexes_KeepsDuplicateWhenTwinInvalid(t *testing.T) {
	db := indexTestDB(t)
	createOldDuplicates(t, db)
	// What a failed CREATE INDEX CONCURRENTLY leaves behind.
	if _, err := db.Exec(`UPDATE pg_index SET indisvalid = false WHERE indexrelid = to_regclass('kindtimeidx')`); err != nil {
		// Editing pg_index needs superuser, which CI's postgres role has.
		t.Skipf("cannot invalidate twin (needs superuser): %v", err)
	}

	if err := storage.OptimizeIndexes(db); err != nil {
		t.Fatal(err)
	}
	if !indexExists(t, db, "event_kind_created_at_idx") {
		t.Error("event_kind_created_at_idx was dropped although kindtimeidx is invalid")
	}
}

// TestRemainingIndexesServeEventstoreQueries is the query-plan proof that the
// eventstore indexes left after the drop serve the queries the duplicates did.
// Each query is planned with the duplicates present and again after
// OptimizeIndexes drops them; the plans must match once the duplicate's name
// is read as its twin's, and must use the twin.
//
// The SQL is the shape eventstore's queryEventsSql emits for a kinds filter
// and a tag filter (postgresql/query.go). Sequential scans are switched off so
// the planner reports which index it would use, not that a small table is
// cheaper to read whole. The kinds query asks for a rare kind, the case a
// kind index exists for; a common kind is cheaper to find by scanning timeidx.
func TestRemainingIndexesServeEventstoreQueries(t *testing.T) {
	db := indexTestDB(t)
	// Build every other index first, so the drop is the only difference
	// between the two plans.
	if err := storage.OptimizeIndexes(db); err != nil {
		t.Fatal(err)
	}
	createOldDuplicates(t, db)

	if _, err := db.Exec(`
		INSERT INTO event (id, pubkey, created_at, kind, tags, content, sig)
		SELECT md5(g::text), md5((g % 50)::text), 1700000000 + g,
		       CASE WHEN g % 1000 = 0 THEN 1111 ELSE g % 40 END,
		       jsonb_build_array(jsonb_build_array('t', 'tag' || (g % 200))), '', ''
		FROM generate_series(1, 20000) g`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Exec(`ANALYZE event`); err != nil {
		t.Fatalf("analyze: %v", err)
	}

	ctx := context.Background()
	cases := []struct {
		name  string
		query string
		args  []any
		dup   string
		want  string
	}{
		{
			name: "kinds filter",
			query: `SELECT id, pubkey, created_at, kind, tags, content, sig FROM event
				WHERE kind IN ($1, $2) ORDER BY created_at DESC, id LIMIT $3`,
			args: []any{1111, 1112, 100},
			dup:  "event_kind_created_at_idx",
			want: "kindtimeidx",
		},
		{
			name: "tag filter",
			query: `SELECT id, pubkey, created_at, kind, tags, content, sig FROM event
				WHERE tagvalues && ARRAY[$1] ORDER BY created_at DESC, id LIMIT $2`,
			args: []any{"t:tag7", 100},
			dup:  "event_tagvalues_gin_idx",
			want: "arbitrarytagvalues",
		},
	}

	explain := func(query string, args []any) string {
		t.Helper()
		conn, err := db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = conn.Close() }()
		if _, err := conn.ExecContext(ctx, `SET enable_seqscan = off`); err != nil {
			t.Fatal(err)
		}
		rows, err := conn.QueryContext(ctx, `EXPLAIN (COSTS OFF) `+query, args...)
		if err != nil {
			t.Fatalf("explain: %v", err)
		}
		defer func() { _ = rows.Close() }()
		var plan []string
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				t.Fatal(err)
			}
			plan = append(plan, line)
		}
		return strings.Join(plan, "\n")
	}

	before := make([]string, len(cases))
	for i, tc := range cases {
		before[i] = explain(tc.query, tc.args)
	}

	if err := storage.OptimizeIndexes(db); err != nil {
		t.Fatal(err)
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			after := explain(tc.query, tc.args)
			t.Logf("plan with duplicates:\n%s\nplan after drop:\n%s", before[i], after)
			if !strings.Contains(after, tc.want) {
				t.Errorf("plan after drop does not use %s", tc.want)
			}
			if !strings.Contains(before[i], tc.dup) {
				// Twin and duplicate are identical, so the planner may already
				// pick the twin; the plans must then simply be equal.
				t.Logf("planner chose %s even with %s present", tc.want, tc.dup)
			}
			if strings.ReplaceAll(before[i], tc.dup, tc.want) != after {
				t.Errorf("plan changed beyond the index name")
			}
		})
	}
}

// TestOptimizeIndexes_DropDoesNotBlockWrites proves the drop is concurrent.
// A plain DROP INDEX needs an exclusive lock on the event table: while it waits
// behind any open transaction, every insert queues behind it, which on the
// live relay is every event. DROP INDEX CONCURRENTLY waits for that
// transaction too, but lets inserts through meanwhile.
func TestOptimizeIndexes_DropDoesNotBlockWrites(t *testing.T) {
	db := indexTestDB(t)
	if err := storage.OptimizeIndexes(db); err != nil {
		t.Fatal(err)
	}
	createOldDuplicates(t, db)

	ctx := context.Background()
	insert := func(ex interface {
		ExecContext(context.Context, string, ...any) (sql.Result, error)
	}, id string) error {
		_, err := ex.ExecContext(ctx, `INSERT INTO event (id, pubkey, created_at, kind, tags, content, sig)
			VALUES ($1, 'pk', 1, 1, '[]', '', '')`, id)
		return err
	}

	// An open writing transaction, as the relay always has in flight.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := insert(tx, "in-flight"); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- storage.OptimizeIndexes(db) }()

	// Wait until the drop is parked behind the open transaction.
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting bool
		if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM pg_stat_activity
			WHERE query LIKE 'DROP INDEX%' AND wait_event_type = 'Lock')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("drop never started waiting on the open transaction")
		}
		time.Sleep(20 * time.Millisecond)
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, `SET lock_timeout = '1s'`); err != nil {
		t.Fatal(err)
	}
	if err := insert(conn, "during-drop"); err != nil {
		t.Errorf("insert blocked while the duplicate index was being dropped: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("OptimizeIndexes did not finish after the transaction committed")
	}
	for _, dup := range []string{"event_kind_created_at_idx", "event_tagvalues_gin_idx"} {
		if indexExists(t, db, dup) {
			t.Errorf("duplicate index %s still exists", dup)
		}
	}
}

package search

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var DefaultSearchKinds = []int{1, 11, 1111, 30023}

const backfillBatchSize = 10000

// searchLockKey serializes InitSchema and Backfill across replicas so two pods
// starting simultaneously cannot race on CREATE INDEX CONCURRENTLY or duplicate
// backfill work. Distinct from the main DDL lock key in storage/ddllock.go.
const searchLockKey int64 = 0x636C73726368 // "clsrch"

var SearchSetupFailures = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: "cloistr",
	Subsystem: "search",
	Name:      "setup_failures_total",
	Help:      "Number of times search schema or backfill setup failed.",
})

type SearchBackend struct {
	db    *sql.DB
	kinds []int
}

func NewSearchBackend(db *sql.DB, kinds []int) *SearchBackend {
	if len(kinds) == 0 {
		kinds = DefaultSearchKinds
	}
	return &SearchBackend{db: db, kinds: kinds}
}

// acquireSearchLock polls pg_try_advisory_lock on a dedicated connection.
// Polling (not blocking) avoids the snapshot-holds-CIC-open deadlock that
// blocking pg_advisory_lock causes: a blocked waiter's snapshot prevents the
// holder's CREATE INDEX CONCURRENTLY from completing.
func acquireSearchLock(ctx context.Context, db *sql.DB) (*sql.Conn, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("search lock connection: %w", err)
	}

	start := time.Now()
	lastLog := start
	for {
		var acquired bool
		if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", searchLockKey).Scan(&acquired); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("pg_try_advisory_lock (search): %w", err)
		}
		if acquired {
			return conn, nil
		}
		if ctx.Err() != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("search lock timeout after %v: %w", time.Since(start).Round(time.Second), ctx.Err())
		}
		if time.Since(lastLog) >= 30*time.Second {
			log.Printf("Waiting for search advisory lock (held by another replica), %vs elapsed", int(time.Since(start).Seconds()))
			lastLog = time.Now()
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func releaseSearchLock(conn *sql.Conn) {
	var released bool
	if err := conn.QueryRowContext(context.Background(), "SELECT pg_advisory_unlock($1)", searchLockKey).Scan(&released); err != nil {
		log.Printf("search advisory unlock failed: %v", err)
	}
	_ = conn.Close()
}

func (s *SearchBackend) InitSchema(ctx context.Context) error {
	lockConn, err := acquireSearchLock(ctx, s.db)
	if err != nil {
		SearchSetupFailures.Inc()
		return fmt.Errorf("search InitSchema: %w", err)
	}
	defer releaseSearchLock(lockConn)

	kindsSQL := kindsList(s.kinds)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		SearchSetupFailures.Inc()
		return fmt.Errorf("begin DDL transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `SET LOCAL lock_timeout = '5s'`); err != nil {
		SearchSetupFailures.Inc()
		return fmt.Errorf("set lock_timeout: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DROP INDEX IF EXISTS content_search_idx`); err != nil {
		SearchSetupFailures.Inc()
		return fmt.Errorf("drop legacy index: %w", err)
	}

	// ADD COLUMN and CREATE TRIGGER lock the table even when there is nothing
	// to do, so they wait behind any open read (a long REQ, the expiry
	// sweeper's scan) and hit lock_timeout. Run them only when missing.
	var colExists, trigExists bool
	if err := tx.QueryRowContext(ctx, `
		SELECT
			EXISTS (SELECT 1 FROM information_schema.columns
			        WHERE table_name = 'event' AND column_name = 'search_tsv'),
			EXISTS (SELECT 1 FROM pg_trigger
			        WHERE tgname = 'search_tsv_update' AND tgrelid = 'event'::regclass)
	`).Scan(&colExists, &trigExists); err != nil {
		SearchSetupFailures.Inc()
		return fmt.Errorf("check search schema: %w", err)
	}

	if !colExists {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE event ADD COLUMN IF NOT EXISTS search_tsv tsvector`); err != nil {
			SearchSetupFailures.Inc()
			return fmt.Errorf("add search_tsv column: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION search_tsv_trigger() RETURNS trigger AS $$
		BEGIN
			IF NEW.kind IN (%s) THEN
				NEW.search_tsv := to_tsvector('english', NEW.content);
			ELSE
				NEW.search_tsv := NULL;
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql
	`, kindsSQL)); err != nil {
		SearchSetupFailures.Inc()
		return fmt.Errorf("create trigger function: %w", err)
	}

	// The function above is replaced in place (no table lock), so a changed
	// kinds list takes effect without recreating the trigger.
	if !trigExists {
		if _, err := tx.ExecContext(ctx, `
			CREATE OR REPLACE TRIGGER search_tsv_update
			BEFORE INSERT OR UPDATE ON event
			FOR EACH ROW
			EXECUTE FUNCTION search_tsv_trigger()
		`); err != nil {
			SearchSetupFailures.Inc()
			return fmt.Errorf("create trigger: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		SearchSetupFailures.Inc()
		return fmt.Errorf("commit DDL: %w", err)
	}

	// CREATE INDEX CONCURRENTLY cannot run inside a transaction.
	// Check for a valid index; drop any invalid leftover from a prior failed CIC.
	var indexValid bool
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(
			(SELECT i.indisvalid FROM pg_class c
			 JOIN pg_index i ON i.indexrelid = c.oid
			 WHERE c.relname = 'search_tsv_idx'),
			false)
	`).Scan(&indexValid); err != nil {
		SearchSetupFailures.Inc()
		return fmt.Errorf("check search_tsv_idx: %w", err)
	}
	if !indexValid {
		// DROP INDEX CONCURRENTLY avoids ACCESS EXCLUSIVE on the table.
		if _, err := s.db.ExecContext(ctx, `DROP INDEX CONCURRENTLY IF EXISTS search_tsv_idx`); err != nil {
			SearchSetupFailures.Inc()
			return fmt.Errorf("drop invalid search index: %w", err)
		}
		if _, err := s.db.ExecContext(ctx, `
			CREATE INDEX CONCURRENTLY search_tsv_idx
			ON event USING GIN (search_tsv)
			WHERE search_tsv IS NOT NULL
		`); err != nil {
			SearchSetupFailures.Inc()
			return fmt.Errorf("create search_tsv index: %w", err)
		}
	}

	log.Printf("NIP-50 search initialized (indexed kinds: %s)", kindsSQL)
	return nil
}

// InitSchemaRetry runs InitSchema until it succeeds or ctx ends, waiting
// every between attempts. A lock timeout on a busy table is transient; giving
// up after one attempt would leave search off until the next restart.
func (s *SearchBackend) InitSchemaRetry(ctx context.Context, every time.Duration) (int, error) {
	for attempt := 1; ; attempt++ {
		err := s.InitSchema(ctx)
		if err == nil {
			return attempt, nil
		}
		log.Printf("ERROR: search InitSchema attempt %d failed, retrying in %v: %v", attempt, every, err)
		select {
		case <-ctx.Done():
			return attempt, fmt.Errorf("search InitSchema gave up after %d attempts: %w", attempt, err)
		case <-time.After(every):
		}
	}
}

func (s *SearchBackend) Backfill(ctx context.Context) (int64, error) {
	kindsSQL := kindsList(s.kinds)
	var total int64
	lastID := ""
	for {
		var n int64
		var maxID sql.NullString
		err := s.db.QueryRowContext(ctx, fmt.Sprintf(`
			WITH batch AS (
				SELECT id FROM event
				WHERE kind IN (%s) AND search_tsv IS NULL AND id > $1
				ORDER BY id
				LIMIT %d
			),
			updated AS (
				UPDATE event e SET search_tsv = to_tsvector('english', e.content)
				FROM batch WHERE e.id = batch.id
				RETURNING e.id
			)
			SELECT count(*), max(id) FROM updated
		`, kindsSQL, backfillBatchSize), lastID).Scan(&n, &maxID)
		if err != nil {
			SearchSetupFailures.Inc()
			return total, err
		}
		total += n
		if n == 0 || !maxID.Valid {
			break
		}
		lastID = maxID.String
		if n < int64(backfillBatchSize) {
			break
		}
	}
	log.Printf("NIP-50 search: backfill complete, %d rows updated", total)
	return total, nil
}

func (s *SearchBackend) ClearNonSearchable(ctx context.Context) (int64, error) {
	kindsSQL := kindsList(s.kinds)
	res, err := s.db.ExecContext(ctx, fmt.Sprintf(`
		UPDATE event
		SET search_tsv = NULL
		WHERE kind NOT IN (%s) AND search_tsv IS NOT NULL
	`, kindsSQL))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		log.Printf("NIP-50 search: cleared %d non-searchable vectors", n)
	}
	return n, nil
}

func (s *SearchBackend) QueryEvents(ctx context.Context, filter nostr.Filter) (chan *nostr.Event, error) {
	ch := make(chan *nostr.Event)

	go func() {
		defer close(ch)

		searchTerm := filter.Search
		if searchTerm == "" {
			return
		}

		tsquery := toTSQuery(searchTerm)
		if tsquery == "" {
			return
		}

		query, args := s.buildSearchQuery(filter, tsquery)

		rows, err := s.db.QueryContext(ctx, query, args...)
		if err != nil {
			log.Printf("Search query error: %v", err)
			return
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var (
				id        string
				pubkey    string
				createdAt int64
				kind      int
				tagsJSON  []byte
				content   string
				sig       string
			)

			if err := rows.Scan(&id, &pubkey, &createdAt, &kind, &tagsJSON, &content, &sig); err != nil {
				log.Printf("Row scan error: %v", err)
				continue
			}

			var tags nostr.Tags
			if err := json.Unmarshal(tagsJSON, &tags); err != nil {
				log.Printf("Tags unmarshal error: %v", err)
				continue
			}

			event := &nostr.Event{
				ID:        id,
				PubKey:    pubkey,
				CreatedAt: nostr.Timestamp(createdAt),
				Kind:      kind,
				Tags:      tags,
				Content:   content,
				Sig:       sig,
			}

			select {
			case ch <- event:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

func (s *SearchBackend) buildSearchQuery(filter nostr.Filter, tsquery string) (string, []any) {
	var conditions []string
	var args []any
	argNum := 1

	conditions = append(conditions, "search_tsv @@ to_tsquery('english', $"+strconv.Itoa(argNum)+")")
	args = append(args, tsquery)
	argNum++

	if len(filter.Kinds) > 0 {
		kindPlaceholders := make([]string, len(filter.Kinds))
		for i, k := range filter.Kinds {
			kindPlaceholders[i] = "$" + strconv.Itoa(argNum)
			args = append(args, k)
			argNum++
		}
		conditions = append(conditions, "kind IN ("+strings.Join(kindPlaceholders, ",")+")")
	}

	if len(filter.Authors) > 0 {
		authorPlaceholders := make([]string, len(filter.Authors))
		for i, a := range filter.Authors {
			authorPlaceholders[i] = "$" + strconv.Itoa(argNum)
			args = append(args, a)
			argNum++
		}
		conditions = append(conditions, "pubkey IN ("+strings.Join(authorPlaceholders, ",")+")")
	}

	if filter.Since != nil {
		conditions = append(conditions, "created_at >= $"+strconv.Itoa(argNum))
		args = append(args, int64(*filter.Since))
		argNum++
	}
	if filter.Until != nil {
		conditions = append(conditions, "created_at <= $"+strconv.Itoa(argNum))
		args = append(args, int64(*filter.Until))
		argNum++
	}

	query := `
		SELECT id, pubkey, created_at, kind, tags, content, sig
		FROM event
		WHERE ` + strings.Join(conditions, " AND ") + `
		ORDER BY ts_rank(search_tsv, to_tsquery('english', $1)) DESC
	`

	limit := 100
	if filter.Limit > 0 && filter.Limit < limit {
		limit = filter.Limit
	}
	query += " LIMIT $" + strconv.Itoa(argNum)
	args = append(args, limit)

	return query, args
}

// toTSQuery converts a search string to PostgreSQL tsquery format.
// Uses prefix matching with OR semantics (orchestrator directive: keep current OR).
func toTSQuery(search string) string {
	words := strings.Fields(search)
	if len(words) == 0 {
		return ""
	}

	escaped := make([]string, 0, len(words))
	for _, word := range words {
		clean := strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
				return r
			}
			return -1
		}, word)
		if clean != "" {
			escaped = append(escaped, clean+":*")
		}
	}

	return strings.Join(escaped, " | ")
}

func HasSearch(filter nostr.Filter) bool {
	return filter.Search != ""
}

func kindsList(kinds []int) string {
	parts := make([]string, len(kinds))
	for i, k := range kinds {
		parts[i] = strconv.Itoa(k)
	}
	return strings.Join(parts, ", ")
}

func (s *SearchBackend) SearchKinds() []int {
	return s.kinds
}

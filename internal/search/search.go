package search

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/nbd-wtf/go-nostr"
)

var DefaultSearchKinds = []int{1, 11, 1111, 30023}

const backfillBatchSize = 10000

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

// InitSchema sets up the search_tsv column, trigger, and partial GIN index.
// Runs under the relay's DDL advisory lock so only one instance executes at a time.
func (s *SearchBackend) InitSchema() error {
	kindsSQL := kindsList(s.kinds)

	_, err := s.db.Exec(`DROP INDEX IF EXISTS content_search_idx`)
	if err != nil {
		return fmt.Errorf("drop legacy index: %w", err)
	}

	_, err = s.db.Exec(`ALTER TABLE event ADD COLUMN IF NOT EXISTS search_tsv tsvector`)
	if err != nil {
		return fmt.Errorf("add search_tsv column: %w", err)
	}

	_, err = s.db.Exec(`
		CREATE INDEX IF NOT EXISTS search_tsv_idx
		ON event USING GIN (search_tsv)
		WHERE search_tsv IS NOT NULL
	`)
	if err != nil {
		return fmt.Errorf("create search_tsv index: %w", err)
	}

	_, err = s.db.Exec(fmt.Sprintf(`
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
	`, kindsSQL))
	if err != nil {
		return fmt.Errorf("create trigger function: %w", err)
	}

	_, err = s.db.Exec(`DROP TRIGGER IF EXISTS search_tsv_update ON event`)
	if err != nil {
		return fmt.Errorf("drop old trigger: %w", err)
	}

	_, err = s.db.Exec(`
		CREATE TRIGGER search_tsv_update
		BEFORE INSERT OR UPDATE ON event
		FOR EACH ROW
		EXECUTE FUNCTION search_tsv_trigger()
	`)
	if err != nil {
		return fmt.Errorf("create trigger: %w", err)
	}

	log.Printf("NIP-50 search initialized (indexed kinds: %s)", kindsSQL)
	return nil
}

func (s *SearchBackend) Backfill() (int64, error) {
	kindsSQL := kindsList(s.kinds)
	var total int64
	for {
		res, err := s.db.Exec(fmt.Sprintf(`
			UPDATE event
			SET search_tsv = to_tsvector('english', content)
			WHERE ctid IN (
				SELECT ctid FROM event
				WHERE kind IN (%s) AND search_tsv IS NULL
				LIMIT %d
			)
		`, kindsSQL, backfillBatchSize))
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += n
		if n < int64(backfillBatchSize) {
			break
		}
	}
	log.Printf("NIP-50 search: backfill complete, %d rows updated", total)
	return total, nil
}

func (s *SearchBackend) ClearNonSearchable() (int64, error) {
	kindsSQL := kindsList(s.kinds)
	res, err := s.db.Exec(fmt.Sprintf(`
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

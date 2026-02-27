package search

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"strings"

	"github.com/nbd-wtf/go-nostr"
)

// SearchBackend provides NIP-50 search functionality using PostgreSQL full-text search
type SearchBackend struct {
	db *sql.DB
}

// NewSearchBackend creates a new search backend with the given database connection
func NewSearchBackend(db *sql.DB) *SearchBackend {
	return &SearchBackend{db: db}
}

// InitSchema creates the full-text search index if it doesn't exist
func (s *SearchBackend) InitSchema() error {
	// Add full-text search index on content column
	_, err := s.db.Exec(`
		CREATE INDEX IF NOT EXISTS content_search_idx
		ON event USING GIN (to_tsvector('english', content))
	`)
	if err != nil {
		return err
	}
	log.Println("NIP-50 search index initialized")
	return nil
}

// QueryEvents searches for events matching the search term
// This should be called when filter.Search is set
func (s *SearchBackend) QueryEvents(ctx context.Context, filter nostr.Filter) (chan *nostr.Event, error) {
	ch := make(chan *nostr.Event)

	go func() {
		defer close(ch)

		// Get the search term from the filter's extension data
		searchTerm := getSearchTerm(filter)
		if searchTerm == "" {
			return
		}

		// Build the search query
		query, args := s.buildSearchQuery(filter, searchTerm)

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

// buildSearchQuery constructs the PostgreSQL full-text search query
func (s *SearchBackend) buildSearchQuery(filter nostr.Filter, searchTerm string) (string, []any) {
	var conditions []string
	var args []any
	argNum := 1

	// Full-text search condition
	// Convert search term to tsquery format
	tsquery := toTSQuery(searchTerm)
	conditions = append(conditions, "to_tsvector('english', content) @@ to_tsquery('english', $"+itoa(argNum)+")")
	args = append(args, tsquery)
	argNum++

	// Add kind filter if specified
	if len(filter.Kinds) > 0 {
		kindPlaceholders := make([]string, len(filter.Kinds))
		for i, k := range filter.Kinds {
			kindPlaceholders[i] = "$" + itoa(argNum)
			args = append(args, k)
			argNum++
		}
		conditions = append(conditions, "kind IN ("+strings.Join(kindPlaceholders, ",")+")")
	}

	// Add author filter if specified
	if len(filter.Authors) > 0 {
		authorPlaceholders := make([]string, len(filter.Authors))
		for i, a := range filter.Authors {
			authorPlaceholders[i] = "$" + itoa(argNum)
			args = append(args, a)
			argNum++
		}
		conditions = append(conditions, "pubkey IN ("+strings.Join(authorPlaceholders, ",")+")")
	}

	// Add time bounds
	if filter.Since != nil {
		conditions = append(conditions, "created_at >= $"+itoa(argNum))
		args = append(args, int64(*filter.Since))
		argNum++
	}
	if filter.Until != nil {
		conditions = append(conditions, "created_at <= $"+itoa(argNum))
		args = append(args, int64(*filter.Until))
		argNum++
	}

	// Build query with relevance ordering
	query := `
		SELECT id, pubkey, created_at, kind, tags, content, sig
		FROM event
		WHERE ` + strings.Join(conditions, " AND ") + `
		ORDER BY ts_rank(to_tsvector('english', content), to_tsquery('english', $1)) DESC
	`

	// Add limit
	limit := 100
	if filter.Limit > 0 && filter.Limit < limit {
		limit = filter.Limit
	}
	query += " LIMIT $" + itoa(argNum)
	args = append(args, limit)

	return query, args
}

// toTSQuery converts a search string to PostgreSQL tsquery format
func toTSQuery(search string) string {
	// Split into words and join with & for AND semantics
	words := strings.Fields(search)
	if len(words) == 0 {
		return ""
	}

	// Escape and join words
	escaped := make([]string, len(words))
	for i, word := range words {
		// Remove special characters and escape
		clean := strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
				return r
			}
			return -1
		}, word)
		if clean != "" {
			escaped[i] = clean + ":*" // Prefix matching
		}
	}

	// Filter empty strings and join with OR for broader matching
	var nonEmpty []string
	for _, e := range escaped {
		if e != "" {
			nonEmpty = append(nonEmpty, e)
		}
	}
	return strings.Join(nonEmpty, " | ")
}

// getSearchTerm extracts the search term from a filter
// NIP-50 uses a "search" field in the filter
func getSearchTerm(filter nostr.Filter) string {
	// The go-nostr library stores extra fields in filter.Search
	return filter.Search
}

// itoa converts int to string without importing strconv
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var s string
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}

// HasSearch checks if a filter contains a search term
func HasSearch(filter nostr.Filter) bool {
	return filter.Search != ""
}

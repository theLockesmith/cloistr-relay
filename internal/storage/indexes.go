package storage

import (
	"database/sql"
	"log"
)

// OptimizeIndexes creates additional indexes for common Nostr query patterns
// These supplement the indexes created by the eventstore library
func OptimizeIndexes(db *sql.DB) error {
	indexes := []struct {
		name string
		sql  string
	}{
		// Composite index for kind + created_at (common filter pattern)
		{
			name: "event_kind_created_at_idx",
			sql:  `CREATE INDEX CONCURRENTLY IF NOT EXISTS event_kind_created_at_idx ON event (kind, created_at DESC)`,
		},
		// Composite index for pubkey + kind (profile lookups, user's notes of specific kind)
		{
			name: "event_pubkey_kind_idx",
			sql:  `CREATE INDEX CONCURRENTLY IF NOT EXISTS event_pubkey_kind_idx ON event (pubkey, kind)`,
		},
		// Composite index for pubkey + created_at (user timeline queries)
		{
			name: "event_pubkey_created_at_idx",
			sql:  `CREATE INDEX CONCURRENTLY IF NOT EXISTS event_pubkey_created_at_idx ON event (pubkey, created_at DESC)`,
		},
		// Partial index for metadata events (kind 0) - heavily queried
		{
			name: "event_metadata_idx",
			sql:  `CREATE INDEX CONCURRENTLY IF NOT EXISTS event_metadata_idx ON event (pubkey) WHERE kind = 0`,
		},
		// Partial index for contact lists (kind 3) - heavily queried for WoT
		{
			name: "event_contacts_idx",
			sql:  `CREATE INDEX CONCURRENTLY IF NOT EXISTS event_contacts_idx ON event (pubkey) WHERE kind = 3`,
		},
		// Partial index for text notes (kind 1) with time ordering
		{
			name: "event_text_notes_idx",
			sql:  `CREATE INDEX CONCURRENTLY IF NOT EXISTS event_text_notes_idx ON event (created_at DESC) WHERE kind = 1`,
		},
		// Partial index for reactions (kind 7)
		{
			name: "event_reactions_idx",
			sql:  `CREATE INDEX CONCURRENTLY IF NOT EXISTS event_reactions_idx ON event (created_at DESC) WHERE kind = 7`,
		},
		// Partial index for reposts (kind 6)
		{
			name: "event_reposts_idx",
			sql:  `CREATE INDEX CONCURRENTLY IF NOT EXISTS event_reposts_idx ON event (created_at DESC) WHERE kind = 6`,
		},
		// Partial index for DMs (kind 4) - often queried by recipient
		{
			name: "event_dms_idx",
			sql:  `CREATE INDEX CONCURRENTLY IF NOT EXISTS event_dms_idx ON event (pubkey, created_at DESC) WHERE kind = 4`,
		},
		// Partial index for NIP-17 chat messages (kind 14)
		{
			name: "event_nip17_chat_idx",
			sql:  `CREATE INDEX CONCURRENTLY IF NOT EXISTS event_nip17_chat_idx ON event (pubkey, created_at DESC) WHERE kind = 14`,
		},
		// Partial index for NIP-17 DM relay lists (kind 10050)
		{
			name: "event_nip17_relay_list_idx",
			sql:  `CREATE INDEX CONCURRENTLY IF NOT EXISTS event_nip17_relay_list_idx ON event (pubkey) WHERE kind = 10050`,
		},
		// Partial index for gift wraps (kind 1059) - NIP-59
		{
			name: "event_giftwrap_idx",
			sql:  `CREATE INDEX CONCURRENTLY IF NOT EXISTS event_giftwrap_idx ON event (created_at DESC) WHERE kind = 1059`,
		},
		// Partial index for zap receipts (kind 9735) - NIP-57
		{
			name: "event_zaps_idx",
			sql:  `CREATE INDEX CONCURRENTLY IF NOT EXISTS event_zaps_idx ON event (created_at DESC) WHERE kind = 9735`,
		},
		// Index for replaceable events (kind 10000-19999) - by pubkey for latest lookup
		{
			name: "event_replaceable_idx",
			sql:  `CREATE INDEX CONCURRENTLY IF NOT EXISTS event_replaceable_idx ON event (pubkey, kind, created_at DESC) WHERE kind >= 10000 AND kind < 20000`,
		},
		// Index for parameterized replaceable events (kind 30000-39999)
		{
			name: "event_param_replaceable_idx",
			sql:  `CREATE INDEX CONCURRENTLY IF NOT EXISTS event_param_replaceable_idx ON event (pubkey, kind, created_at DESC) WHERE kind >= 30000 AND kind < 40000`,
		},
	}

	for _, idx := range indexes {
		// CONCURRENTLY requires running outside a transaction and may fail if index exists
		// We use IF NOT EXISTS so this is idempotent
		_, err := db.Exec(idx.sql)
		if err != nil {
			// Log but don't fail - indexes are optimizations, not requirements
			log.Printf("Warning: failed to create index %s: %v", idx.name, err)
		} else {
			log.Printf("Index %s ready", idx.name)
		}
	}

	// Create additional statistics targets for better query planning
	_, err := db.Exec(`
		ALTER TABLE event ALTER COLUMN kind SET STATISTICS 1000;
		ALTER TABLE event ALTER COLUMN pubkey SET STATISTICS 1000;
		ALTER TABLE event ALTER COLUMN created_at SET STATISTICS 1000;
	`)
	if err != nil {
		log.Printf("Warning: failed to set statistics targets: %v", err)
	}

	// Analyze table to update statistics
	_, err = db.Exec(`ANALYZE event`)
	if err != nil {
		log.Printf("Warning: failed to analyze event table: %v", err)
	} else {
		log.Println("Event table statistics updated")
	}

	return nil
}

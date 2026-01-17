package storage

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"
	"gitlab.com/coldforge/coldforge-relay/internal/config"
)

// NewPostgresConnection creates and initializes a PostgreSQL connection
func NewPostgresConnection(cfg *config.Config) (*sql.DB, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.DBHost,
		cfg.DBPort,
		cfg.DBUser,
		cfg.DBPassword,
		cfg.DBName,
	)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("Connected to PostgreSQL")

	// Initialize database schema
	if err := initSchema(db); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return db, nil
}

// initSchema creates necessary database tables
func initSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS events (
		id TEXT PRIMARY KEY,
		pubkey TEXT NOT NULL,
		created_at BIGINT NOT NULL,
		kind INTEGER NOT NULL,
		tags JSONB NOT NULL DEFAULT '[]',
		content TEXT NOT NULL,
		sig TEXT NOT NULL,
		inserted_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_pubkey (pubkey),
		INDEX idx_kind (kind),
		INDEX idx_created_at (created_at)
	);

	CREATE TABLE IF NOT EXISTS event_tags (
		event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
		tag_name TEXT NOT NULL,
		tag_value TEXT NOT NULL,
		position INTEGER NOT NULL,
		PRIMARY KEY (event_id, tag_name, position),
		INDEX idx_tag_name_value (tag_name, tag_value)
	);
	`

	// Note: The above schema uses PostgreSQL syntax, but we're using standard SQL
	// that should work with postgres. The actual indexes might need adjustment
	// for proper PostgreSQL syntax.

	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS events (
			id TEXT PRIMARY KEY,
			pubkey TEXT NOT NULL,
			created_at BIGINT NOT NULL,
			kind INTEGER NOT NULL,
			tags JSONB NOT NULL DEFAULT '[]'::jsonb,
			content TEXT NOT NULL,
			sig TEXT NOT NULL,
			inserted_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		return fmt.Errorf("failed to create events table: %w", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS event_tags (
			event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
			tag_name TEXT NOT NULL,
			tag_value TEXT NOT NULL,
			position INTEGER NOT NULL,
			PRIMARY KEY (event_id, tag_name, position)
		);
	`)
	if err != nil {
		return fmt.Errorf("failed to create event_tags table: %w", err)
	}

	// Create indexes for common queries
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_events_pubkey ON events(pubkey)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_events_kind ON events(kind)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_event_tags_tag ON event_tags(tag_name, tag_value)`)

	return nil
}

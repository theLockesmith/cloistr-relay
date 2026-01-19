package storage

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/fiatjaf/eventstore/postgresql"
	_ "github.com/lib/pq"
	"gitlab.com/coldforge/coldforge-relay/internal/config"
)

// NewPostgresBackend creates and initializes a PostgreSQL eventstore backend
func NewPostgresBackend(cfg *config.Config) (*postgresql.PostgresBackend, error) {
	// Build connection URL
	dbURL := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.DBUser,
		cfg.DBPassword,
		cfg.DBHost,
		cfg.DBPort,
		cfg.DBName,
	)

	// Create backend with configuration
	backend := &postgresql.PostgresBackend{
		DatabaseURL:      dbURL,
		QueryLimit:       1000,      // Max events per query
		QueryIDsLimit:    500,       // Max IDs in a filter
		QueryAuthorsLimit: 100,      // Max authors in a filter
		QueryKindsLimit:  20,        // Max kinds in a filter
		QueryTagsLimit:   10,        // Max tag filters
	}

	// Initialize the database connection and schema
	if err := backend.Init(); err != nil {
		return nil, fmt.Errorf("failed to initialize postgres backend: %w", err)
	}

	log.Printf("Connected to PostgreSQL at %s:%d/%s", cfg.DBHost, cfg.DBPort, cfg.DBName)

	return backend, nil
}

// GetDatabaseURL returns the database connection URL for a given config
func GetDatabaseURL(cfg *config.Config) string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.DBUser,
		cfg.DBPassword,
		cfg.DBHost,
		cfg.DBPort,
		cfg.DBName,
	)
}

// NewRawConnection creates a raw database/sql connection for advanced queries
func NewRawConnection(cfg *config.Config) (*sql.DB, error) {
	dbURL := GetDatabaseURL(cfg)
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

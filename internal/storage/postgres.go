package storage

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/fiatjaf/eventstore/postgresql"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/config"
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

	// Create sqlx connection with tuned pool settings
	db, err := sqlx.Connect("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	// Apply connection pool tuning
	db.SetMaxOpenConns(cfg.DBMaxOpenConns)
	db.SetMaxIdleConns(cfg.DBMaxIdleConns)
	db.SetConnMaxLifetime(cfg.DBConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.DBConnMaxIdleTime)

	// Create backend with pre-configured connection
	backend := &postgresql.PostgresBackend{
		DatabaseURL:       dbURL,
		DB:                db, // Use our tuned connection
		QueryLimit:        1000, // Max events per query
		QueryIDsLimit:     500,  // Max IDs in a filter
		QueryAuthorsLimit: 500,  // Max authors in a filter
		QueryKindsLimit:   50,   // Max kinds in a filter
		QueryTagsLimit:    1000, // Max tag values in filters (for Amethyst compatibility)
	}

	// Initialize schema (won't create new connection since DB is already set)
	if err := backend.Init(); err != nil {
		return nil, fmt.Errorf("failed to initialize postgres backend: %w", err)
	}

	log.Printf("Connected to PostgreSQL at %s:%d/%s (pool: %d open, %d idle, %v lifetime)",
		cfg.DBHost, cfg.DBPort, cfg.DBName,
		cfg.DBMaxOpenConns, cfg.DBMaxIdleConns, cfg.DBConnMaxLifetime)

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

	// Apply connection pool tuning
	db.SetMaxOpenConns(cfg.DBMaxOpenConns)
	db.SetMaxIdleConns(cfg.DBMaxIdleConns)
	db.SetConnMaxLifetime(cfg.DBConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.DBConnMaxIdleTime)

	// Test the connection
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

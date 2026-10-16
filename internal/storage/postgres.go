package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/fiatjaf/eventstore/postgresql"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/config"
)

// tunedDialer wraps net.Dialer to implement pq.Dialer with TCP keepalive and
// TCP_USER_TIMEOUT, so the kernel reaps dead sockets instead of leaving them
// for the full TCP retransmission timeout (~15 min on Linux defaults).
type tunedDialer struct {
	d net.Dialer
}

func (td *tunedDialer) Dial(network, address string) (net.Conn, error) {
	return td.d.Dial(network, address)
}

func (td *tunedDialer) DialTimeout(network, address string, timeout time.Duration) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return td.d.DialContext(ctx, network, address)
}

func (td *tunedDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return td.d.DialContext(ctx, network, address)
}

var tunedConnectTimeout = 5 * time.Second

func newTunedConnector(dsn string) (*pq.Connector, error) {
	pqCfg, err := pq.NewConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	pqCfg.ConnectTimeout = tunedConnectTimeout

	connector, err := pq.NewConnectorConfig(pqCfg)
	if err != nil {
		return nil, fmt.Errorf("create connector: %w", err)
	}
	connector.Dialer(&tunedDialer{d: net.Dialer{
		KeepAlive: 30 * time.Second,
		Control:   tcpUserTimeoutControl(30 * time.Second),
	}})

	return connector, nil
}

// NewPostgresBackend creates and initializes a PostgreSQL eventstore backend
func NewPostgresBackend(cfg *config.Config) (*postgresql.PostgresBackend, error) {
	dbURL := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.DBUser,
		cfg.DBPassword,
		cfg.DBHost,
		cfg.DBPort,
		cfg.DBName,
	)

	connector, err := newTunedConnector(dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create tuned connector: %w", err)
	}
	db := sqlx.NewDb(sql.OpenDB(connector), "postgres")
	if err := db.Ping(); err != nil {
		_ = db.Close()
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
		DB:                db,
		QueryLimit:        1000,
		QueryIDsLimit:     500,
		QueryAuthorsLimit: 500,
		QueryKindsLimit:   50,
		QueryTagsLimit:    1000,
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

// NewRawConnection creates a raw database/sql connection for advanced queries.
// It uses a tuned pq connector with connect_timeout, TCP keepalive, and
// TCP_USER_TIMEOUT (Linux) so dead sockets are reaped by the kernel rather
// than blocking for the full TCP retransmission timeout.
func NewRawConnection(cfg *config.Config) (*sql.DB, error) {
	dbURL := GetDatabaseURL(cfg)
	connector, err := newTunedConnector(dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create tuned connector: %w", err)
	}

	db := sql.OpenDB(connector)

	db.SetMaxOpenConns(cfg.DBMaxOpenConns)
	db.SetMaxIdleConns(cfg.DBMaxIdleConns)
	db.SetConnMaxLifetime(cfg.DBConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.DBConnMaxIdleTime)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

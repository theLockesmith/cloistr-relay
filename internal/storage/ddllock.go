package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq"
)

// ddlLockKey is a fixed advisory lock ID used to serialize schema DDL across
// replicas. Two pods starting simultaneously both run CREATE INDEX CONCURRENTLY
// and backend.Init(), which races on PostgreSQL catalog rows and produces
// "pq: tuple concurrently updated (XX000)".
const ddlLockKey int64 = 0x636C6F69737472 // "cloistr" truncated to int64

// DDLLockKey is exported for testing.
const DDLLockKey = ddlLockKey

// DDLLock holds a PostgreSQL advisory lock on a dedicated connection.
type DDLLock struct {
	conn *sql.Conn
	db   *sql.DB
}

// AcquireDDLLock polls pg_try_advisory_lock on a dedicated connection until the
// lock is acquired or the context expires.
//
// Polling instead of blocking is required because pg_advisory_lock holds a
// snapshot for the duration of the wait. CREATE INDEX CONCURRENTLY (run by the
// lock holder) waits for every older snapshot to drain before it can finish.
// A blocking waiter therefore deadlocks the holder's CIC indefinitely, and
// PostgreSQL's deadlock detector cannot see it.
//
// The caller controls the deadline via ctx. A context without a deadline polls
// indefinitely (not recommended in production -- use context.WithTimeout).
func AcquireDDLLock(ctx context.Context, dbURL string) (*DDLLock, error) {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("ddl lock connection: %w", err)
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ddl lock dedicated conn: %w", err)
	}

	start := time.Now()
	lastLog := start
	for {
		var acquired bool
		if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", ddlLockKey).Scan(&acquired); err != nil {
			_ = conn.Close()
			_ = db.Close()
			return nil, fmt.Errorf("pg_try_advisory_lock: %w", err)
		}
		if acquired {
			break
		}
		if ctx.Err() != nil {
			_ = conn.Close()
			_ = db.Close()
			return nil, fmt.Errorf("timed out waiting for DDL advisory lock after %v: %w", time.Since(start).Round(time.Second), ctx.Err())
		}
		if time.Since(lastLog) >= 30*time.Second {
			log.Printf("Waiting for DDL lock (held by another replica), %vs elapsed", int(time.Since(start).Seconds()))
			lastLog = time.Now()
		}
		time.Sleep(200 * time.Millisecond)
	}

	log.Println("Acquired DDL advisory lock (serializing schema init across replicas)")

	return &DDLLock{conn: conn, db: db}, nil
}

// Unlock releases the advisory lock while keeping the connection open.
func (l *DDLLock) Unlock() {
	var released bool
	if err := l.conn.QueryRowContext(context.Background(), "SELECT pg_advisory_unlock($1)", ddlLockKey).Scan(&released); err != nil {
		log.Printf("Warning: DDL advisory unlock query failed: %v", err)
	} else if !released {
		log.Println("Warning: DDL advisory unlock returned false (lock was not held on this connection)")
	} else {
		log.Println("Released DDL advisory lock")
	}
}

// Close releases the underlying connection and pool. If Unlock was not called,
// closing the connection implicitly releases the lock.
func (l *DDLLock) Close() {
	if err := l.conn.Close(); err != nil {
		log.Printf("Warning: DDL lock connection close failed: %v", err)
	}
	if err := l.db.Close(); err != nil {
		log.Printf("Warning: DDL lock pool close failed: %v", err)
	}
}

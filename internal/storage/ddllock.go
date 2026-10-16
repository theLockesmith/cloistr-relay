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
// lock is acquired or the deadline (5 minutes) expires.
//
// Polling instead of blocking is required because pg_advisory_lock holds a
// snapshot for the duration of the wait. CREATE INDEX CONCURRENTLY (run by the
// lock holder) waits for every older snapshot to drain before it can finish.
// A blocking waiter therefore deadlocks the holder's CIC indefinitely, and
// PostgreSQL's deadlock detector cannot see it.
func AcquireDDLLock(dbURL string) (*DDLLock, error) {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("ddl lock connection: %w", err)
	}

	conn, err := db.Conn(context.Background())
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("ddl lock dedicated conn: %w", err)
	}

	deadline := time.Now().Add(5 * time.Minute)
	for {
		var acquired bool
		if err := conn.QueryRowContext(context.Background(), "SELECT pg_try_advisory_lock($1)", ddlLockKey).Scan(&acquired); err != nil {
			conn.Close()
			db.Close()
			return nil, fmt.Errorf("pg_try_advisory_lock: %w", err)
		}
		if acquired {
			break
		}
		if time.Now().After(deadline) {
			conn.Close()
			db.Close()
			return nil, fmt.Errorf("timed out waiting for DDL advisory lock after 5 minutes")
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
	l.conn.Close()
	l.db.Close()
}

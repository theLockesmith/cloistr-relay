package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"
)

// ddlLockKey is a fixed advisory lock ID used to serialize schema DDL across
// replicas. Two pods starting simultaneously both run CREATE INDEX CONCURRENTLY
// and backend.Init(), which races on PostgreSQL catalog rows and produces
// "pq: tuple concurrently updated (XX000)".
const ddlLockKey int64 = 0x636C6F69737472 // "cloistr" truncated to int64

// DDLLockKey is exported for testing.
const DDLLockKey = ddlLockKey

// AcquireDDLLock takes a PostgreSQL session-level advisory lock on a dedicated
// connection. The lock and unlock MUST run on the same connection because
// pg_advisory_lock is session-scoped; using db.Exec on a pool can route them
// to different pooled connections, making the unlock a silent no-op.
//
// Returns a release function that unlocks on the same connection and closes it.
func AcquireDDLLock(dbURL string) (release func(), err error) {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("ddl lock connection: %w", err)
	}

	conn, err := db.Conn(context.Background())
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("ddl lock dedicated conn: %w", err)
	}

	if _, err := conn.ExecContext(context.Background(), "SELECT pg_advisory_lock($1)", ddlLockKey); err != nil {
		conn.Close()
		db.Close()
		return nil, fmt.Errorf("acquire ddl advisory lock: %w", err)
	}

	log.Println("Acquired DDL advisory lock (serializing schema init across replicas)")

	return func() {
		var released bool
		if err := conn.QueryRowContext(context.Background(), "SELECT pg_advisory_unlock($1)", ddlLockKey).Scan(&released); err != nil {
			log.Printf("Warning: DDL advisory unlock query failed: %v", err)
		} else if !released {
			log.Println("Warning: DDL advisory unlock returned false (lock was not held on this connection)")
		} else {
			log.Println("Released DDL advisory lock")
		}
		conn.Close()
		db.Close()
	}, nil
}

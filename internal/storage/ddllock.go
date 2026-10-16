package storage

import (
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

// AcquireDDLLock takes a PostgreSQL session-level advisory lock. Other
// connections calling AcquireDDLLock with the same key block until this one
// releases. Returns a release function that must be called when DDL is done.
func AcquireDDLLock(dbURL string) (release func(), err error) {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("ddl lock connection: %w", err)
	}

	if _, err := db.Exec("SELECT pg_advisory_lock($1)", ddlLockKey); err != nil {
		db.Close()
		return nil, fmt.Errorf("acquire ddl advisory lock: %w", err)
	}

	log.Println("Acquired DDL advisory lock (serializing schema init across replicas)")

	return func() {
		db.Exec("SELECT pg_advisory_unlock($1)", ddlLockKey)
		db.Close()
		log.Println("Released DDL advisory lock")
	}, nil
}

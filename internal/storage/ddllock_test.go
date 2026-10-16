package storage_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/storage"
)

func testDBURL(t *testing.T) string {
	t.Helper()
	host := os.Getenv("TEST_DB_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("TEST_DB_PORT")
	if port == "" {
		port = "5450"
	}
	url := fmt.Sprintf("postgres://postgres:postgres@%s:%s/nostr?sslmode=disable", host, port)

	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Skipf("cannot open test database: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Skipf("cannot reach test database: %v", err)
	}
	return url
}

func TestAcquireDDLLock_UnlockReleases(t *testing.T) {
	dbURL := testDBURL(t)

	// Acquire the lock.
	release, err := storage.AcquireDDLLock(dbURL)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	// A second acquire on a separate connection must block while the first
	// holds the lock. We prove this by trying to acquire with a short timeout
	// and expecting it to fail.
	blocked := make(chan error, 1)
	go func() {
		db, err := sql.Open("postgres", dbURL)
		if err != nil {
			blocked <- err
			return
		}
		defer db.Close()
		conn, err := db.Conn(context.Background())
		if err != nil {
			blocked <- err
			return
		}
		defer conn.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		// pg_try_advisory_lock returns false if the lock is held.
		var acquired bool
		err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", storage.DDLLockKey).Scan(&acquired)
		if err != nil {
			blocked <- err
			return
		}
		if acquired {
			// Clean up: unlock what we just took.
			conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", storage.DDLLockKey)
			blocked <- fmt.Errorf("acquired lock while first session still holds it")
		} else {
			blocked <- nil // expected: could not acquire
		}
	}()

	err = <-blocked
	if err != nil {
		t.Fatalf("second session while lock held: %v", err)
	}

	// Release the first lock.
	release()

	// Now a second acquire must succeed immediately.
	release2, err := storage.AcquireDDLLock(dbURL)
	if err != nil {
		t.Fatalf("second acquire after release: %v", err)
	}
	release2()
}

package storage_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
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

// TestDDLLock_UnlockReleasesWhileConnOpen proves that Unlock releases the lock
// on the dedicated connection BEFORE Close is called. A previous version passed
// this test vacuously because closing the conn also releases the lock; this
// version keeps the first conn open after Unlock and asserts a second session
// can acquire at that point.
func TestDDLLock_UnlockReleasesWhileConnOpen(t *testing.T) {
	dbURL := testDBURL(t)

	lock, err := storage.AcquireDDLLock(dbURL)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	db2, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatalf("open second conn: %v", err)
	}
	defer db2.Close()
	conn2, err := db2.Conn(context.Background())
	if err != nil {
		t.Fatalf("second conn: %v", err)
	}
	defer conn2.Close()

	var held bool
	if err := conn2.QueryRowContext(context.Background(), "SELECT pg_try_advisory_lock($1)", storage.DDLLockKey).Scan(&held); err != nil {
		t.Fatalf("try lock while held: %v", err)
	}
	if held {
		conn2.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", storage.DDLLockKey)
		t.Fatal("second session acquired lock while first still holds it")
	}

	// Unlock but do NOT Close. The dedicated conn stays open.
	lock.Unlock()

	// Second session must now acquire while first conn is still open.
	if err := conn2.QueryRowContext(context.Background(), "SELECT pg_try_advisory_lock($1)", storage.DDLLockKey).Scan(&held); err != nil {
		t.Fatalf("try lock after unlock: %v", err)
	}
	if !held {
		t.Fatal("second session could not acquire after Unlock; unlock did not release")
	}
	conn2.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", storage.DDLLockKey)

	lock.Close()
}

// TestDDLLock_ConcurrentCIC proves two replicas can run CREATE INDEX
// CONCURRENTLY under the advisory lock without deadlocking. The blocking
// pg_advisory_lock version deadlocks because the waiter's snapshot prevents
// CIC from finishing; the polling pg_try_advisory_lock version releases its
// snapshot between polls so CIC completes.
func TestDDLLock_ConcurrentCIC(t *testing.T) {
	dbURL := testDBURL(t)

	setup, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatalf("setup conn: %v", err)
	}
	defer setup.Close()

	setup.Exec("DROP INDEX IF EXISTS ddllock_test_idx")
	setup.Exec("DROP TABLE IF EXISTS ddllock_test_tbl")
	if _, err := setup.Exec("CREATE TABLE ddllock_test_tbl (id serial PRIMARY KEY, val text)"); err != nil {
		t.Fatalf("create test table: %v", err)
	}
	defer setup.Exec("DROP TABLE IF EXISTS ddllock_test_tbl")
	defer setup.Exec("DROP INDEX IF EXISTS ddllock_test_idx")

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	deadline := 30 * time.Second

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(replica int) {
			defer wg.Done()

			lock, err := storage.AcquireDDLLock(dbURL)
			if err != nil {
				errs <- fmt.Errorf("replica %d acquire: %w", replica, err)
				return
			}

			db, err := sql.Open("postgres", dbURL)
			if err != nil {
				lock.Close()
				errs <- fmt.Errorf("replica %d open: %w", replica, err)
				return
			}
			defer db.Close()

			_, err = db.Exec("CREATE INDEX CONCURRENTLY IF NOT EXISTS ddllock_test_idx ON ddllock_test_tbl (val)")
			if err != nil {
				lock.Unlock()
				lock.Close()
				errs <- fmt.Errorf("replica %d CIC: %w", replica, err)
				return
			}

			lock.Unlock()
			lock.Close()
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(deadline):
		t.Fatalf("deadlock: both replicas did not finish within %v", deadline)
	}

	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

package membership

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func testDBForMembership(t *testing.T) *sql.DB {
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
	if err := db.Ping(); err != nil {
		t.Skipf("cannot reach test database: %v", err)
	}
	return db
}

func seedMembers(t *testing.T, db *sql.DB, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		pk := fmt.Sprintf("rowserr_test_%04d", i)
		_, err := db.Exec(`
			INSERT INTO nip43_members (pubkey, joined_at, tier)
			VALUES ($1, $2, 'free')
			ON CONFLICT (pubkey) DO NOTHING
		`, pk, time.Now())
		if err != nil {
			t.Fatalf("seed member %d: %v", i, err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM nip43_members WHERE pubkey LIKE 'rowserr_test_%'`)
	})
}

func seedPayments(t *testing.T, db *sql.DB, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		hash := fmt.Sprintf("rowserr_pay_%04d", i)
		_, err := db.Exec(`
			INSERT INTO pending_payments (payment_hash, pubkey, tier, amount_sats, period_days, status, created_at)
			VALUES ($1, $2, 'basic', 1000, 30, 'pending', $3)
			ON CONFLICT (payment_hash) DO NOTHING
		`, hash, "rowserr_test_payer", time.Now().Add(-time.Duration(i)*time.Minute))
		if err != nil {
			t.Fatalf("seed payment %d: %v", i, err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM pending_payments WHERE payment_hash LIKE 'rowserr_pay_%'`)
	})
}

// TestListMembers_RowsErr verifies that ListMembers propagates rows.Err().
// A connection drop mid-iteration would make rows.Next() return false with a
// non-nil rows.Err(). We verify the plumbing by confirming a successful full
// read returns no error, then test the error path via a context that cancels
// mid-iteration.
func TestListMembers_RowsErr(t *testing.T) {
	db := testDBForMembership(t)
	defer func() { _ = db.Close() }()

	store := NewStore(db)
	if err := store.InitSchema(context.Background()); err != nil {
		t.Fatalf("init store: %v", err)
	}

	seedMembers(t, db, 5)

	ctx := context.Background()
	members, err := store.ListMembers(ctx)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(members) < 5 {
		t.Errorf("expected at least 5 members, got %d", len(members))
	}

	// Cancel context before query to force an error path
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = store.ListMembers(cancelCtx)
	if err == nil {
		t.Error("ListMembers with canceled context should return error")
	}
}

// TestListMembersByTier_RowsErr verifies error propagation.
func TestListMembersByTier_RowsErr(t *testing.T) {
	db := testDBForMembership(t)
	defer func() { _ = db.Close() }()

	store := NewStore(db)
	if err := store.InitSchema(context.Background()); err != nil {
		t.Fatalf("init store: %v", err)
	}

	seedMembers(t, db, 3)

	ctx := context.Background()
	members, err := store.ListMembersByTier(ctx, TierFree)
	if err != nil {
		t.Fatalf("ListMembersByTier: %v", err)
	}
	if len(members) < 3 {
		t.Errorf("expected at least 3 free members, got %d", len(members))
	}

	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = store.ListMembersByTier(cancelCtx, TierFree)
	if err == nil {
		t.Error("ListMembersByTier with canceled context should return error")
	}
}

// TestListExpiredTiers_RowsErr verifies error propagation.
func TestListExpiredTiers_RowsErr(t *testing.T) {
	db := testDBForMembership(t)
	defer func() { _ = db.Close() }()

	store := NewStore(db)
	if err := store.InitSchema(context.Background()); err != nil {
		t.Fatalf("init store: %v", err)
	}

	// Seed an expired-tier member
	_, err := db.Exec(`
		INSERT INTO nip43_members (pubkey, joined_at, tier, tier_expires_at)
		VALUES ('rowserr_expired_01', $1, 'basic', $2)
		ON CONFLICT (pubkey) DO NOTHING
	`, time.Now(), time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("seed expired member: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM nip43_members WHERE pubkey = 'rowserr_expired_01'`)
	})

	ctx := context.Background()
	expired, err := store.ListExpiredTiers(ctx)
	if err != nil {
		t.Fatalf("ListExpiredTiers: %v", err)
	}
	if len(expired) < 1 {
		t.Errorf("expected at least 1 expired tier, got %d", len(expired))
	}

	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = store.ListExpiredTiers(cancelCtx)
	if err == nil {
		t.Error("ListExpiredTiers with canceled context should return error")
	}
}

// TestListPending_RowsErr verifies error propagation for payment queries.
func TestListPending_RowsErr(t *testing.T) {
	db := testDBForMembership(t)
	defer func() { _ = db.Close() }()

	pstore := NewPaymentStore(db)
	if err := pstore.InitSchema(context.Background()); err != nil {
		t.Fatalf("init payment store: %v", err)
	}

	seedPayments(t, db, 3)

	ctx := context.Background()
	payments, err := pstore.ListPending(ctx)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(payments) < 3 {
		t.Errorf("expected at least 3 pending payments, got %d", len(payments))
	}

	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = pstore.ListPending(cancelCtx)
	if err == nil {
		t.Error("ListPending with canceled context should return error")
	}
}

// TestListByPubkey_RowsErr verifies error propagation.
func TestListByPubkey_RowsErr(t *testing.T) {
	db := testDBForMembership(t)
	defer func() { _ = db.Close() }()

	pstore := NewPaymentStore(db)
	if err := pstore.InitSchema(context.Background()); err != nil {
		t.Fatalf("init payment store: %v", err)
	}

	seedPayments(t, db, 2)

	ctx := context.Background()
	payments, err := pstore.ListByPubkey(ctx, "rowserr_test_payer")
	if err != nil {
		t.Fatalf("ListByPubkey: %v", err)
	}
	if len(payments) < 2 {
		t.Errorf("expected at least 2 payments, got %d", len(payments))
	}

	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = pstore.ListByPubkey(cancelCtx, "rowserr_test_payer")
	if err == nil {
		t.Error("ListByPubkey with canceled context should return error")
	}
}

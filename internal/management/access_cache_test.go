package management

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

const (
	cacheTestPubkey  = "c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00"
	cacheTestPubkey2 = "c0ffee11c0ffee11c0ffee11c0ffee11c0ffee11c0ffee11c0ffee11c0ffee11"
	cacheTestEventID = "e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0"
)

func freshAccessTables(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, q := range []string{
		"DELETE FROM management_banned_pubkeys",
		"DELETE FROM management_banned_events",
		"DELETE FROM management_rate_limit_exempt_pubkeys",
		"DELETE FROM management_allowed_kinds",
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
}

// The per-event checks must not touch the database. Before 2026-10-29 each one
// was a Postgres query (five per event, ahead of auth), which is what drove the
// relay's p95 event latency past 0.5s. Proof: load the lists, then make the
// database unreachable. The old code queried, got an error, and answered
// "not banned / kind allowed / not exempt" for everything.
func TestAccessChecks_AnswerFromMemoryWithoutTheDatabase(t *testing.T) {
	testDB(t) // skips without TEST_DATABASE_URL
	db, err := sql.Open("postgres", testDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	if err := store.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	freshAccessTables(t, db)
	if err := store.BanPubkey(cacheTestPubkey, "spam"); err != nil {
		t.Fatal(err)
	}
	if err := store.BanEvent(cacheTestEventID, "abuse"); err != nil {
		t.Fatal(err)
	}
	if err := store.ExemptPubkeyFromRateLimit(cacheTestPubkey2, "fleet"); err != nil {
		t.Fatal(err)
	}
	if err := store.AllowKind(1); err != nil {
		t.Fatal(err)
	}

	_ = db.Close() // every query from here on fails

	if !store.IsPubkeyBanned(cacheTestPubkey) {
		t.Error("banned pubkey reads as not banned once the database is unreachable")
	}
	if !store.IsEventBanned(cacheTestEventID) {
		t.Error("banned event reads as not banned once the database is unreachable")
	}
	if !store.IsRateLimitExempt(cacheTestPubkey2) {
		t.Error("exempt pubkey reads as not exempt once the database is unreachable")
	}
	if store.IsKindAllowed(7) {
		t.Error("kind 7 reads as allowed although only kind 1 is on the allow list")
	}
	if !store.IsKindAllowed(1) {
		t.Error("kind 1 is on the allow list but reads as not allowed")
	}
	if store.IsPubkeyBanned(cacheTestPubkey2) {
		t.Error("unbanned pubkey reads as banned")
	}
}

// A failed reload keeps the last good lists. Clearing them would unban
// everyone whenever the database blips.
func TestAccessRefresh_FailureKeepsPreviousLists(t *testing.T) {
	testDB(t)
	db, err := sql.Open("postgres", testDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	if err := store.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	freshAccessTables(t, db)
	if err := store.BanPubkey(cacheTestPubkey, "spam"); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	if err := store.RefreshAccess(context.Background()); err == nil {
		t.Fatal("refresh against a closed database reported success")
	}
	if !store.IsPubkeyBanned(cacheTestPubkey) {
		t.Error("a failed refresh cleared the ban list")
	}
}

// A write on THIS store applies to the next event at once; a write made
// elsewhere (the other relay pod, cmd/admin) appears after a refresh.
func TestAccessLists_WritesApplyImmediatelyAndOtherWritersOnRefresh(t *testing.T) {
	db := testDB(t)
	mine, other := NewStore(db), NewStore(db)
	if err := mine.Init(); err != nil {
		t.Fatal(err)
	}
	if err := other.Init(); err != nil {
		t.Fatal(err)
	}
	freshAccessTables(t, db)
	if err := mine.RefreshAccess(context.Background()); err != nil {
		t.Fatal(err)
	}

	if err := mine.BanPubkey(cacheTestPubkey, "spam"); err != nil {
		t.Fatal(err)
	}
	if !mine.IsPubkeyBanned(cacheTestPubkey) {
		t.Fatal("ban did not apply on the store that wrote it")
	}
	if err := mine.UnbanPubkey(cacheTestPubkey); err != nil {
		t.Fatal(err)
	}
	if mine.IsPubkeyBanned(cacheTestPubkey) {
		t.Fatal("unban did not apply on the store that wrote it")
	}

	if err := other.BanEvent(cacheTestEventID, "abuse"); err != nil {
		t.Fatal(err)
	}
	if mine.IsEventBanned(cacheTestEventID) {
		t.Fatal("test premise: another writer's ban is visible before a refresh")
	}
	if err := mine.RefreshAccess(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !mine.IsEventBanned(cacheTestEventID) {
		t.Error("another writer's ban is not visible after a refresh")
	}
}

func testDSN(t *testing.T) string {
	t.Helper()
	return os.Getenv("TEST_DATABASE_URL")
}

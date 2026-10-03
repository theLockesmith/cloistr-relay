package management

import (
	"database/sql"
	"encoding/json"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestStore_RateLimitExemptions(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	if err := store.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Clean up from previous runs
	_, _ = db.Exec("DELETE FROM management_rate_limit_exempt_pubkeys")

	pk := "aabbccdd00112233aabbccdd00112233aabbccdd00112233aabbccdd00112233"

	t.Run("not exempt initially", func(t *testing.T) {
		if store.IsRateLimitExempt(pk) {
			t.Error("expected pubkey to NOT be exempt before adding")
		}
	})

	t.Run("add exemption", func(t *testing.T) {
		if err := store.ExemptPubkeyFromRateLimit(pk, "fleet service"); err != nil {
			t.Fatalf("exempt: %v", err)
		}
		if !store.IsRateLimitExempt(pk) {
			t.Error("expected pubkey to be exempt after adding")
		}
	})

	t.Run("upsert updates reason", func(t *testing.T) {
		if err := store.ExemptPubkeyFromRateLimit(pk, "updated reason"); err != nil {
			t.Fatalf("upsert: %v", err)
		}
		list, err := store.ListRateLimitExemptions(100, 0)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(list) != 1 {
			t.Fatalf("expected 1 exemption, got %d", len(list))
		}
		if list[0].Reason != "updated reason" {
			t.Errorf("expected reason 'updated reason', got %q", list[0].Reason)
		}
	})

	t.Run("remove exemption", func(t *testing.T) {
		if err := store.RemoveRateLimitExemption(pk); err != nil {
			t.Fatalf("remove: %v", err)
		}
		if store.IsRateLimitExempt(pk) {
			t.Error("expected pubkey to NOT be exempt after removing")
		}
	})

	t.Run("list empty after removal", func(t *testing.T) {
		list, err := store.ListRateLimitExemptions(100, 0)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(list) != 0 {
			t.Errorf("expected 0 exemptions, got %d", len(list))
		}
	})
}

func TestMethodHandler_RateLimitExemptDispatch(t *testing.T) {
	handler := NewMethodHandler(nil)

	methods := []string{
		"exemptpubkeyfromratelimit",
		"removeexemptpubkey",
		"listratelimitexemptions",
	}
	for _, m := range methods {
		t.Run(m+" is routed", func(t *testing.T) {
			func() {
				defer func() { recover() }()
				_, err := handler.Dispatch(m, nil)
				if err != nil && err.Error() == "unsupported method: "+m {
					t.Errorf("method %q should be routed but got unsupported", m)
				}
			}()
		})
	}
}

func TestSupportedMethods_IncludesRateLimitExemptions(t *testing.T) {
	expected := map[string]bool{
		"exemptpubkeyfromratelimit": false,
		"removeexemptpubkey":       false,
		"listratelimitexemptions":  false,
	}
	for _, m := range SupportedMethods {
		if _, ok := expected[m]; ok {
			expected[m] = true
		}
	}
	for m, found := range expected {
		if !found {
			t.Errorf("SupportedMethods missing %q", m)
		}
	}
	if len(SupportedMethods) != 23 {
		t.Errorf("expected 23 supported methods (20 original + 3 new), got %d", len(SupportedMethods))
	}
}

func TestMethodHandler_ExemptPubkeyFromRateLimit(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	if err := store.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	_, _ = db.Exec("DELETE FROM management_rate_limit_exempt_pubkeys")

	handler := NewMethodHandler(store)
	pk := "ff00ff00ff00ff00ff00ff00ff00ff00ff00ff00ff00ff00ff00ff00ff00ff00"

	t.Run("add via API", func(t *testing.T) {
		pkJSON, _ := json.Marshal(pk)
		reasonJSON, _ := json.Marshal("test service")
		result, err := handler.Dispatch("exemptpubkeyfromratelimit", []json.RawMessage{pkJSON, reasonJSON})
		if err != nil {
			t.Fatalf("dispatch: %v", err)
		}
		if result != true {
			t.Errorf("expected true, got %v", result)
		}
		if !store.IsRateLimitExempt(pk) {
			t.Error("pubkey should be exempt after API call")
		}
	})

	t.Run("list via API", func(t *testing.T) {
		result, err := handler.Dispatch("listratelimitexemptions", nil)
		if err != nil {
			t.Fatalf("dispatch: %v", err)
		}
		list, ok := result.([]RateLimitExemptPubkey)
		if !ok {
			t.Fatalf("expected []RateLimitExemptPubkey, got %T", result)
		}
		if len(list) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(list))
		}
		if list[0].Pubkey != pk {
			t.Errorf("expected pubkey %q, got %q", pk, list[0].Pubkey)
		}
	})

	t.Run("remove via API", func(t *testing.T) {
		pkJSON, _ := json.Marshal(pk)
		result, err := handler.Dispatch("removeexemptpubkey", []json.RawMessage{pkJSON})
		if err != nil {
			t.Fatalf("dispatch: %v", err)
		}
		if result != true {
			t.Errorf("expected true, got %v", result)
		}
		if store.IsRateLimitExempt(pk) {
			t.Error("pubkey should NOT be exempt after removal")
		}
	})
}

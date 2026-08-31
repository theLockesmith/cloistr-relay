package arbiterclaim

import (
	"encoding/hex"
	"testing"
)

// RFC 5869 Appendix A test vectors for HKDF-SHA256. This implementation emits a
// single 32-byte block, i.e. T(1), so each expectation is the first 32 bytes of
// the RFC's OKM.
func TestHKDF_MatchesRFC5869Vectors(t *testing.T) {
	tests := []struct {
		name, ikm, salt, info, wantT1 string
	}{
		{
			name:   "A.1 basic",
			ikm:    "0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b",
			salt:   "000102030405060708090a0b0c",
			info:   "f0f1f2f3f4f5f6f7f8f9",
			wantT1: "3cb25f25faacd57a90434f64d0362f2a2d2d0a90cf1a5a4c5db02d56ecc4c5bf",
		},
		{
			name:   "A.3 empty salt and info",
			ikm:    "0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b",
			salt:   "",
			info:   "",
			wantT1: "8da4e775a563c18f715f802a063c5a31b8a11f5c5ee1879ec3454e5f3c738d2d",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ikm, _ := hex.DecodeString(tc.ikm)
			salt, _ := hex.DecodeString(tc.salt)
			info, _ := hex.DecodeString(tc.info)
			got := hex.EncodeToString(hkdf(ikm, salt, info))
			if got != tc.wantT1 {
				t.Errorf("hkdf =\n  %s\nwant\n  %s", got, tc.wantT1)
			}
		})
	}
}

// Distinct roles must get distinct keys -- that is the entire point of per-role
// derivation over one shared fleet key.
func TestDeriveRoleKey_RolesAreSeparated(t *testing.T) {
	seed := []byte("fleet-seed-not-a-real-secret")
	roles := []string{"cloistr-relay", "cloistr-orch", "cloistr-space", "cloistr-00"}
	seen := map[string]string{}
	for _, r := range roles {
		k, err := DeriveRoleKey(seed, r)
		if err != nil {
			t.Fatalf("DeriveRoleKey(%q): %v", r, err)
		}
		if len(k) != 64 {
			t.Errorf("role %q: key length %d, want 64 hex chars", r, len(k))
		}
		if prev, dup := seen[k]; dup {
			t.Errorf("roles %q and %q derived the SAME key -- silent claim erasure", r, prev)
		}
		seen[k] = r
	}
}

func TestDeriveRoleKey_IsDeterministicAndSeedBound(t *testing.T) {
	a, err := DeriveRoleKey([]byte("seed-one"), "cloistr-relay")
	if err != nil {
		t.Fatal(err)
	}
	again, _ := DeriveRoleKey([]byte("seed-one"), "cloistr-relay")
	if a != again {
		t.Error("derivation must be deterministic for the same seed and role")
	}
	b, _ := DeriveRoleKey([]byte("seed-two"), "cloistr-relay")
	if a == b {
		t.Error("a different fleet seed must yield a different role key")
	}
}

func TestDeriveRoleKey_RejectsEmptyInputs(t *testing.T) {
	if _, err := DeriveRoleKey(nil, "role"); err == nil {
		t.Error("empty seed must be an error")
	}
	if _, err := DeriveRoleKey([]byte("seed"), ""); err == nil {
		t.Error("empty role must be an error")
	}
}

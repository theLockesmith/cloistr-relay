package arbiterclaim

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/nbd-wtf/go-nostr"
)

// hkdfInfo domain-separates this derivation from any other use of the fleet seed.
const hkdfInfo = "cloistr-arbiter-claim-v1"

// DeriveRoleKey derives a signing key for a ROLE from the fleet seed.
//
// One key per ROLE -- never one shared fleet key, and never one key per session.
//
// A shared fleet key is actively worse than no exclusion at all. Addressable
// events key on (pubkey, kind, d), so with one key every session writes to the
// same address: one session's claim SILENTLY ERASES another's, with no conflict
// and no audit trail. Per-session keys are unbounded and each costs a whitelist
// entry and group membership. Roles are enumerable and are already the
// granularity the fleet addresses each other at, so within-role last-write-wins
// becomes correct behaviour rather than a bug -- a role really is one actor.
func DeriveRoleKey(fleetSeed []byte, role string) (string, error) {
	if len(fleetSeed) == 0 {
		return "", errors.New("arbiterclaim: empty fleet seed")
	}
	if role == "" {
		return "", errors.New("arbiterclaim: empty role")
	}
	// A uniformly random 32 bytes is a valid secp256k1 scalar with overwhelming
	// probability, but "overwhelming" is not "always": retry with a counter
	// rather than emit a key that cannot sign.
	for counter := byte(0); counter < 255; counter++ {
		okm := hkdf(fleetSeed, []byte(hkdfInfo), append([]byte(role), counter))
		sk := hex.EncodeToString(okm)
		if _, err := nostr.GetPublicKey(sk); err == nil {
			return sk, nil
		}
	}
	return "", fmt.Errorf("arbiterclaim: no valid key for role %q after 255 attempts", role)
}

// hkdf is RFC 5869 HKDF-SHA256 producing exactly 32 bytes.
//
// Implemented against the stdlib rather than pulling in golang.org/x/crypto:
// this repo builds CGO_ENABLED=0 against a shared CI template, and a dependency
// added for one 15-line primitive is a pipeline risk for no benefit. Correctness
// is pinned by the RFC 5869 Appendix A test vectors in rolekey_test.go.
func hkdf(ikm, salt, info []byte) []byte {
	// Extract
	h := hmac.New(sha256.New, salt)
	h.Write(ikm)
	prk := h.Sum(nil)

	// Expand, one block: 32 bytes out of SHA-256 needs exactly T(1).
	h = hmac.New(sha256.New, prk)
	h.Write(info)
	h.Write([]byte{0x01})
	return h.Sum(nil)
}

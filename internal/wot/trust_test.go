package wot

import (
	"testing"
)

func TestTrustLevel_String(t *testing.T) {
	tests := []struct {
		level    TrustLevel
		expected string
	}{
		{TrustLevelPaid, "paid"},
		{TrustLevelOwner, "owner"},
		{TrustLevelFollow, "follow"},
		{TrustLevelFollowOfFollow, "follow-of-follow"},
		{TrustLevelUnknown, "unknown"},
		{TrustLevel(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.level.String(); got != tt.expected {
				t.Errorf("TrustLevel.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDefaultPolicies(t *testing.T) {
	policies := DefaultPolicies()

	// Verify all levels have policies
	levels := []TrustLevel{
		TrustLevelPaid,
		TrustLevelOwner,
		TrustLevelFollow,
		TrustLevelFollowOfFollow,
		TrustLevelUnknown,
	}

	for _, level := range levels {
		policy, ok := policies[level]
		if !ok {
			t.Errorf("Missing policy for trust level %s", level)
			continue
		}

		// Paid, owner, and follow should not require PoW
		if (level == TrustLevelPaid || level <= TrustLevelFollow) && policy.RequirePoW {
			t.Errorf("Trust level %s should not require PoW", level)
		}

		// Unknown should NOT require PoW by default (the env var override
		// sets RequirePoW=true when WOT_UNKNOWN_POW_BITS > 0).
		if level == TrustLevelUnknown && policy.RequirePoW {
			t.Errorf("Trust level %s should not require PoW by default", level)
		}
	}
}

func TestAllowedPubkeysBypassPoW(t *testing.T) {
	// Create a handler with an allowed pubkey
	allowedPubkey := "532aceee51a63b3a7a242aca4e0b79f57352046b8743d0ea1833d135d2034ce6"
	untrustedPubkey := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	cfg := &Config{
		Enabled:        true,
		OwnerPubkey:    "0000000000000000000000000000000000000000000000000000000000000001",
		Policies:       DefaultPolicies(),
		AllowedPubkeys: []string{allowedPubkey},
	}

	// NewHandler builds the allowedPubkeys map
	handler := NewHandler(nil, []string{cfg.OwnerPubkey}, cfg)

	// Verify allowed pubkey is in the map
	if _, ok := handler.allowedPubkeys[allowedPubkey]; !ok {
		t.Error("allowed pubkey should be in the map")
	}

	// Verify untrusted pubkey is not in the map
	if _, ok := handler.allowedPubkeys[untrustedPubkey]; ok {
		t.Error("untrusted pubkey should not be in the map")
	}
}

func TestPaidMemberBypassesGraph(t *testing.T) {
	// A paid member should get TrustLevelPaid regardless of graph position.
	// We only test the paid path here — the graph path needs a real store
	// and is covered by integration tests.
	paidPubkey := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	ownerPubkey := "0000000000000000000000000000000000000000000000000000000000000001"

	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
		Policies:    DefaultPolicies(),
		IsPaidMember: func(pubkey string) bool {
			return pubkey == paidPubkey
		},
	}

	handler := NewHandler(nil, []string{ownerPubkey}, cfg)

	// Paid member gets TrustLevelPaid
	level := handler.getTrustLevel(paidPubkey)
	if level != TrustLevelPaid {
		t.Errorf("paid member got trust level %s, want paid", level)
	}

	// Owner still gets TrustLevelOwner (paid check runs first but owner
	// is not in the IsPaidMember set — the graph check handles it)
	// Can't test this without a store, but the paid check itself is verified above.

	// Paid policy should not require PoW
	policy := handler.policies[TrustLevelPaid]
	if policy.RequirePoW {
		t.Error("paid policy should not require PoW")
	}
}

func TestMultiRootTrustCalculator(t *testing.T) {
	root1 := "1111111111111111111111111111111111111111111111111111111111111111"
	root2 := "2222222222222222222222222222222222222222222222222222222222222222"
	outsider := "3333333333333333333333333333333333333333333333333333333333333333"

	// No store — just test the root check in calculateTrustLevel
	calc := NewTrustCalculator(nil, []string{root1, root2}, 2)

	// Both roots should be owner-level
	if level := calc.calculateTrustLevel(root1); level != TrustLevelOwner {
		t.Errorf("root1 got %s, want owner", level)
	}
	if level := calc.calculateTrustLevel(root2); level != TrustLevelOwner {
		t.Errorf("root2 got %s, want owner", level)
	}

	// An outsider should be unknown (no store to check follows)
	// This panics without a store, so skip the follow checks
	// The root-is-owner check is the key thing to verify here
	_ = outsider
}

func TestMultiRootPageRank(t *testing.T) {
	root1 := "root1"
	root2 := "root2"

	calc := &PageRankCalculator{
		trustRoots: map[string]struct{}{root1: {}, root2: {}},
		config:     DefaultPageRankConfig(),
		scores: map[string]float64{
			root1:         1.0,
			root2:         0.9,
			"high_trust":  0.05,
			"unknown_key": 0.00001,
		},
	}

	// Both roots are owner-level
	if level := calc.GetTrustLevelFromPageRank(root1); level != TrustLevelOwner {
		t.Errorf("root1 got %s, want owner", level)
	}
	if level := calc.GetTrustLevelFromPageRank(root2); level != TrustLevelOwner {
		t.Errorf("root2 got %s, want owner", level)
	}

	// High-trust node is Follow
	if level := calc.GetTrustLevelFromPageRank("high_trust"); level != TrustLevelFollow {
		t.Errorf("high_trust got %s, want follow", level)
	}
}

func TestNilIsPaidMemberSafe(t *testing.T) {
	// With no IsPaidMember set, the handler should not panic on the paid check.
	// We test by checking that the paid path returns false for a pubkey that
	// would otherwise need the graph, but we can't call getTrustLevel without
	// a store. Instead verify the handler was constructed without panic and
	// that isPaidMember is nil.
	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: "0000000000000000000000000000000000000000000000000000000000000001",
		Policies:    DefaultPolicies(),
	}

	handler := NewHandler(nil, []string{cfg.OwnerPubkey}, cfg)

	if handler.isPaidMember != nil {
		t.Error("isPaidMember should be nil when not configured")
	}
}

func TestCountLeadingZeroBits(t *testing.T) {
	tests := []struct {
		hexID    string
		expected int
	}{
		{"0000000000000000", 64},  // All zeros
		{"f000000000000000", 0},   // No leading zeros
		{"0f00000000000000", 4},   // 4 bits
		{"00ff000000000000", 8},   // 8 bits
		{"001f000000000000", 11},  // 11 bits
		{"0007000000000000", 13},  // 13 bits
		{"0001000000000000", 15},  // 15 bits
		{"0000800000000000", 16},  // 16 bits (8 nibble = 0, then 8 = 1000)
		{"0000100000000000", 19},  // 19 bits
	}

	for _, tt := range tests {
		t.Run(tt.hexID, func(t *testing.T) {
			got := countLeadingZeroBits(tt.hexID)
			if got != tt.expected {
				t.Errorf("countLeadingZeroBits(%s) = %d, want %d", tt.hexID, got, tt.expected)
			}
		})
	}
}

package wot

import (
	"testing"
)

func TestTrustLevel_String(t *testing.T) {
	tests := []struct {
		level    TrustLevel
		expected string
	}{
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

		// Owner and follow should not require PoW
		if level <= TrustLevelFollow && policy.RequirePoW {
			t.Errorf("Trust level %s should not require PoW", level)
		}

		// Unknown should require PoW
		if level == TrustLevelUnknown && !policy.RequirePoW {
			t.Errorf("Trust level %s should require PoW", level)
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
	handler := NewHandler(nil, cfg.OwnerPubkey, cfg)

	// Verify allowed pubkey is in the map
	if _, ok := handler.allowedPubkeys[allowedPubkey]; !ok {
		t.Error("allowed pubkey should be in the map")
	}

	// Verify untrusted pubkey is not in the map
	if _, ok := handler.allowedPubkeys[untrustedPubkey]; ok {
		t.Error("untrusted pubkey should not be in the map")
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

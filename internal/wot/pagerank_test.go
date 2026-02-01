package wot

import (
	"testing"
	"time"
)

func TestPageRankCalculator_GetTrustLevelFromPageRank(t *testing.T) {
	// Create a calculator with no store (for testing threshold logic only)
	cfg := DefaultPageRankConfig()
	calc := &PageRankCalculator{
		ownerPubkey: "owner123",
		config:      cfg,
		scores: map[string]float64{
			"owner123":     1.0,   // Owner
			"high_trust":   0.05,  // Above follow threshold
			"medium_trust": 0.001, // At follow threshold
			"low_trust":    0.0005, // Between thresholds
			"unknown_user": 0.00001, // Below all thresholds
		},
	}

	tests := []struct {
		pubkey   string
		expected TrustLevel
	}{
		{"owner123", TrustLevelOwner},
		{"high_trust", TrustLevelFollow},
		{"medium_trust", TrustLevelFollow},
		{"low_trust", TrustLevelFollowOfFollow},
		{"unknown_user", TrustLevelUnknown},
		{"nonexistent", TrustLevelUnknown}, // Not in scores
	}

	for _, tt := range tests {
		t.Run(tt.pubkey, func(t *testing.T) {
			result := calc.GetTrustLevelFromPageRank(tt.pubkey)
			if result != tt.expected {
				t.Errorf("GetTrustLevelFromPageRank(%s) = %v, want %v", tt.pubkey, result, tt.expected)
			}
		})
	}
}

func TestDefaultPageRankConfig(t *testing.T) {
	cfg := DefaultPageRankConfig()

	if !cfg.Enabled {
		t.Error("Expected Enabled to be true")
	}
	if cfg.ComputeInterval != 1*time.Hour {
		t.Errorf("Expected ComputeInterval to be 1h, got %v", cfg.ComputeInterval)
	}
	if cfg.OwnerBoost != 2.0 {
		t.Errorf("Expected OwnerBoost to be 2.0, got %v", cfg.OwnerBoost)
	}
	if cfg.TrustThresholds.FollowThreshold != 0.001 {
		t.Errorf("Expected FollowThreshold to be 0.001, got %v", cfg.TrustThresholds.FollowThreshold)
	}
}

func TestPageRankCalculator_GetPageRank(t *testing.T) {
	calc := &PageRankCalculator{
		ownerPubkey: "owner",
		config:      DefaultPageRankConfig(),
		scores: map[string]float64{
			"alice": 0.5,
			"bob":   0.25,
		},
	}

	tests := []struct {
		pubkey   string
		expected float64
	}{
		{"alice", 0.5},
		{"bob", 0.25},
		{"charlie", 0.0}, // Not in scores
	}

	for _, tt := range tests {
		t.Run(tt.pubkey, func(t *testing.T) {
			result := calc.GetPageRank(tt.pubkey)
			if result != tt.expected {
				t.Errorf("GetPageRank(%s) = %v, want %v", tt.pubkey, result, tt.expected)
			}
		})
	}
}

func TestPageRankAlgorithmConstants(t *testing.T) {
	// Verify algorithm constants are reasonable
	if dampingFactor < 0 || dampingFactor > 1 {
		t.Errorf("dampingFactor should be between 0 and 1, got %v", dampingFactor)
	}
	if maxIterations < 10 {
		t.Errorf("maxIterations should be at least 10, got %d", maxIterations)
	}
	if convergenceEps <= 0 {
		t.Errorf("convergenceEps should be positive, got %v", convergenceEps)
	}
}

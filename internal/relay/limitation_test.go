package relay

import (
	"testing"

	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/config"
)

func TestBuildLimitation_Defaults(t *testing.T) {
	cfg := &config.Config{
		MaxMessageSize:     512000,
		MinPoWDifficulty:   0,
		MaxCreatedAtFuture: 300,
	}

	lim := buildLimitation(cfg)
	if lim.MaxMessageLength != 512000 {
		t.Errorf("MaxMessageLength = %d, want 512000", lim.MaxMessageLength)
	}
	if lim.MinPowDifficulty != 0 {
		t.Errorf("MinPowDifficulty = %d, want 0", lim.MinPowDifficulty)
	}
	if lim.AuthRequired {
		t.Error("AuthRequired should be false when AuthPolicy is empty")
	}
	if lim.CreatedAtUpperLimit != 300 {
		t.Errorf("CreatedAtUpperLimit = %d, want 300", lim.CreatedAtUpperLimit)
	}
	if lim.RestrictedWrites {
		t.Error("RestrictedWrites should be false when whitelist is empty")
	}
}

func TestBuildLimitation_AuthWrite(t *testing.T) {
	cfg := &config.Config{
		MaxMessageSize: 512000,
		AuthPolicy:     "auth-write",
	}
	lim := buildLimitation(cfg)
	if !lim.AuthRequired {
		t.Error("AuthRequired should be true for auth-write")
	}
}

func TestBuildLimitation_AuthAll(t *testing.T) {
	cfg := &config.Config{
		MaxMessageSize: 512000,
		AuthPolicy:     "auth-all",
	}
	lim := buildLimitation(cfg)
	if !lim.AuthRequired {
		t.Error("AuthRequired should be true for auth-all")
	}
}

func TestBuildLimitation_Open(t *testing.T) {
	cfg := &config.Config{
		MaxMessageSize: 512000,
		AuthPolicy:     "open",
	}
	lim := buildLimitation(cfg)
	if lim.AuthRequired {
		t.Error("AuthRequired should be false for open")
	}
}

func TestBuildLimitation_RestrictedWrites(t *testing.T) {
	cfg := &config.Config{
		MaxMessageSize:        512000,
		WriteWhitelistPubkeys: []string{"abc123"},
	}
	lim := buildLimitation(cfg)
	if !lim.RestrictedWrites {
		t.Error("RestrictedWrites should be true when whitelist is non-empty")
	}
}

func TestBuildLimitation_CustomSize(t *testing.T) {
	cfg := &config.Config{
		MaxMessageSize: 1048576,
	}
	lim := buildLimitation(cfg)
	if lim.MaxMessageLength != 1048576 {
		t.Errorf("MaxMessageLength = %d, want 1048576", lim.MaxMessageLength)
	}
}

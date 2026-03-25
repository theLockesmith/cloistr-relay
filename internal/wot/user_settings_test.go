package wot

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

func TestUserSettings_IsBlocked(t *testing.T) {
	settings := &UserSettings{
		Pubkey:         "user1",
		BlockedPubkeys: []string{"blocked1", "blocked2"},
	}

	tests := []struct {
		pubkey   string
		expected bool
	}{
		{"blocked1", true},
		{"blocked2", true},
		{"notblocked", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := settings.IsBlocked(tt.pubkey); got != tt.expected {
			t.Errorf("IsBlocked(%s) = %v, want %v", tt.pubkey, got, tt.expected)
		}
	}
}

func TestUserSettings_IsTrusted(t *testing.T) {
	settings := &UserSettings{
		Pubkey:         "user1",
		TrustedPubkeys: []string{"trusted1", "trusted2"},
	}

	tests := []struct {
		pubkey   string
		expected bool
	}{
		{"trusted1", true},
		{"trusted2", true},
		{"nottrusted", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := settings.IsTrusted(tt.pubkey); got != tt.expected {
			t.Errorf("IsTrusted(%s) = %v, want %v", tt.pubkey, got, tt.expected)
		}
	}
}

func TestUserSettings_GetEffectiveMaxTrustDepth(t *testing.T) {
	relayDefault := 2

	// nil uses relay default
	settings1 := &UserSettings{Pubkey: "user1"}
	if got := settings1.GetEffectiveMaxTrustDepth(relayDefault); got != relayDefault {
		t.Errorf("nil MaxTrustDepth should use relay default %d, got %d", relayDefault, got)
	}

	// User override
	depth := 3
	settings2 := &UserSettings{Pubkey: "user2", MaxTrustDepth: &depth}
	if got := settings2.GetEffectiveMaxTrustDepth(relayDefault); got != depth {
		t.Errorf("MaxTrustDepth override should return %d, got %d", depth, got)
	}
}

func TestUserSettings_GetEffectiveMinPowBits(t *testing.T) {
	relayDefault := 8

	// nil uses relay default
	settings1 := &UserSettings{Pubkey: "user1"}
	if got := settings1.GetEffectiveMinPowBits(relayDefault); got != relayDefault {
		t.Errorf("nil MinPowBits should use relay default %d, got %d", relayDefault, got)
	}

	// User override
	bits := 12
	settings2 := &UserSettings{Pubkey: "user2", MinPowBits: &bits}
	if got := settings2.GetEffectiveMinPowBits(relayDefault); got != bits {
		t.Errorf("MinPowBits override should return %d, got %d", bits, got)
	}
}

func TestFilterResult_Sources(t *testing.T) {
	sources := []FilterSource{
		FilterSourceRelayBlock,
		FilterSourceUserBlock,
		FilterSourceUserTrust,
		FilterSourceRelayWoT,
		FilterSourceDefault,
	}

	for _, source := range sources {
		if source == "" {
			t.Error("FilterSource should not be empty")
		}
	}
}

func TestUserFilter_ShouldAllowToInbox_NoStore(t *testing.T) {
	// Without a settings store, everything should be allowed by default
	filter := NewUserFilter(nil, nil)

	// Test with nil event (edge case)
	result := filter.ShouldAllowToInbox(nil, nil, "recipient")
	if !result.Allowed {
		t.Error("Without settings store and nil event, should allow by default")
	}
	if result.Source != FilterSourceDefault {
		t.Errorf("Source should be default, got %s", result.Source)
	}
}

func TestUserFilter_ShouldAllowToInbox_WithEvent(t *testing.T) {
	filter := NewUserFilter(nil, nil)

	event := &nostr.Event{
		PubKey: "sender123",
		Kind:   1,
	}

	result := filter.ShouldAllowToInbox(nil, event, "recipient456")

	if !result.Allowed {
		t.Error("Without settings store, should allow by default")
	}
	if result.Source != FilterSourceDefault {
		t.Errorf("Source should be default, got %s", result.Source)
	}
}

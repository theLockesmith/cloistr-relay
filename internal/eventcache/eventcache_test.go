package eventcache

import (
	"context"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.Enabled {
		t.Error("Expected Enabled to be true by default")
	}
	if cfg.DefaultTTL != 5*time.Minute {
		t.Errorf("Expected DefaultTTL to be 5m, got %v", cfg.DefaultTTL)
	}
	if cfg.ProfileTTL != 15*time.Minute {
		t.Errorf("Expected ProfileTTL to be 15m, got %v", cfg.ProfileTTL)
	}
	if cfg.ContactsTTL != 10*time.Minute {
		t.Errorf("Expected ContactsTTL to be 10m, got %v", cfg.ContactsTTL)
	}
	if cfg.KeyPrefix != "event:" {
		t.Errorf("Expected KeyPrefix to be 'event:', got %s", cfg.KeyPrefix)
	}
	if cfg.MaxCacheSize != 100000 {
		t.Errorf("Expected MaxCacheSize to be 100000, got %d", cfg.MaxCacheSize)
	}
}

func TestCache_New(t *testing.T) {
	cache := New(nil, nil)
	if cache == nil {
		t.Fatal("Expected non-nil cache")
	}
	if cache.config == nil {
		t.Fatal("Expected non-nil config")
	}
	if cache.config.DefaultTTL != 5*time.Minute {
		t.Errorf("Expected default DefaultTTL, got %v", cache.config.DefaultTTL)
	}
}

func TestCache_EventKey(t *testing.T) {
	cache := New(nil, nil)

	key := cache.eventKey("abc123")
	expected := "event:id:abc123"
	if key != expected {
		t.Errorf("Expected %s, got %s", expected, key)
	}
}

func TestCache_PubkeyKindKey(t *testing.T) {
	cache := New(nil, nil)

	key := cache.pubkeyKindKey("pubkey123", 0)
	expected := "event:pk:pubkey123:0"
	if key != expected {
		t.Errorf("Expected %s, got %s", expected, key)
	}

	key = cache.pubkeyKindKey("pubkey456", 3)
	expected = "event:pk:pubkey456:3"
	if key != expected {
		t.Errorf("Expected %s, got %s", expected, key)
	}
}

func TestCache_GetTTL(t *testing.T) {
	cache := New(nil, nil)

	// Profile (kind 0)
	if ttl := cache.getTTL(0); ttl != 15*time.Minute {
		t.Errorf("Expected 15m for kind 0, got %v", ttl)
	}

	// Contacts (kind 3)
	if ttl := cache.getTTL(3); ttl != 10*time.Minute {
		t.Errorf("Expected 10m for kind 3, got %v", ttl)
	}

	// Regular event
	if ttl := cache.getTTL(1); ttl != 5*time.Minute {
		t.Errorf("Expected 5m for kind 1, got %v", ttl)
	}
}

func TestCache_SetWithoutRedis(t *testing.T) {
	cache := New(nil, DefaultConfig())

	// Should not error without Redis
	err := cache.Set(context.TODO(), nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestCache_GetWithoutRedis(t *testing.T) {
	cache := New(nil, DefaultConfig())

	event, err := cache.Get(context.TODO(), "nonexistent")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if event != nil {
		t.Error("Expected nil event without Redis")
	}
}

func TestCache_GetByPubkeyKindWithoutRedis(t *testing.T) {
	cache := New(nil, DefaultConfig())

	event, err := cache.GetByPubkeyKind(context.TODO(), "pubkey", 0)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if event != nil {
		t.Error("Expected nil event without Redis")
	}
}

func TestCache_GetProfileWithoutRedis(t *testing.T) {
	cache := New(nil, DefaultConfig())

	event, err := cache.GetProfile(context.TODO(), "pubkey")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if event != nil {
		t.Error("Expected nil event without Redis")
	}
}

func TestCache_GetContactsWithoutRedis(t *testing.T) {
	cache := New(nil, DefaultConfig())

	event, err := cache.GetContacts(context.TODO(), "pubkey")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if event != nil {
		t.Error("Expected nil event without Redis")
	}
}

func TestCache_DeleteWithoutRedis(t *testing.T) {
	cache := New(nil, DefaultConfig())

	err := cache.Delete(context.TODO(), "eventid")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestCache_MultiGetWithoutRedis(t *testing.T) {
	cache := New(nil, DefaultConfig())

	events, err := cache.MultiGet(context.TODO(), []string{"id1", "id2"})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if events != nil {
		t.Error("Expected nil events without Redis")
	}
}

func TestCache_InvalidateWithoutRedis(t *testing.T) {
	cache := New(nil, DefaultConfig())

	err := cache.Invalidate(context.TODO(), "pubkey", 0)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestCache_GetStatsWithoutRedis(t *testing.T) {
	cache := New(nil, DefaultConfig())

	stats, err := cache.GetStats(context.TODO())
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if stats == nil {
		t.Fatal("Expected non-nil stats")
	}
	if stats.EventCount != 0 {
		t.Errorf("Expected 0 events without Redis, got %d", stats.EventCount)
	}
}

func TestIsCacheable(t *testing.T) {
	tests := []struct {
		kind     int
		expected bool
	}{
		{0, true},          // Profile
		{3, true},          // Contacts
		{10002, true},      // Relay list
		{1, false},         // Regular note
		{4, false},         // DM
		{30023, true},      // Parameterized replaceable (article)
		{30000, true},      // Parameterized replaceable start
		{39999, true},      // Parameterized replaceable end
		{40000, false},     // Out of parameterized range
		{9735, false},      // Zap receipt
	}

	for _, tt := range tests {
		t.Run("kind_"+string(rune(tt.kind+'0')), func(t *testing.T) {
			result := IsCacheable(tt.kind)
			if result != tt.expected {
				t.Errorf("IsCacheable(%d) = %v, expected %v", tt.kind, result, tt.expected)
			}
		})
	}
}

func TestIsReplaceable(t *testing.T) {
	tests := []struct {
		kind     int
		expected bool
	}{
		{0, true},          // Profile
		{3, true},          // Contacts
		{1, false},         // Regular note
		{4, false},         // DM
		{10000, true},      // Replaceable start
		{10002, true},      // Relay list
		{19999, true},      // Replaceable end
		{20000, false},     // Ephemeral start
		{30000, true},      // Parameterized replaceable start
		{30023, true},      // Article
		{39999, true},      // Parameterized replaceable end
		{40000, false},     // Out of range
	}

	for _, tt := range tests {
		t.Run("kind", func(t *testing.T) {
			result := isReplaceable(tt.kind)
			if result != tt.expected {
				t.Errorf("isReplaceable(%d) = %v, expected %v", tt.kind, result, tt.expected)
			}
		})
	}
}

func TestCacheableKinds(t *testing.T) {
	if !CacheableKinds[0] {
		t.Error("Expected kind 0 to be cacheable")
	}
	if !CacheableKinds[3] {
		t.Error("Expected kind 3 to be cacheable")
	}
	if !CacheableKinds[10002] {
		t.Error("Expected kind 10002 to be cacheable")
	}
	if CacheableKinds[1] {
		t.Error("Expected kind 1 to not be in CacheableKinds map")
	}
}

// Integration tests would require a real Redis/Dragonfly connection
// These are left as structural tests that validate the API without Redis

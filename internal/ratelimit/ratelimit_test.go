package ratelimit

import (
	"net/http"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.Enabled {
		t.Error("Expected Enabled to be true by default")
	}
	if cfg.EventsPerSecond != 10 {
		t.Errorf("Expected EventsPerSecond to be 10, got %d", cfg.EventsPerSecond)
	}
	if cfg.FiltersPerSecond != 20 {
		t.Errorf("Expected FiltersPerSecond to be 20, got %d", cfg.FiltersPerSecond)
	}
	if cfg.ConnectionsPerSecond != 5 {
		t.Errorf("Expected ConnectionsPerSecond to be 5, got %d", cfg.ConnectionsPerSecond)
	}
	if cfg.BurstMultiplier != 5 {
		t.Errorf("Expected BurstMultiplier to be 5, got %d", cfg.BurstMultiplier)
	}
	if cfg.WindowSize != time.Second {
		t.Errorf("Expected WindowSize to be 1s, got %v", cfg.WindowSize)
	}
}

func TestLimiter_New(t *testing.T) {
	limiter := New(nil, nil)
	if limiter == nil {
		t.Error("Expected non-nil limiter")
	}
	if limiter.config == nil {
		t.Error("Expected non-nil config")
	}
	// With nil Redis client, should use defaults
	if limiter.config.EventsPerSecond != 10 {
		t.Errorf("Expected default EventsPerSecond, got %d", limiter.config.EventsPerSecond)
	}
}

func TestLimiter_KeyPrefix(t *testing.T) {
	limiter := New(nil, nil)

	tests := []struct {
		limitType string
		expected  string
	}{
		{"events", "ratelimit:events:"},
		{"filters", "ratelimit:filters:"},
		{"connections", "ratelimit:connections:"},
	}

	for _, tt := range tests {
		t.Run(tt.limitType, func(t *testing.T) {
			result := limiter.keyPrefix(tt.limitType)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestLimiter_AllowWithoutRedis(t *testing.T) {
	// Without Redis, should always allow (fail open)
	limiter := New(nil, DefaultConfig())

	allowed, err := limiter.Allow(nil, "events", "192.168.1.1", 10)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !allowed {
		t.Error("Expected request to be allowed without Redis")
	}
}

func TestLimiter_AllowDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false
	limiter := New(nil, cfg)

	allowed, err := limiter.Allow(nil, "events", "192.168.1.1", 10)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !allowed {
		t.Error("Expected request to be allowed when disabled")
	}
}

func TestLimiter_RejectEventByRateLimit_Disabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EventsPerSecond = 0 // Disabled
	limiter := New(nil, cfg)

	rejectFn := limiter.RejectEventByRateLimit()
	reject, msg := rejectFn(nil, nil)

	if reject {
		t.Errorf("Expected no rejection when disabled, got: %s", msg)
	}
}

func TestLimiter_RejectFilterByRateLimit_Disabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FiltersPerSecond = 0 // Disabled
	limiter := New(nil, cfg)

	rejectFn := limiter.RejectFilterByRateLimit()
	reject, msg := rejectFn(nil, nostr.Filter{})

	if reject {
		t.Errorf("Expected no rejection when disabled, got: %s", msg)
	}
}

func TestLimiter_RejectConnectionByRateLimit_Disabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ConnectionsPerSecond = 0 // Disabled
	limiter := New(nil, cfg)

	rejectFn := limiter.RejectConnectionByRateLimit()
	// Create a minimal request for testing
	req, _ := http.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	reject := rejectFn(req)

	if reject {
		t.Error("Expected no rejection when disabled")
	}
}

// Integration tests would require a real Redis/Dragonfly connection
// These are left as structural tests that validate the API without Redis

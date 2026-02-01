package giftwrap

import (
	"context"
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

func TestKindConstants(t *testing.T) {
	if KindSeal != 13 {
		t.Errorf("KindSeal should be 13, got %d", KindSeal)
	}
	if KindGiftWrap != 1059 {
		t.Errorf("KindGiftWrap should be 1059, got %d", KindGiftWrap)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Enabled {
		t.Error("Expected Enabled to be true by default")
	}
	if !cfg.RequireAuthForGiftWrap {
		t.Error("Expected RequireAuthForGiftWrap to be true by default")
	}
}

func TestRejectGiftWrapFilter_NoGiftWrap(t *testing.T) {
	handler := NewHandler(DefaultConfig())
	rejectFn := handler.RejectGiftWrapFilter()

	// Filter without gift wrap kind should pass
	filter := nostr.Filter{
		Kinds: []int{1, 4, 7},
	}

	reject, msg := rejectFn(context.Background(), filter)
	if reject {
		t.Errorf("Expected non-gift-wrap filter to pass, got rejection: %s", msg)
	}
}

func TestRejectGiftWrapFilter_NoAuth(t *testing.T) {
	handler := NewHandler(DefaultConfig())
	rejectFn := handler.RejectGiftWrapFilter()

	// Filter with gift wrap but no auth should be rejected
	filter := nostr.Filter{
		Kinds: []int{KindGiftWrap},
	}

	reject, msg := rejectFn(context.Background(), filter)
	if !reject {
		t.Error("Expected gift wrap filter without auth to be rejected")
	}
	if msg != "auth-required: authentication required to query gift wrap events" {
		t.Errorf("Unexpected rejection message: %s", msg)
	}
}

func TestRejectGiftWrapFilter_AuthDisabled(t *testing.T) {
	cfg := &Config{
		Enabled:                true,
		RequireAuthForGiftWrap: false,
	}
	handler := NewHandler(cfg)
	rejectFn := handler.RejectGiftWrapFilter()

	// Filter with gift wrap but auth disabled should pass
	filter := nostr.Filter{
		Kinds: []int{KindGiftWrap},
	}

	reject, msg := rejectFn(context.Background(), filter)
	if reject {
		t.Errorf("Expected gift wrap filter with auth disabled to pass, got: %s", msg)
	}
}

func TestGetRecipient(t *testing.T) {
	tests := []struct {
		name     string
		event    *nostr.Event
		expected string
	}{
		{
			name: "with p tag",
			event: &nostr.Event{
				Tags: nostr.Tags{
					{"p", "abc123def456789012345678901234567890123456789012345678901234"},
				},
			},
			expected: "abc123de...",
		},
		{
			name: "no p tag",
			event: &nostr.Event{
				Tags: nostr.Tags{
					{"e", "eventid"},
				},
			},
			expected: "unknown",
		},
		{
			name:     "empty tags",
			event:    &nostr.Event{},
			expected: "unknown",
		},
		{
			name: "short p tag",
			event: &nostr.Event{
				Tags: nostr.Tags{
					{"p", "abc"},
				},
			},
			expected: "abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getRecipient(tt.event)
			if result != tt.expected {
				t.Errorf("getRecipient() = %s, want %s", result, tt.expected)
			}
		})
	}
}

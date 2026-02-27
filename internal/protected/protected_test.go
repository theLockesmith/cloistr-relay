package protected

import (
	"context"
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.Enabled {
		t.Error("Expected Enabled to be true by default")
	}
	if !cfg.AllowProtectedEvents {
		t.Error("Expected AllowProtectedEvents to be true by default")
	}
}

func TestHandler_New(t *testing.T) {
	handler := NewHandler(nil)
	if handler == nil {
		t.Fatal("Expected non-nil handler")
	}
	if handler.config == nil {
		t.Fatal("Expected non-nil config")
	}
	if !handler.config.AllowProtectedEvents {
		t.Error("Expected AllowProtectedEvents to be true by default")
	}
}

func TestIsProtected(t *testing.T) {
	tests := []struct {
		name     string
		event    *nostr.Event
		expected bool
	}{
		{
			name: "not protected - no tags",
			event: &nostr.Event{
				Tags: nostr.Tags{},
			},
			expected: false,
		},
		{
			name: "not protected - other tags",
			event: &nostr.Event{
				Tags: nostr.Tags{
					{"p", "somepubkey"},
					{"e", "someeventid"},
				},
			},
			expected: false,
		},
		{
			name: "protected - dash tag alone",
			event: &nostr.Event{
				Tags: nostr.Tags{
					{"-"},
				},
			},
			expected: true,
		},
		{
			name: "protected - dash tag with other tags",
			event: &nostr.Event{
				Tags: nostr.Tags{
					{"p", "somepubkey"},
					{"-"},
					{"e", "someeventid"},
				},
			},
			expected: true,
		},
		{
			name: "protected - dash tag with value",
			event: &nostr.Event{
				Tags: nostr.Tags{
					{"-", "somevalue"},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsProtected(tt.event)
			if result != tt.expected {
				t.Errorf("IsProtected() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestRejectProtectedEvent_Disabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false
	handler := NewHandler(cfg)

	protectedEvent := &nostr.Event{
		PubKey: "abc123def456789012345678901234567890123456789012345678901234",
		Tags:   nostr.Tags{{"-"}},
	}

	rejectFn := handler.RejectProtectedEvent()
	reject, msg := rejectFn(context.Background(), protectedEvent)

	if reject {
		t.Errorf("Expected no rejection when disabled, got: %s", msg)
	}
}

func TestRejectProtectedEvent_NotProtected(t *testing.T) {
	handler := NewHandler(DefaultConfig())

	normalEvent := &nostr.Event{
		PubKey: "abc123def456789012345678901234567890123456789012345678901234",
		Tags:   nostr.Tags{{"p", "somepubkey"}},
	}

	rejectFn := handler.RejectProtectedEvent()
	reject, msg := rejectFn(context.Background(), normalEvent)

	if reject {
		t.Errorf("Expected no rejection for non-protected event, got: %s", msg)
	}
}

func TestRejectProtectedEvent_NotAllowed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AllowProtectedEvents = false
	handler := NewHandler(cfg)

	protectedEvent := &nostr.Event{
		PubKey: "abc123def456789012345678901234567890123456789012345678901234",
		Tags:   nostr.Tags{{"-"}},
	}

	rejectFn := handler.RejectProtectedEvent()
	reject, msg := rejectFn(context.Background(), protectedEvent)

	if !reject {
		t.Error("Expected rejection when AllowProtectedEvents is false")
	}
	if msg != "blocked: this relay does not accept protected events" {
		t.Errorf("Unexpected rejection message: %s", msg)
	}
}

func TestRejectProtectedEvent_NoAuth(t *testing.T) {
	handler := NewHandler(DefaultConfig())

	protectedEvent := &nostr.Event{
		PubKey: "abc123def456789012345678901234567890123456789012345678901234",
		Tags:   nostr.Tags{{"-"}},
	}

	// Background context has no auth
	rejectFn := handler.RejectProtectedEvent()
	reject, msg := rejectFn(context.Background(), protectedEvent)

	if !reject {
		t.Error("Expected rejection when not authenticated")
	}
	if msg != "auth-required: authentication required to publish protected events" {
		t.Errorf("Unexpected rejection message: %s", msg)
	}
}

// Note: Testing with authenticated context requires mocking khatru.GetAuthed
// which is difficult without integration tests. The above tests cover the
// core logic paths.

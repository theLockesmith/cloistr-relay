package haven

import (
	"context"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

func TestNewSubscriber(t *testing.T) {
	cfg := &Config{
		Enabled:                 true,
		OwnerPubkey:             "owner123",
		ImporterEnabled:         true,
		ImporterRealtimeEnabled: true,
		ImporterRelays:          []string{"wss://relay1.example.com", "wss://relay2.example.com"},
	}

	storeFunc := func(ctx context.Context, event *nostr.Event) error {
		return nil
	}

	sub := NewSubscriber(cfg, storeFunc)
	if sub == nil {
		t.Fatal("NewSubscriber returned nil")
	}

	if sub.config != cfg {
		t.Error("config not set correctly")
	}

	if sub.storeFunc == nil {
		t.Error("storeFunc not set")
	}

	if len(sub.seenEvents) != 0 {
		t.Error("seenEvents should be empty initially")
	}
}

func TestSubscriber_Stats(t *testing.T) {
	cfg := &Config{
		Enabled:                 true,
		OwnerPubkey:             "owner123",
		ImporterEnabled:         true,
		ImporterRealtimeEnabled: true,
		ImporterRelays:          []string{"wss://relay1.example.com"},
	}

	sub := NewSubscriber(cfg, nil)

	stats := sub.Stats()
	if stats.EventsReceived != 0 {
		t.Errorf("expected EventsReceived=0, got %d", stats.EventsReceived)
	}
	if stats.EventsImported != 0 {
		t.Errorf("expected EventsImported=0, got %d", stats.EventsImported)
	}
	if stats.RelaysTotal != 0 {
		t.Errorf("expected RelaysTotal=0 before Start, got %d", stats.RelaysTotal)
	}
}

func TestSubscriber_RelayStatus(t *testing.T) {
	cfg := &Config{
		Enabled:                 true,
		OwnerPubkey:             "owner123",
		ImporterEnabled:         true,
		ImporterRealtimeEnabled: true,
		ImporterRelays:          []string{"wss://relay1.example.com", "wss://relay2.example.com"},
	}

	sub := NewSubscriber(cfg, nil)

	// Before Start(), relays map is empty
	status := sub.RelayStatus()
	if len(status) != 0 {
		t.Errorf("expected 0 relay statuses before Start, got %d", len(status))
	}
}

func TestSubscriber_DisabledWhenNoRelays(t *testing.T) {
	cfg := &Config{
		Enabled:                 true,
		OwnerPubkey:             "owner123",
		ImporterEnabled:         true,
		ImporterRealtimeEnabled: true,
		ImporterRelays:          []string{}, // No relays
	}

	sub := NewSubscriber(cfg, func(ctx context.Context, event *nostr.Event) error {
		return nil
	})

	// Start should return immediately without starting workers
	sub.Start()
	// Stop should not panic
	sub.Stop()
}

func TestSubscriber_DisabledWhenRealtimeOff(t *testing.T) {
	cfg := &Config{
		Enabled:                 true,
		OwnerPubkey:             "owner123",
		ImporterEnabled:         true,
		ImporterRealtimeEnabled: false, // Realtime disabled
		ImporterRelays:          []string{"wss://relay1.example.com"},
	}

	sub := NewSubscriber(cfg, func(ctx context.Context, event *nostr.Event) error {
		return nil
	})

	// Start should return immediately
	sub.Start()
	sub.Stop()
}

func TestSubscriber_CleanupSeenEvents(t *testing.T) {
	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: "owner123",
	}

	sub := NewSubscriber(cfg, nil)

	// Add some old and new events
	now := time.Now()
	sub.seenEvents["old1"] = now.Add(-2 * time.Hour) // 2 hours ago (should be cleaned)
	sub.seenEvents["old2"] = now.Add(-90 * time.Minute) // 90 mins ago (should be cleaned)
	sub.seenEvents["new1"] = now.Add(-30 * time.Minute) // 30 mins ago (should remain)
	sub.seenEvents["new2"] = now.Add(-5 * time.Minute)  // 5 mins ago (should remain)

	sub.cleanupSeenEvents()

	if len(sub.seenEvents) != 2 {
		t.Errorf("expected 2 events after cleanup, got %d", len(sub.seenEvents))
	}

	if _, ok := sub.seenEvents["new1"]; !ok {
		t.Error("new1 should not have been cleaned up")
	}
	if _, ok := sub.seenEvents["new2"]; !ok {
		t.Error("new2 should not have been cleaned up")
	}
}

func TestRelaySubscriptionStatus(t *testing.T) {
	status := RelaySubscriptionStatus{
		URL:           "wss://relay.example.com",
		Connected:     true,
		LastConnected: time.Now(),
		EventsRecv:    42,
		Errors:        3,
	}

	if status.URL != "wss://relay.example.com" {
		t.Error("URL not set correctly")
	}
	if !status.Connected {
		t.Error("Connected should be true")
	}
	if status.EventsRecv != 42 {
		t.Errorf("expected EventsRecv=42, got %d", status.EventsRecv)
	}
}

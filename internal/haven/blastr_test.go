package haven

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

// TestNewBlastr tests Blastr creation
func TestNewBlastr(t *testing.T) {
	cfg := &Config{
		Enabled:       true,
		OwnerPubkey:   ownerPubkey,
		BlastrEnabled: true,
		BlastrRelays:  []string{"wss://relay1.example.com", "wss://relay2.example.com"},
	}

	blastr := NewBlastr(cfg)
	if blastr == nil {
		t.Fatal("NewBlastr returned nil")
	}
	if blastr.config != cfg {
		t.Error("Blastr config not set correctly")
	}
	if blastr.relayPool == nil {
		t.Error("Blastr relay pool not initialized")
	}
	if blastr.eventQueue == nil {
		t.Error("Blastr event queue not initialized")
	}
}

// TestBlastr_Broadcast_OnlyOutboxEvents tests that only outbox events are broadcast
func TestBlastr_Broadcast_OnlyOutboxEvents(t *testing.T) {
	cfg := &Config{
		Enabled:       true,
		OwnerPubkey:   ownerPubkey,
		BlastrEnabled: true,
		BlastrRelays:  []string{"wss://relay.example.com"},
	}

	blastr := NewBlastr(cfg)

	tests := []struct {
		name          string
		event         *nostr.Event
		shouldQueue   bool
	}{
		{
			name: "owner event goes to outbox - should queue",
			event: &nostr.Event{
				ID:     "event1",
				PubKey: ownerPubkey,
				Kind:   1,
				Tags:   nostr.Tags{},
			},
			shouldQueue: true,
		},
		{
			name: "non-owner event - should not queue",
			event: &nostr.Event{
				ID:     "event2",
				PubKey: alicePubkey,
				Kind:   1,
				Tags:   nostr.Tags{},
			},
			shouldQueue: false,
		},
		{
			name: "chat event from owner - should not queue (goes to chat)",
			event: &nostr.Event{
				ID:     "event3",
				PubKey: ownerPubkey,
				Kind:   4, // DM kind goes to chat, not outbox
				Tags:   nostr.Tags{},
			},
			shouldQueue: false,
		},
		{
			name: "private event from owner - should not queue",
			event: &nostr.Event{
				ID:     "event4",
				PubKey: ownerPubkey,
				Kind:   30024, // Draft kind goes to private
				Tags:   nostr.Tags{},
			},
			shouldQueue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear queue
			select {
			case <-blastr.eventQueue:
			default:
			}

			blastr.Broadcast(tt.event)

			// Check if event was queued
			select {
			case <-blastr.eventQueue:
				if !tt.shouldQueue {
					t.Error("Event was queued but should not have been")
				}
			default:
				if tt.shouldQueue {
					t.Error("Event was not queued but should have been")
				}
			}
		})
	}
}

// TestBlastr_Disabled tests that disabled Blastr doesn't queue events
func TestBlastr_Disabled(t *testing.T) {
	cfg := &Config{
		Enabled:       true,
		OwnerPubkey:   ownerPubkey,
		BlastrEnabled: false, // Disabled
		BlastrRelays:  []string{"wss://relay.example.com"},
	}

	blastr := NewBlastr(cfg)

	event := &nostr.Event{
		ID:     "event1",
		PubKey: ownerPubkey,
		Kind:   1,
		Tags:   nostr.Tags{},
	}

	blastr.Broadcast(event)

	// Should not queue when disabled
	select {
	case <-blastr.eventQueue:
		t.Error("Event was queued but Blastr is disabled")
	default:
		// Expected
	}
}

// TestBlastr_Stats tests statistics tracking
func TestBlastr_Stats(t *testing.T) {
	cfg := &Config{
		Enabled:       true,
		OwnerPubkey:   ownerPubkey,
		BlastrEnabled: true,
		BlastrRelays:  []string{"wss://relay.example.com"},
	}

	blastr := NewBlastr(cfg)

	stats := blastr.Stats()
	if stats.EventsBroadcast != 0 {
		t.Errorf("Initial EventsBroadcast = %d, want 0", stats.EventsBroadcast)
	}
	if stats.EventsFailed != 0 {
		t.Errorf("Initial EventsFailed = %d, want 0", stats.EventsFailed)
	}
}

// TestBlastr_OnEventSaved tests the OnEventSaved handler
func TestBlastr_OnEventSaved(t *testing.T) {
	cfg := &Config{
		Enabled:       true,
		OwnerPubkey:   ownerPubkey,
		BlastrEnabled: true,
		BlastrRelays:  []string{"wss://relay.example.com"},
	}

	blastr := NewBlastr(cfg)
	handler := blastr.OnEventSaved()

	if handler == nil {
		t.Fatal("OnEventSaved returned nil handler")
	}

	// Call the handler with an outbox event
	event := &nostr.Event{
		ID:     "event1",
		PubKey: ownerPubkey,
		Kind:   1,
		Tags:   nostr.Tags{},
	}

	handler(context.Background(), event)

	// Check if event was queued
	select {
	case queued := <-blastr.eventQueue:
		if queued.ID != event.ID {
			t.Errorf("Queued event ID = %s, want %s", queued.ID, event.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Event was not queued by OnEventSaved handler")
	}
}

// TestRelayPool_ConnectedCount tests relay pool counting
func TestRelayPool_ConnectedCount(t *testing.T) {
	pool := NewRelayPool()

	count := pool.ConnectedCount()
	if count != 0 {
		t.Errorf("Initial ConnectedCount = %d, want 0", count)
	}
}

// TestRelayPool_Close tests relay pool cleanup
func TestRelayPool_Close(t *testing.T) {
	pool := NewRelayPool()

	// Close should not panic on empty pool
	pool.Close()

	count := pool.ConnectedCount()
	if count != 0 {
		t.Errorf("After Close ConnectedCount = %d, want 0", count)
	}
}

// TestBlastrStats_String would test string formatting if we add it
// For now, just test that stats struct works
func TestBlastrStats_Fields(t *testing.T) {
	stats := BlastrStats{
		EventsBroadcast: 10,
		EventsFailed:    2,
		RelaysConnected: 3,
		LastBroadcast:   time.Now(),
	}

	if stats.EventsBroadcast != 10 {
		t.Error("EventsBroadcast not set correctly")
	}
	if stats.EventsFailed != 2 {
		t.Error("EventsFailed not set correctly")
	}
	if stats.RelaysConnected != 3 {
		t.Error("RelaysConnected not set correctly")
	}
}

// TestRetryBackoff tests the exponential backoff calculation
func TestRetryBackoff(t *testing.T) {
	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 30 * time.Second},   // Attempt 0 treated as 1
		{1, 30 * time.Second},   // 30 * 2^0 = 30
		{2, 60 * time.Second},   // 30 * 2^1 = 60
		{3, 120 * time.Second},  // 30 * 2^2 = 120
		{4, 240 * time.Second},  // 30 * 2^3 = 240
		{5, 480 * time.Second},  // 30 * 2^4 = 480
		{6, 960 * time.Second},  // 30 * 2^5 = 960
		{7, 960 * time.Second},  // Capped at 960
		{10, 960 * time.Second}, // Capped at 960
	}

	for _, tt := range tests {
		result := retryBackoff(tt.attempt)
		if result != tt.expected {
			t.Errorf("retryBackoff(%d) = %v, want %v", tt.attempt, result, tt.expected)
		}
	}
}

// TestRetryEntry_JSON tests RetryEntry JSON marshaling
func TestRetryEntry_JSON(t *testing.T) {
	entry := RetryEntry{
		EventID:   "event123",
		Event:     []byte(`{"id":"event123","kind":1}`),
		RelayURL:  "wss://relay.example.com",
		Attempts:  3,
		AddedAt:   1234567890,
		LastError: "connection refused",
	}

	// Test that it can be marshaled without error
	data, err := json.Marshal(entry)
	if err != nil {
		t.Errorf("Failed to marshal RetryEntry: %v", err)
	}

	// Test that it can be unmarshaled
	var decoded RetryEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Errorf("Failed to unmarshal RetryEntry: %v", err)
	}

	if decoded.EventID != entry.EventID {
		t.Errorf("EventID = %s, want %s", decoded.EventID, entry.EventID)
	}
	if decoded.RelayURL != entry.RelayURL {
		t.Errorf("RelayURL = %s, want %s", decoded.RelayURL, entry.RelayURL)
	}
	if decoded.Attempts != entry.Attempts {
		t.Errorf("Attempts = %d, want %d", decoded.Attempts, entry.Attempts)
	}
}

// TestBlastr_SetRedisClient tests setting the Redis client
func TestBlastr_SetRedisClient(t *testing.T) {
	cfg := &Config{
		Enabled:       true,
		OwnerPubkey:   ownerPubkey,
		BlastrEnabled: true,
		BlastrRelays:  []string{"wss://relay.example.com"},
	}

	blastr := NewBlastr(cfg)

	// Initially nil
	if blastr.rdb != nil {
		t.Error("Redis client should be nil initially")
	}

	// SetRedisClient with nil shouldn't panic
	blastr.SetRedisClient(nil)
}

// TestBlastr_RetryQueueSize_NoRedis tests RetryQueueSize without Redis
func TestBlastr_RetryQueueSize_NoRedis(t *testing.T) {
	cfg := &Config{
		Enabled:       true,
		OwnerPubkey:   ownerPubkey,
		BlastrEnabled: true,
		BlastrRelays:  []string{"wss://relay.example.com"},
	}

	blastr := NewBlastr(cfg)

	// Without Redis, should return 0
	size := blastr.RetryQueueSize(context.Background())
	if size != 0 {
		t.Errorf("RetryQueueSize without Redis = %d, want 0", size)
	}
}

// TestBlastr_RetryConfig tests retry configuration
func TestBlastr_RetryConfig(t *testing.T) {
	cfg := &Config{
		Enabled:             true,
		OwnerPubkey:         ownerPubkey,
		BlastrEnabled:       true,
		BlastrRelays:        []string{"wss://relay.example.com"},
		BlastrRetryEnabled:  true,
		BlastrRetryKey:      "custom:retry:key",
		BlastrMaxRetries:    10,
		BlastrRetryInterval: 60,
	}

	blastr := NewBlastr(cfg)

	// Check that custom retry key is used
	if blastr.retryKey != "custom:retry:key" {
		t.Errorf("retryKey = %s, want custom:retry:key", blastr.retryKey)
	}
}

// TestBlastr_RetryConfig_Defaults tests default retry configuration
func TestBlastr_RetryConfig_Defaults(t *testing.T) {
	cfg := &Config{
		Enabled:       true,
		OwnerPubkey:   ownerPubkey,
		BlastrEnabled: true,
		BlastrRelays:  []string{"wss://relay.example.com"},
		// No retry config set
	}

	blastr := NewBlastr(cfg)

	// Check that default retry key is used
	if blastr.retryKey != "haven:blastr:retry" {
		t.Errorf("retryKey = %s, want haven:blastr:retry", blastr.retryKey)
	}
}

// TestBlastr_ClearRetryQueue_NoRedis tests ClearRetryQueue without Redis
func TestBlastr_ClearRetryQueue_NoRedis(t *testing.T) {
	cfg := &Config{
		Enabled:       true,
		OwnerPubkey:   ownerPubkey,
		BlastrEnabled: true,
		BlastrRelays:  []string{"wss://relay.example.com"},
	}

	blastr := NewBlastr(cfg)

	// Without Redis, should return nil (no error)
	err := blastr.ClearRetryQueue(context.Background())
	if err != nil {
		t.Errorf("ClearRetryQueue without Redis returned error: %v", err)
	}
}

// TestBlastrStats_RetryFields tests retry-related stats fields
func TestBlastrStats_RetryFields(t *testing.T) {
	stats := BlastrStats{
		EventsBroadcast:  10,
		EventsFailed:     2,
		RelaysConnected:  3,
		LastBroadcast:    time.Now(),
		RetryQueueSize:   5,
		EventsRetried:    3,
		RetriesExhausted: 1,
	}

	if stats.RetryQueueSize != 5 {
		t.Error("RetryQueueSize not set correctly")
	}
	if stats.EventsRetried != 3 {
		t.Error("EventsRetried not set correctly")
	}
	if stats.RetriesExhausted != 1 {
		t.Error("RetriesExhausted not set correctly")
	}
}

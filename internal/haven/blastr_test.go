package haven

import (
	"context"
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

package pubsub

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

func TestEventMessage_Marshal(t *testing.T) {
	event := &nostr.Event{
		ID:        "abc123def456",
		PubKey:    "pubkey123",
		Kind:      1,
		Content:   "test content",
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
	}

	em := eventMessage{
		PodID: "pod-1234",
		Event: event,
	}

	data, err := json.Marshal(em)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded eventMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.PodID != em.PodID {
		t.Errorf("PodID mismatch: got %q, want %q", decoded.PodID, em.PodID)
	}

	if decoded.Event.ID != em.Event.ID {
		t.Errorf("Event.ID mismatch: got %q, want %q", decoded.Event.ID, em.Event.ID)
	}

	if decoded.Event.Content != em.Event.Content {
		t.Errorf("Event.Content mismatch: got %q, want %q", decoded.Event.Content, em.Event.Content)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig returned nil")
	}

	if !cfg.Enabled {
		t.Error("Default config should have Enabled = true")
	}
}

func TestNew_NilRedis(t *testing.T) {
	// Creating PubSub with nil Redis client should work (graceful degradation)
	ps := New(nil, nil, nil)

	if ps == nil {
		t.Fatal("New returned nil")
	}

	// Publish should silently succeed with nil client
	err := ps.Publish(context.Background(), &nostr.Event{ID: "test"})
	if err != nil {
		t.Errorf("Publish with nil client should return nil, got %v", err)
	}
}

func TestNew_DisabledConfig(t *testing.T) {
	cfg := &Config{Enabled: false}
	ps := New(nil, nil, cfg)

	if ps == nil {
		t.Fatal("New returned nil")
	}

	if ps.config.Enabled {
		t.Error("Config should be disabled")
	}
}

func TestPubSub_PodID_Unique(t *testing.T) {
	ps1 := New(nil, nil, nil)
	// Small delay to ensure different nanosecond timestamps
	time.Sleep(time.Microsecond)
	ps2 := New(nil, nil, nil)

	if ps1.podID == ps2.podID {
		t.Error("Two PubSub instances should have different pod IDs")
	}
}

func TestCreateStoreEventHook(t *testing.T) {
	ps := New(nil, nil, nil)
	hook := ps.CreateStoreEventHook()

	if hook == nil {
		t.Fatal("CreateStoreEventHook returned nil")
	}

	// Hook should not error with nil Redis client
	event := &nostr.Event{
		ID:      "testid123456",
		PubKey:  "pubkey",
		Kind:    1,
		Content: "test",
	}

	err := hook(context.Background(), event)
	if err != nil {
		t.Errorf("Hook should return nil error, got %v", err)
	}
}

package haven

import (
	"context"
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

func TestTierPriority(t *testing.T) {
	tests := []struct {
		tier     string
		expected int
	}{
		{"enterprise", 100},
		{"premium", 75},
		{"hybrid", 50},
		{"free", 25},
		{"unknown", 10},
		{"", 10},
	}

	for _, tt := range tests {
		t.Run(tt.tier, func(t *testing.T) {
			got := tierPriority(tt.tier)
			if got != tt.expected {
				t.Errorf("tierPriority(%q) = %d, want %d", tt.tier, got, tt.expected)
			}
		})
	}
}

func TestBlastrJob_Fields(t *testing.T) {
	event := &nostr.Event{
		ID:     "testevent123",
		PubKey: alicePubkey,
		Kind:   1,
	}

	job := BlastrJob{
		Event:      event,
		UserPubkey: alicePubkey,
		Relays:     []string{"wss://relay1.com", "wss://relay2.com"},
		Tier:       "premium",
		Priority:   75,
	}

	if job.Event.ID != "testevent123" {
		t.Error("Event ID mismatch")
	}
	if job.UserPubkey != alicePubkey {
		t.Error("UserPubkey mismatch")
	}
	if len(job.Relays) != 2 {
		t.Errorf("Expected 2 relays, got %d", len(job.Relays))
	}
	if job.Tier != "premium" {
		t.Error("Tier mismatch")
	}
	if job.Priority != 75 {
		t.Error("Priority mismatch")
	}
}

func TestBlastrManagerConfig_Defaults(t *testing.T) {
	cfg := DefaultBlastrManagerConfig()

	if cfg.WorkerCount != 10 {
		t.Errorf("WorkerCount = %d, want 10", cfg.WorkerCount)
	}
	if cfg.QueueSize != 1000 {
		t.Errorf("QueueSize = %d, want 1000", cfg.QueueSize)
	}
	if cfg.MaxRetries != 6 {
		t.Errorf("MaxRetries = %d, want 6", cfg.MaxRetries)
	}
	if cfg.RetryEnabled {
		t.Error("RetryEnabled should default to false")
	}
}

func TestNewBlastrManager(t *testing.T) {
	cfg := DefaultBlastrManagerConfig()
	manager := NewBlastrManager(cfg, nil, nil)

	if manager == nil {
		t.Fatal("NewBlastrManager returned nil")
	}
	if manager.workerCount != 10 {
		t.Errorf("workerCount = %d, want 10", manager.workerCount)
	}
	if cap(manager.jobQueue) != 1000 {
		t.Errorf("jobQueue capacity = %d, want 1000", cap(manager.jobQueue))
	}
}

func TestNewBlastrManager_NilConfig(t *testing.T) {
	manager := NewBlastrManager(nil, nil, nil)

	if manager == nil {
		t.Fatal("NewBlastrManager(nil) returned nil")
	}
	if manager.workerCount != 10 {
		t.Error("Should use default worker count")
	}
}

func TestBlastrManager_QueueSize(t *testing.T) {
	cfg := &BlastrManagerConfig{
		WorkerCount: 1,
		QueueSize:   10,
	}
	manager := NewBlastrManager(cfg, nil, nil)

	if manager.QueueSize() != 0 {
		t.Error("Initial queue size should be 0")
	}
}

func TestBlastrManager_Stats(t *testing.T) {
	manager := NewBlastrManager(nil, nil, nil)
	stats := manager.Stats()

	if stats.JobsQueued != 0 {
		t.Error("Initial JobsQueued should be 0")
	}
	if stats.JobsProcessed != 0 {
		t.Error("Initial JobsProcessed should be 0")
	}
}

func TestBlastrManagerStats_Fields(t *testing.T) {
	stats := BlastrManagerStats{
		JobsQueued:       100,
		JobsProcessed:    90,
		JobsFailed:       5,
		JobsDropped:      5,
		UserCount:        10,
		RelaysConnected:  3,
		RetryQueueSize:   2,
		EventsRetried:    8,
		RetriesExhausted: 1,
	}

	if stats.JobsQueued != 100 {
		t.Error("JobsQueued mismatch")
	}
	if stats.JobsProcessed != 90 {
		t.Error("JobsProcessed mismatch")
	}
	if stats.JobsFailed+stats.JobsDropped != 10 {
		t.Error("Failed + Dropped should equal 10")
	}
}

func TestUserRetryEntry_Fields(t *testing.T) {
	entry := UserRetryEntry{
		EventID:    "event123",
		UserPubkey: alicePubkey,
		RelayURL:   "wss://relay.test",
		Tier:       "premium",
		Attempts:   2,
		AddedAt:    1234567890,
		LastError:  "connection refused",
	}

	if entry.EventID != "event123" {
		t.Error("EventID mismatch")
	}
	if entry.UserPubkey != alicePubkey {
		t.Error("UserPubkey mismatch")
	}
	if entry.Attempts != 2 {
		t.Error("Attempts mismatch")
	}
}

// mockMemberStoreForBlastr implements MemberStore for testing
type mockMemberStoreForBlastr struct {
	members map[string]*MemberInfo
}

func (m *mockMemberStoreForBlastr) GetMemberInfo(ctx context.Context, pubkey string) (*MemberInfo, error) {
	if info, ok := m.members[pubkey]; ok {
		return info, nil
	}
	return nil, nil
}

func (m *mockMemberStoreForBlastr) IsMember(ctx context.Context, pubkey string) (bool, error) {
	_, ok := m.members[pubkey]
	return ok, nil
}

func TestBlastrManager_OnEventSaved_NoStores(t *testing.T) {
	manager := NewBlastrManager(nil, nil, nil)
	handler := manager.OnEventSaved()

	// Should not panic with no stores
	event := &nostr.Event{
		ID:     "test123",
		PubKey: alicePubkey,
		Kind:   1,
	}

	// This should do nothing without panicking
	handler(context.Background(), event)

	if manager.QueueSize() != 0 {
		t.Error("Queue should remain empty without stores")
	}
}

func TestBlastrManager_BroadcastForUser(t *testing.T) {
	cfg := &BlastrManagerConfig{
		WorkerCount: 1,
		QueueSize:   10,
	}
	manager := NewBlastrManager(cfg, nil, nil)

	event := &nostr.Event{
		ID:     "test123",
		PubKey: alicePubkey,
		Kind:   1,
	}

	err := manager.BroadcastForUser(context.Background(), event, alicePubkey, []string{"wss://relay.test"})
	if err != nil {
		t.Errorf("BroadcastForUser returned error: %v", err)
	}

	// Job should be queued
	if manager.QueueSize() != 1 {
		t.Errorf("QueueSize = %d, want 1", manager.QueueSize())
	}
}

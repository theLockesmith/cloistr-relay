package haven

import (
	"context"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

func TestImporterJob_Fields(t *testing.T) {
	since := time.Now().Add(-1 * time.Hour)
	job := ImporterJob{
		UserPubkey: alicePubkey,
		Relays:     []string{"wss://relay1.com", "wss://relay2.com"},
		Since:      since,
		Tier:       "premium",
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
	if !job.Since.Equal(since) {
		t.Error("Since mismatch")
	}
}

func TestImporterManagerConfig_Defaults(t *testing.T) {
	cfg := DefaultImporterManagerConfig()

	if cfg.WorkerCount != 5 {
		t.Errorf("WorkerCount = %d, want 5", cfg.WorkerCount)
	}
	if cfg.QueueSize != 500 {
		t.Errorf("QueueSize = %d, want 500", cfg.QueueSize)
	}
	if cfg.PollInterval != 5*time.Minute {
		t.Errorf("PollInterval = %v, want 5m", cfg.PollInterval)
	}
	if cfg.LookbackDefault != 24*time.Hour {
		t.Errorf("LookbackDefault = %v, want 24h", cfg.LookbackDefault)
	}
	if cfg.MaxEventsPerJob != 100 {
		t.Errorf("MaxEventsPerJob = %d, want 100", cfg.MaxEventsPerJob)
	}
}

func TestNewImporterManager(t *testing.T) {
	cfg := DefaultImporterManagerConfig()
	manager := NewImporterManager(cfg, nil, nil)

	if manager == nil {
		t.Fatal("NewImporterManager returned nil")
	}
	if manager.workerCount != 5 {
		t.Errorf("workerCount = %d, want 5", manager.workerCount)
	}
	if cap(manager.jobQueue) != 500 {
		t.Errorf("jobQueue capacity = %d, want 500", cap(manager.jobQueue))
	}
	if manager.pollInterval != 5*time.Minute {
		t.Errorf("pollInterval = %v, want 5m", manager.pollInterval)
	}
}

func TestNewImporterManager_NilConfig(t *testing.T) {
	manager := NewImporterManager(nil, nil, nil)

	if manager == nil {
		t.Fatal("NewImporterManager(nil) returned nil")
	}
	if manager.workerCount != 5 {
		t.Error("Should use default worker count")
	}
}

func TestImporterManager_QueueSize(t *testing.T) {
	cfg := &ImporterManagerConfig{
		WorkerCount: 1,
		QueueSize:   10,
	}
	manager := NewImporterManager(cfg, nil, nil)

	if manager.QueueSize() != 0 {
		t.Error("Initial queue size should be 0")
	}
}

func TestImporterManager_Stats(t *testing.T) {
	manager := NewImporterManager(nil, nil, nil)
	stats := manager.Stats()

	if stats.JobsQueued != 0 {
		t.Error("Initial JobsQueued should be 0")
	}
	if stats.JobsProcessed != 0 {
		t.Error("Initial JobsProcessed should be 0")
	}
	if stats.EventsImported != 0 {
		t.Error("Initial EventsImported should be 0")
	}
}

func TestImporterManagerStats_Fields(t *testing.T) {
	stats := ImporterManagerStats{
		JobsQueued:        100,
		JobsProcessed:     95,
		EventsImported:    500,
		EventsSkipped:     200,
		FetchErrors:       5,
		UsersWithImporter: 20,
		RelaysPolled:      10,
	}

	if stats.JobsQueued != 100 {
		t.Error("JobsQueued mismatch")
	}
	if stats.JobsProcessed != 95 {
		t.Error("JobsProcessed mismatch")
	}
	if stats.EventsImported != 500 {
		t.Error("EventsImported mismatch")
	}
}

func TestImporterManager_SetStoreFunc(t *testing.T) {
	manager := NewImporterManager(nil, nil, nil)

	storeFunc := func(ctx context.Context, event *nostr.Event, userPubkey string) error {
		return nil
	}

	manager.SetStoreFunc(storeFunc)

	if manager.storeFunc == nil {
		t.Error("storeFunc should be set")
	}
}

func TestImporterManager_ImportForUser(t *testing.T) {
	cfg := &ImporterManagerConfig{
		WorkerCount: 1,
		QueueSize:   10,
	}
	manager := NewImporterManager(cfg, nil, nil)

	since := time.Now().Add(-1 * time.Hour)
	manager.ImportForUser(context.Background(), alicePubkey, []string{"wss://relay.test"}, since)

	// Job should be queued
	if manager.QueueSize() != 1 {
		t.Errorf("QueueSize = %d, want 1", manager.QueueSize())
	}
}

func TestImporterManager_CleanupSeenEvents(t *testing.T) {
	manager := NewImporterManager(nil, nil, nil)

	// Add some seen events
	manager.mu.Lock()
	for i := 0; i < 100; i++ {
		manager.seenEvents["event"+string(rune(i))] = true
	}
	manager.mu.Unlock()

	// Should not clean up (under threshold)
	manager.CleanupSeenEvents()

	manager.mu.RLock()
	count := len(manager.seenEvents)
	manager.mu.RUnlock()

	if count != 100 {
		t.Errorf("Should not cleanup with %d events", count)
	}

	// Add more to exceed threshold
	manager.mu.Lock()
	for i := 0; i < 10001; i++ {
		manager.seenEvents["event"+string(rune(i))] = true
	}
	manager.mu.Unlock()

	// Should clean up (over threshold)
	manager.CleanupSeenEvents()

	manager.mu.RLock()
	count = len(manager.seenEvents)
	manager.mu.RUnlock()

	if count != 0 {
		t.Errorf("Should cleanup when over threshold, got %d", count)
	}
}

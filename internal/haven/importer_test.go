package haven

import (
	"context"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

// TestNewImporter tests Importer creation
func TestNewImporter(t *testing.T) {
	cfg := &Config{
		Enabled:         true,
		OwnerPubkey:     ownerPubkey,
		ImporterEnabled: true,
		ImporterRelays:  []string{"wss://relay1.example.com", "wss://relay2.example.com"},
	}

	storeFunc := func(ctx context.Context, event *nostr.Event) error {
		return nil
	}

	importer := NewImporter(cfg, storeFunc)
	if importer == nil {
		t.Fatal("NewImporter returned nil")
	}
	if importer.config != cfg {
		t.Error("Importer config not set correctly")
	}
	if importer.relayPool == nil {
		t.Error("Importer relay pool not initialized")
	}
	if importer.storeFunc == nil {
		t.Error("Importer store function not set")
	}
	if importer.lastFetch == nil {
		t.Error("Importer lastFetch map not initialized")
	}
	if importer.seenEvents == nil {
		t.Error("Importer seenEvents map not initialized")
	}
}

// TestNewImporter_NilStoreFunc tests Importer with nil store function
func TestNewImporter_NilStoreFunc(t *testing.T) {
	cfg := &Config{
		Enabled:         true,
		OwnerPubkey:     ownerPubkey,
		ImporterEnabled: true,
		ImporterRelays:  []string{"wss://relay.example.com"},
	}

	importer := NewImporter(cfg, nil)
	if importer == nil {
		t.Fatal("NewImporter returned nil")
	}
	if importer.storeFunc != nil {
		t.Error("Importer store function should be nil")
	}
}

// TestImporter_Stats tests statistics tracking
func TestImporter_Stats(t *testing.T) {
	cfg := &Config{
		Enabled:         true,
		OwnerPubkey:     ownerPubkey,
		ImporterEnabled: true,
		ImporterRelays:  []string{"wss://relay.example.com"},
	}

	importer := NewImporter(cfg, nil)

	stats := importer.Stats()
	if stats.EventsImported != 0 {
		t.Errorf("Initial EventsImported = %d, want 0", stats.EventsImported)
	}
	if stats.EventsSkipped != 0 {
		t.Errorf("Initial EventsSkipped = %d, want 0", stats.EventsSkipped)
	}
	if stats.FetchErrors != 0 {
		t.Errorf("Initial FetchErrors = %d, want 0", stats.FetchErrors)
	}
}

// TestImporter_CleanupSeenEvents tests seen events cleanup
func TestImporter_CleanupSeenEvents(t *testing.T) {
	cfg := &Config{
		Enabled:         true,
		OwnerPubkey:     ownerPubkey,
		ImporterEnabled: true,
		ImporterRelays:  []string{"wss://relay.example.com"},
	}

	importer := NewImporter(cfg, nil)

	// Add more than 1000 events
	for i := 0; i < 1100; i++ {
		importer.seenEvents[string(rune(i))] = true
	}

	if len(importer.seenEvents) != 1100 {
		t.Errorf("seenEvents count = %d, want 1100", len(importer.seenEvents))
	}

	// Cleanup should clear when over 1000
	importer.cleanupSeenEvents()

	if len(importer.seenEvents) != 0 {
		t.Errorf("After cleanup seenEvents count = %d, want 0", len(importer.seenEvents))
	}
}

// TestImporter_CleanupSeenEvents_UnderLimit tests cleanup doesn't clear under limit
func TestImporter_CleanupSeenEvents_UnderLimit(t *testing.T) {
	cfg := &Config{
		Enabled:         true,
		OwnerPubkey:     ownerPubkey,
		ImporterEnabled: true,
		ImporterRelays:  []string{"wss://relay.example.com"},
	}

	importer := NewImporter(cfg, nil)

	// Add fewer than 1000 events
	for i := 0; i < 500; i++ {
		importer.seenEvents[string(rune(i))] = true
	}

	// Cleanup should not clear when under 1000
	importer.cleanupSeenEvents()

	if len(importer.seenEvents) != 500 {
		t.Errorf("After cleanup seenEvents count = %d, want 500", len(importer.seenEvents))
	}
}

// TestDefaultImporterConfig tests default configuration values
func TestDefaultImporterConfig(t *testing.T) {
	cfg := DefaultImporterConfig()

	if cfg.PollInterval != 5*time.Minute {
		t.Errorf("PollInterval = %v, want 5m", cfg.PollInterval)
	}
	if cfg.LookbackDuration != 24*time.Hour {
		t.Errorf("LookbackDuration = %v, want 24h", cfg.LookbackDuration)
	}
	if cfg.MaxEventsPerPoll != 100 {
		t.Errorf("MaxEventsPerPoll = %d, want 100", cfg.MaxEventsPerPoll)
	}
	if cfg.EventKinds != nil {
		t.Error("EventKinds should be nil by default")
	}
}

// TestImporterStats_Fields tests statistics fields
func TestImporterStats_Fields(t *testing.T) {
	stats := ImporterStats{
		EventsImported: 100,
		EventsSkipped:  20,
		FetchErrors:    5,
		RelaysPolled:   3,
		LastImport:     time.Now(),
		LastPollTime:   time.Now(),
	}

	if stats.EventsImported != 100 {
		t.Error("EventsImported not set correctly")
	}
	if stats.EventsSkipped != 20 {
		t.Error("EventsSkipped not set correctly")
	}
	if stats.FetchErrors != 5 {
		t.Error("FetchErrors not set correctly")
	}
	if stats.RelaysPolled != 3 {
		t.Error("RelaysPolled not set correctly")
	}
}

// TestImporter_StartStop_Disabled tests start/stop with disabled importer
func TestImporter_StartStop_Disabled(t *testing.T) {
	cfg := &Config{
		Enabled:         true,
		OwnerPubkey:     ownerPubkey,
		ImporterEnabled: false, // Disabled
		ImporterRelays:  []string{"wss://relay.example.com"},
	}

	importer := NewImporter(cfg, nil)

	// Should not panic when disabled
	importer.Start()
	importer.Stop()
}

// TestImporter_StartStop_NoRelays tests start/stop with no relays
func TestImporter_StartStop_NoRelays(t *testing.T) {
	cfg := &Config{
		Enabled:         true,
		OwnerPubkey:     ownerPubkey,
		ImporterEnabled: true,
		ImporterRelays:  []string{}, // No relays
	}

	importer := NewImporter(cfg, nil)

	// Should not panic with no relays
	importer.Start()
	importer.Stop()
}

// TestImporter_StartStop_NoStoreFunc tests start/stop with no store function
func TestImporter_StartStop_NoStoreFunc(t *testing.T) {
	cfg := &Config{
		Enabled:         true,
		OwnerPubkey:     ownerPubkey,
		ImporterEnabled: true,
		ImporterRelays:  []string{"wss://relay.example.com"},
	}

	importer := NewImporter(cfg, nil) // nil store function

	// Should not panic with no store function
	importer.Start()
	importer.Stop()
}

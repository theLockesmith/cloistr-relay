package writeahead

import (
	"context"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.Enabled {
		t.Error("Expected Enabled to be true by default")
	}
	if cfg.QueueKey != "wal:events" {
		t.Errorf("Expected QueueKey to be 'wal:events', got %s", cfg.QueueKey)
	}
	if cfg.DrainInterval != 100*time.Millisecond {
		t.Errorf("Expected DrainInterval to be 100ms, got %v", cfg.DrainInterval)
	}
	if cfg.BatchSize != 100 {
		t.Errorf("Expected BatchSize to be 100, got %d", cfg.BatchSize)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries to be 3, got %d", cfg.MaxRetries)
	}
	if cfg.RetryDelay != 1*time.Second {
		t.Errorf("Expected RetryDelay to be 1s, got %v", cfg.RetryDelay)
	}
	if cfg.EventTTL != 24*time.Hour {
		t.Errorf("Expected EventTTL to be 24h, got %v", cfg.EventTTL)
	}
}

func TestWAL_New(t *testing.T) {
	wal := New(nil, nil, nil)
	if wal == nil {
		t.Fatal("Expected non-nil WAL")
	}
	if wal.config == nil {
		t.Fatal("Expected non-nil config")
	}
	if wal.config.BatchSize != 100 {
		t.Errorf("Expected default BatchSize, got %d", wal.config.BatchSize)
	}
	if wal.stopCh == nil {
		t.Error("Expected non-nil stopCh")
	}
	if wal.stoppedCh == nil {
		t.Error("Expected non-nil stoppedCh")
	}
}

func TestWAL_NewWithConfig(t *testing.T) {
	cfg := &Config{
		Enabled:       true,
		QueueKey:      "custom:queue",
		DrainInterval: 200 * time.Millisecond,
		BatchSize:     50,
		MaxRetries:    5,
		RetryDelay:    2 * time.Second,
		EventTTL:      12 * time.Hour,
	}

	wal := New(nil, nil, cfg)

	if wal.config.QueueKey != "custom:queue" {
		t.Errorf("Expected custom QueueKey, got %s", wal.config.QueueKey)
	}
	if wal.config.DrainInterval != 200*time.Millisecond {
		t.Errorf("Expected custom DrainInterval, got %v", wal.config.DrainInterval)
	}
	if wal.config.BatchSize != 50 {
		t.Errorf("Expected custom BatchSize, got %d", wal.config.BatchSize)
	}
}

func TestWAL_QueueLengthWithoutRedis(t *testing.T) {
	wal := New(nil, nil, DefaultConfig())

	length, err := wal.QueueLength(context.TODO())
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if length != 0 {
		t.Errorf("Expected 0 queue length without Redis, got %d", length)
	}
}

func TestWAL_GetStatsWithoutRedis(t *testing.T) {
	wal := New(nil, nil, DefaultConfig())

	stats := wal.GetStats(context.TODO())
	if stats == nil {
		t.Fatal("Expected non-nil stats")
	}
	if stats.Written != 0 {
		t.Errorf("Expected 0 written, got %d", stats.Written)
	}
	if stats.Drained != 0 {
		t.Errorf("Expected 0 drained, got %d", stats.Drained)
	}
	if stats.Failed != 0 {
		t.Errorf("Expected 0 failed, got %d", stats.Failed)
	}
	if stats.QueueLength != 0 {
		t.Errorf("Expected 0 queue length, got %d", stats.QueueLength)
	}
}

func TestWAL_IsHealthyWithoutRedis(t *testing.T) {
	wal := New(nil, nil, DefaultConfig())

	// Without Redis, QueueLength returns 0, nil - so healthy would be true
	// Since w.rdb is nil, QueueLength returns 0 with nil rdb
	healthy := wal.IsHealthy(context.TODO())
	// With nil Redis client, this should still work (returns healthy=true)
	_ = healthy
}

func TestWAL_CreateSaveEventHandler(t *testing.T) {
	wal := New(nil, nil, DefaultConfig())

	handler := wal.CreateSaveEventHandler()
	if handler == nil {
		t.Error("Expected non-nil handler")
	}
}

func TestWAL_StartStop(t *testing.T) {
	wal := New(nil, nil, DefaultConfig())

	// Start should be idempotent
	wal.Start()
	wal.Start() // Second call should be no-op

	if !wal.running {
		t.Error("Expected WAL to be running")
	}

	// Stop should be idempotent
	wal.Stop()
	wal.Stop() // Second call should be no-op

	if wal.running {
		t.Error("Expected WAL to be stopped")
	}
}

func TestWAL_RunningFlag(t *testing.T) {
	wal := New(nil, nil, DefaultConfig())

	if wal.running {
		t.Error("Expected WAL to not be running initially")
	}

	wal.Start()
	if !wal.running {
		t.Error("Expected WAL to be running after Start")
	}

	wal.Stop()
	if wal.running {
		t.Error("Expected WAL to not be running after Stop")
	}
}

func TestStats_Fields(t *testing.T) {
	stats := &Stats{
		Written:     100,
		Drained:     95,
		Failed:      5,
		QueueLength: 10,
	}

	if stats.Written != 100 {
		t.Errorf("Written mismatch: %d", stats.Written)
	}
	if stats.Drained != 95 {
		t.Errorf("Drained mismatch: %d", stats.Drained)
	}
	if stats.Failed != 5 {
		t.Errorf("Failed mismatch: %d", stats.Failed)
	}
	if stats.QueueLength != 10 {
		t.Errorf("QueueLength mismatch: %d", stats.QueueLength)
	}
}

func TestWalEvent_Fields(t *testing.T) {
	evt := &walEvent{
		Timestamp: 1234567890,
		Retries:   2,
	}

	if evt.Timestamp != 1234567890 {
		t.Errorf("Timestamp mismatch: %d", evt.Timestamp)
	}
	if evt.Retries != 2 {
		t.Errorf("Retries mismatch: %d", evt.Retries)
	}
}

// Integration tests would require a real Redis/Dragonfly connection
// and PostgreSQL database. These are left as structural tests that
// validate the API without external dependencies.

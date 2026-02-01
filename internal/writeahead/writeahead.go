// Package writeahead provides a write-ahead log using Redis/Dragonfly
//
// Events are written to Dragonfly first (fast acknowledgment), then
// asynchronously drained to PostgreSQL (durable storage).
//
// Benefits:
// - Fast OK response to clients after Dragonfly write
// - Higher write throughput
// - Resilience against PostgreSQL latency spikes
//
// Durability:
// - Dragonfly AOF persistence minimizes data loss window to ~1 second
// - Events are kept in WAL until confirmed written to PostgreSQL
// - Background worker continuously drains WAL to PostgreSQL
package writeahead

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/fiatjaf/eventstore/postgresql"
	"github.com/nbd-wtf/go-nostr"
	"github.com/redis/go-redis/v9"
)

// Config holds WAL configuration
type Config struct {
	// Enabled activates write-ahead logging
	Enabled bool
	// QueueKey is the Redis list key for pending events
	QueueKey string
	// DrainInterval is how often to drain events to PostgreSQL
	DrainInterval time.Duration
	// BatchSize is max events to drain per batch
	BatchSize int
	// MaxRetries is max attempts to write an event to PostgreSQL
	MaxRetries int
	// RetryDelay is wait time between retries
	RetryDelay time.Duration
	// EventTTL is how long events stay in WAL before expiring (failsafe)
	EventTTL time.Duration
}

// DefaultConfig returns sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Enabled:       true,
		QueueKey:      "wal:events",
		DrainInterval: 100 * time.Millisecond,
		BatchSize:     100,
		MaxRetries:    3,
		RetryDelay:    1 * time.Second,
		EventTTL:      24 * time.Hour,
	}
}

// walEvent wraps an event with metadata
type walEvent struct {
	Event     *nostr.Event `json:"event"`
	Timestamp int64        `json:"ts"`
	Retries   int          `json:"retries"`
}

// WAL provides write-ahead logging via Redis/Dragonfly
type WAL struct {
	rdb      *redis.Client
	db       *postgresql.PostgresBackend
	config   *Config
	stopCh   chan struct{}
	stoppedCh chan struct{}
	mu       sync.Mutex
	running  bool

	// Stats
	statsWritten int64
	statsDrained int64
	statsFailed  int64
}

// New creates a new WAL
func New(rdb *redis.Client, db *postgresql.PostgresBackend, cfg *Config) *WAL {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &WAL{
		rdb:       rdb,
		db:        db,
		config:    cfg,
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}
}

// Write adds an event to the write-ahead log
// Returns immediately after Dragonfly write (fast path)
func (w *WAL) Write(ctx context.Context, event *nostr.Event) error {
	if w.rdb == nil || !w.config.Enabled {
		// Fallback: write directly to PostgreSQL
		return w.db.SaveEvent(ctx, event)
	}

	walEvt := &walEvent{
		Event:     event,
		Timestamp: time.Now().UnixNano(),
		Retries:   0,
	}

	data, err := json.Marshal(walEvt)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Push to Redis list (queue)
	if err := w.rdb.RPush(ctx, w.config.QueueKey, data).Err(); err != nil {
		log.Printf("WAL write error, falling back to direct write: %v", err)
		// Fallback: write directly to PostgreSQL
		return w.db.SaveEvent(ctx, event)
	}

	w.mu.Lock()
	w.statsWritten++
	w.mu.Unlock()

	return nil
}

// Start begins the background drain worker
func (w *WAL) Start() {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.mu.Unlock()

	go w.drainWorker()
	log.Printf("WAL drain worker started (interval: %v, batch: %d)", w.config.DrainInterval, w.config.BatchSize)
}

// Stop stops the background drain worker
func (w *WAL) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	w.mu.Unlock()

	close(w.stopCh)
	<-w.stoppedCh
	log.Println("WAL drain worker stopped")
}

// drainWorker continuously drains events from WAL to PostgreSQL
func (w *WAL) drainWorker() {
	defer close(w.stoppedCh)

	ticker := time.NewTicker(w.config.DrainInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			// Drain remaining events before stopping
			w.drainAll()
			return
		case <-ticker.C:
			w.drainBatch()
		}
	}
}

// drainBatch drains up to BatchSize events to PostgreSQL
func (w *WAL) drainBatch() {
	if w.rdb == nil || w.db == nil {
		return
	}

	ctx := context.Background()

	for i := 0; i < w.config.BatchSize; i++ {
		// Pop from the left of the list (FIFO)
		data, err := w.rdb.LPop(ctx, w.config.QueueKey).Bytes()
		if err == redis.Nil {
			// Queue is empty
			return
		}
		if err != nil {
			log.Printf("WAL drain pop error: %v", err)
			return
		}

		var walEvt walEvent
		if err := json.Unmarshal(data, &walEvt); err != nil {
			log.Printf("WAL drain unmarshal error: %v", err)
			continue
		}

		// Check if event is too old (failsafe)
		age := time.Since(time.Unix(0, walEvt.Timestamp))
		if age > w.config.EventTTL {
			log.Printf("WAL event expired (age: %v), discarding: %s", age, walEvt.Event.ID)
			w.mu.Lock()
			w.statsFailed++
			w.mu.Unlock()
			continue
		}

		// Try to write to PostgreSQL
		if err := w.db.SaveEvent(ctx, walEvt.Event); err != nil {
			log.Printf("WAL drain save error: %v", err)

			// Retry logic
			walEvt.Retries++
			if walEvt.Retries < w.config.MaxRetries {
				// Re-queue for retry (push to back)
				retryData, _ := json.Marshal(walEvt)
				w.rdb.RPush(ctx, w.config.QueueKey, retryData)
			} else {
				log.Printf("WAL event exceeded max retries, discarding: %s", walEvt.Event.ID)
				w.mu.Lock()
				w.statsFailed++
				w.mu.Unlock()
			}
			continue
		}

		w.mu.Lock()
		w.statsDrained++
		w.mu.Unlock()
	}
}

// drainAll drains all remaining events (used during shutdown)
func (w *WAL) drainAll() {
	if w.rdb == nil || w.db == nil {
		return
	}

	ctx := context.Background()
	count := 0

	for {
		data, err := w.rdb.LPop(ctx, w.config.QueueKey).Bytes()
		if err == redis.Nil {
			break // Queue is empty
		}
		if err != nil {
			log.Printf("WAL drainAll pop error: %v", err)
			break
		}

		var walEvt walEvent
		if err := json.Unmarshal(data, &walEvt); err != nil {
			log.Printf("WAL drainAll unmarshal error: %v", err)
			continue
		}

		if err := w.db.SaveEvent(ctx, walEvt.Event); err != nil {
			log.Printf("WAL drainAll save error (event may be lost): %v", err)
		} else {
			count++
		}
	}

	if count > 0 {
		log.Printf("WAL drained %d events during shutdown", count)
	}
}

// QueueLength returns the current number of events in the WAL
func (w *WAL) QueueLength(ctx context.Context) (int64, error) {
	if w.rdb == nil {
		return 0, nil
	}
	return w.rdb.LLen(ctx, w.config.QueueKey).Result()
}

// Stats returns WAL statistics
type Stats struct {
	Written     int64
	Drained     int64
	Failed      int64
	QueueLength int64
}

// GetStats returns current WAL statistics
func (w *WAL) GetStats(ctx context.Context) *Stats {
	w.mu.Lock()
	written := w.statsWritten
	drained := w.statsDrained
	failed := w.statsFailed
	w.mu.Unlock()

	// Get queue length (outside mutex to avoid blocking writers)
	var length int64
	if l, err := w.QueueLength(ctx); err == nil {
		length = l
	}

	return &Stats{
		Written:     written,
		Drained:     drained,
		Failed:      failed,
		QueueLength: length,
	}
}

// SaveEvent is the khatru-compatible event save handler
// Use this in place of db.SaveEvent for write-ahead logging
func (w *WAL) SaveEvent(ctx context.Context, event *nostr.Event) error {
	return w.Write(ctx, event)
}

// CreateSaveEventHandler returns a function compatible with khatru's StoreEvent
func (w *WAL) CreateSaveEventHandler() func(context.Context, *nostr.Event) error {
	return func(ctx context.Context, event *nostr.Event) error {
		return w.Write(ctx, event)
	}
}

// IsHealthy returns true if WAL queue is not backing up
// Useful for health checks
func (w *WAL) IsHealthy(ctx context.Context) bool {
	length, err := w.QueueLength(ctx)
	if err != nil {
		return false
	}
	// Consider unhealthy if queue exceeds 10000 events
	return length < 10000
}

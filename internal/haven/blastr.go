package haven

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/redis/go-redis/v9"
)

// Blastr broadcasts outbox events to other relays
// Named after the HAVEN project's "blastr" concept
type Blastr struct {
	config       *Config
	router       *Router
	relayPool    *RelayPool
	eventQueue   chan *nostr.Event
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	mu           sync.RWMutex
	stats        BlastrStats
	metrics      *Metrics

	// Retry queue (optional, requires Redis/Dragonfly)
	rdb          *redis.Client
	retryKey     string
}

// BlastrStats tracks broadcast statistics
type BlastrStats struct {
	EventsBroadcast int64
	EventsFailed    int64
	RelaysConnected int
	LastBroadcast   time.Time
	RetryQueueSize  int64
	EventsRetried   int64
	RetriesExhausted int64
}

// RetryEntry represents a failed broadcast attempt queued for retry
type RetryEntry struct {
	EventID   string          `json:"event_id"`
	Event     json.RawMessage `json:"event"`
	RelayURL  string          `json:"relay"`
	Attempts  int             `json:"attempts"`
	AddedAt   int64           `json:"added_at"`
	LastError string          `json:"last_error,omitempty"`
}

// retryBackoff returns the delay for the given attempt number (exponential backoff)
// Attempt 1: 30s, 2: 60s, 3: 120s, 4: 240s, 5: 480s, 6: 960s
func retryBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}
	seconds := 30 * (1 << (attempt - 1)) // 30 * 2^(attempt-1)
	if seconds > 960 {
		seconds = 960
	}
	return time.Duration(seconds) * time.Second
}

// RelayPool manages connections to multiple relays
type RelayPool struct {
	relays map[string]*nostr.Relay
	mu     sync.RWMutex
}

// NewRelayPool creates a new relay connection pool
func NewRelayPool() *RelayPool {
	return &RelayPool{
		relays: make(map[string]*nostr.Relay),
	}
}

// Connect establishes connection to a relay
func (p *RelayPool) Connect(ctx context.Context, url string) (*nostr.Relay, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if already connected
	if relay, ok := p.relays[url]; ok {
		if relay.IsConnected() {
			return relay, nil
		}
		// Remove stale connection
		delete(p.relays, url)
	}

	// Connect to relay
	relay, err := nostr.RelayConnect(ctx, url)
	if err != nil {
		return nil, err
	}

	p.relays[url] = relay
	return relay, nil
}

// Publish sends an event to a relay
func (p *RelayPool) Publish(ctx context.Context, url string, event *nostr.Event) error {
	relay, err := p.Connect(ctx, url)
	if err != nil {
		return err
	}

	return relay.Publish(ctx, *event)
}

// Close closes all relay connections
func (p *RelayPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for url, relay := range p.relays {
		relay.Close()
		delete(p.relays, url)
	}
}

// ConnectedCount returns the number of connected relays
func (p *RelayPool) ConnectedCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	count := 0
	for _, relay := range p.relays {
		if relay.IsConnected() {
			count++
		}
	}
	return count
}

// NewBlastr creates a new outbox broadcaster
func NewBlastr(cfg *Config) *Blastr {
	ctx, cancel := context.WithCancel(context.Background())

	retryKey := cfg.BlastrRetryKey
	if retryKey == "" {
		retryKey = "haven:blastr:retry"
	}

	return &Blastr{
		config:     cfg,
		router:     NewRouter(cfg),
		relayPool:  NewRelayPool(),
		eventQueue: make(chan *nostr.Event, 100),
		ctx:        ctx,
		cancel:     cancel,
		metrics:    GetMetrics(),
		retryKey:   retryKey,
	}
}

// SetRedisClient sets the Redis client for persistent retry queue
// Call this before Start() to enable retry functionality
func (b *Blastr) SetRedisClient(rdb *redis.Client) {
	b.rdb = rdb
}

// Start begins the broadcast worker
func (b *Blastr) Start() {
	if !b.config.BlastrEnabled || len(b.config.BlastrRelays) == 0 {
		log.Println("HAVEN Blastr: disabled (no relays configured)")
		return
	}

	b.wg.Add(1)
	go b.worker()

	// Start retry worker if Redis is available and retry is enabled
	if b.rdb != nil && b.config.BlastrRetryEnabled {
		b.wg.Add(1)
		go b.retryWorker()
		log.Printf("HAVEN Blastr: retry queue enabled (key: %s, interval: %ds)",
			b.retryKey, b.config.BlastrRetryInterval)
	}

	log.Printf("HAVEN Blastr: started, broadcasting to %d relays", len(b.config.BlastrRelays))
}

// Stop gracefully shuts down the broadcaster
func (b *Blastr) Stop() {
	b.cancel()
	close(b.eventQueue)
	b.wg.Wait()
	b.relayPool.Close()
	log.Println("HAVEN Blastr: stopped")
}

// Broadcast queues an event for broadcasting to other relays
func (b *Blastr) Broadcast(event *nostr.Event) {
	if !b.config.BlastrEnabled {
		return
	}

	// Only broadcast outbox events from owner
	box := b.router.RouteEvent(event)
	if box != BoxOutbox {
		return
	}

	// Queue the event
	select {
	case b.eventQueue <- event:
		// Event queued
		b.metrics.RecordBlastrQueued()
		b.metrics.SetBlastrQueueSize(len(b.eventQueue))
	default:
		// Queue full, log and drop
		log.Printf("HAVEN Blastr: queue full, dropping event %s", truncateID(event.ID))
		b.mu.Lock()
		b.stats.EventsFailed++
		b.mu.Unlock()
		b.metrics.RecordBlastrDropped()
	}
}

// worker processes the broadcast queue
func (b *Blastr) worker() {
	defer b.wg.Done()

	for {
		select {
		case <-b.ctx.Done():
			return
		case event, ok := <-b.eventQueue:
			if !ok {
				return
			}
			b.broadcastEvent(event)
		}
	}
}

// broadcastEvent sends an event to all configured relays
func (b *Blastr) broadcastEvent(event *nostr.Event) {
	var wg sync.WaitGroup
	var successCount int32
	var failedRelays []string
	var mu sync.Mutex

	ctx, cancel := context.WithTimeout(b.ctx, 10*time.Second)
	defer cancel()

	// Update queue size metric after processing
	defer b.metrics.SetBlastrQueueSize(len(b.eventQueue))

	for _, relayURL := range b.config.BlastrRelays {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()

			err := b.relayPool.Publish(ctx, url, event)
			if err != nil {
				log.Printf("HAVEN Blastr: failed to publish to %s: %v", url, err)
				b.metrics.RecordBlastrRelayPublish(url, false)
				mu.Lock()
				failedRelays = append(failedRelays, url)
				mu.Unlock()
				return
			}

			b.metrics.RecordBlastrRelayPublish(url, true)
			mu.Lock()
			successCount++
			mu.Unlock()
		}(relayURL)
	}

	wg.Wait()

	// Queue failed relays for retry if retry is enabled
	if len(failedRelays) > 0 && b.rdb != nil && b.config.BlastrRetryEnabled {
		for _, relayURL := range failedRelays {
			b.queueForRetry(event, relayURL, "initial broadcast failed")
		}
	}

	b.mu.Lock()
	if successCount > 0 {
		b.stats.EventsBroadcast++
		b.stats.LastBroadcast = time.Now()
		b.metrics.RecordBlastrBroadcast()
	} else if len(failedRelays) == 0 {
		// No relays at all
		b.stats.EventsFailed++
		b.metrics.RecordBlastrFailed()
	}
	// Note: If some succeeded and some are queued for retry, we count it as broadcast
	b.stats.RelaysConnected = b.relayPool.ConnectedCount()
	b.metrics.SetBlastrRelaysConnected(b.stats.RelaysConnected)
	b.mu.Unlock()

	log.Printf("HAVEN Blastr: broadcast event %s to %d/%d relays (retry queued: %d)",
		truncateID(event.ID), successCount, len(b.config.BlastrRelays), len(failedRelays))
}

// Stats returns current broadcast statistics
func (b *Blastr) Stats() BlastrStats {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.stats
}

// OnEventSaved returns a handler to be registered with relay.OnEventSaved
func (b *Blastr) OnEventSaved() func(context.Context, *nostr.Event) {
	return func(ctx context.Context, event *nostr.Event) {
		b.Broadcast(event)
	}
}

// queueForRetry adds a failed broadcast to the retry queue
func (b *Blastr) queueForRetry(event *nostr.Event, relayURL string, lastError string) {
	if b.rdb == nil {
		return
	}

	// Serialize the event
	eventJSON, err := json.Marshal(event)
	if err != nil {
		log.Printf("HAVEN Blastr retry: failed to marshal event: %v", err)
		return
	}

	entry := RetryEntry{
		EventID:   event.ID,
		Event:     eventJSON,
		RelayURL:  relayURL,
		Attempts:  1,
		AddedAt:   time.Now().Unix(),
		LastError: lastError,
	}

	entryJSON, err := json.Marshal(entry)
	if err != nil {
		log.Printf("HAVEN Blastr retry: failed to marshal retry entry: %v", err)
		return
	}

	// Calculate next retry time using exponential backoff
	nextRetry := time.Now().Add(retryBackoff(entry.Attempts))
	score := float64(nextRetry.Unix())

	ctx := context.Background()
	err = b.rdb.ZAdd(ctx, b.retryKey, redis.Z{
		Score:  score,
		Member: string(entryJSON),
	}).Err()

	if err != nil {
		log.Printf("HAVEN Blastr retry: failed to queue entry: %v", err)
		return
	}

	b.metrics.RecordBlastrRetryQueued()
	log.Printf("HAVEN Blastr: queued retry for event %s to %s (attempt %d, next: %v)",
		truncateID(event.ID), relayURL, entry.Attempts, nextRetry.Format(time.RFC3339))
}

// retryWorker processes the retry queue periodically
func (b *Blastr) retryWorker() {
	defer b.wg.Done()

	interval := time.Duration(b.config.BlastrRetryInterval) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			b.processRetryQueue()
		}
	}
}

// processRetryQueue processes all due retry entries
func (b *Blastr) processRetryQueue() {
	if b.rdb == nil {
		return
	}

	ctx := context.Background()
	now := float64(time.Now().Unix())

	// Get all entries with score <= now (due for retry)
	entries, err := b.rdb.ZRangeByScoreWithScores(ctx, b.retryKey, &redis.ZRangeBy{
		Min:   "-inf",
		Max:   formatFloat(now),
		Count: 50, // Process up to 50 at a time
	}).Result()

	if err != nil {
		log.Printf("HAVEN Blastr retry: failed to fetch queue: %v", err)
		return
	}

	if len(entries) == 0 {
		return
	}

	maxRetries := b.config.BlastrMaxRetries
	if maxRetries <= 0 {
		maxRetries = 6
	}

	for _, z := range entries {
		entryStr, ok := z.Member.(string)
		if !ok {
			continue
		}

		var entry RetryEntry
		if err := json.Unmarshal([]byte(entryStr), &entry); err != nil {
			log.Printf("HAVEN Blastr retry: failed to unmarshal entry: %v", err)
			b.removeFromRetryQueue(ctx, entryStr)
			continue
		}

		// Remove from queue first (we'll re-add if retry fails)
		b.removeFromRetryQueue(ctx, entryStr)

		// Deserialize the event
		var event nostr.Event
		if err := json.Unmarshal(entry.Event, &event); err != nil {
			log.Printf("HAVEN Blastr retry: failed to unmarshal event: %v", err)
			continue
		}

		// Attempt to publish
		pubCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := b.relayPool.Publish(pubCtx, entry.RelayURL, &event)
		cancel()

		if err != nil {
			entry.Attempts++
			entry.LastError = err.Error()

			if entry.Attempts >= maxRetries {
				// Max retries exhausted
				log.Printf("HAVEN Blastr retry: max retries (%d) exhausted for event %s to %s: %v",
					maxRetries, truncateID(entry.EventID), entry.RelayURL, err)
				b.mu.Lock()
				b.stats.RetriesExhausted++
				b.mu.Unlock()
				b.metrics.RecordBlastrRetryExhausted()
			} else {
				// Re-queue with increased attempt count
				b.requeue(entry)
			}
		} else {
			// Success!
			log.Printf("HAVEN Blastr retry: successfully published event %s to %s (attempt %d)",
				truncateID(entry.EventID), entry.RelayURL, entry.Attempts)
			b.mu.Lock()
			b.stats.EventsRetried++
			b.mu.Unlock()
			b.metrics.RecordBlastrRetrySuccess()
			b.metrics.RecordBlastrRelayPublish(entry.RelayURL, true)
		}
	}

	// Update retry queue size metric
	b.updateRetryQueueSize(ctx)
}

// requeue adds an entry back to the retry queue with updated attempt count
func (b *Blastr) requeue(entry RetryEntry) {
	if b.rdb == nil {
		return
	}

	entryJSON, err := json.Marshal(entry)
	if err != nil {
		log.Printf("HAVEN Blastr retry: failed to marshal entry for requeue: %v", err)
		return
	}

	// Calculate next retry time
	nextRetry := time.Now().Add(retryBackoff(entry.Attempts))
	score := float64(nextRetry.Unix())

	ctx := context.Background()
	err = b.rdb.ZAdd(ctx, b.retryKey, redis.Z{
		Score:  score,
		Member: string(entryJSON),
	}).Err()

	if err != nil {
		log.Printf("HAVEN Blastr retry: failed to requeue entry: %v", err)
		return
	}

	log.Printf("HAVEN Blastr: requeued event %s to %s (attempt %d, next: %v)",
		truncateID(entry.EventID), entry.RelayURL, entry.Attempts, nextRetry.Format(time.RFC3339))
}

// removeFromRetryQueue removes an entry from the retry queue
func (b *Blastr) removeFromRetryQueue(ctx context.Context, member string) {
	if b.rdb == nil {
		return
	}
	b.rdb.ZRem(ctx, b.retryKey, member)
}

// updateRetryQueueSize updates the retry queue size metric
func (b *Blastr) updateRetryQueueSize(ctx context.Context) {
	if b.rdb == nil {
		return
	}

	size, err := b.rdb.ZCard(ctx, b.retryKey).Result()
	if err != nil {
		return
	}

	b.mu.Lock()
	b.stats.RetryQueueSize = size
	b.mu.Unlock()
	b.metrics.SetBlastrRetryQueueSize(int(size))
}

// RetryQueueSize returns the current size of the retry queue
func (b *Blastr) RetryQueueSize(ctx context.Context) int64 {
	if b.rdb == nil {
		return 0
	}
	size, _ := b.rdb.ZCard(ctx, b.retryKey).Result()
	return size
}

// formatFloat formats a float64 for Redis score
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// ClearRetryQueue removes all entries from the retry queue (for testing/maintenance)
func (b *Blastr) ClearRetryQueue(ctx context.Context) error {
	if b.rdb == nil {
		return nil
	}
	return b.rdb.Del(ctx, b.retryKey).Err()
}

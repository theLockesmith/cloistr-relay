package haven

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/redis/go-redis/v9"
)

// BlastrManager manages per-user outbox broadcasting with a shared worker pool.
// Instead of one worker per user, a fixed pool of workers processes jobs for all users.
type BlastrManager struct {
	config        *Config
	memberStore   MemberStore
	userSettings  *UserSettingsStore
	relayPool     *RelayPool
	jobQueue      chan BlastrJob
	workerCount   int
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	mu            sync.RWMutex
	stats         BlastrManagerStats
	metrics       *Metrics

	// Retry queue (optional, requires Redis/Dragonfly)
	rdb        *redis.Client
	retryKey   string
	maxRetries int
}

// BlastrJob represents a job to broadcast an event for a specific user
type BlastrJob struct {
	Event      *nostr.Event
	UserPubkey string   // Whose outbox this is
	Relays     []string // That user's configured relays
	Tier       string   // User's tier for metrics
	Priority   int      // Tier-based priority (higher = more urgent)
}

// BlastrManagerStats tracks broadcast statistics across all users
type BlastrManagerStats struct {
	JobsQueued       int64
	JobsProcessed    int64
	JobsFailed       int64
	JobsDropped      int64
	UserCount        int
	RelaysConnected  int
	LastBroadcast    time.Time
	RetryQueueSize   int64
	EventsRetried    int64
	RetriesExhausted int64
}

// BlastrManagerConfig holds configuration for the BlastrManager
type BlastrManagerConfig struct {
	WorkerCount     int    // Number of worker goroutines (default: 10)
	QueueSize       int    // Job queue size (default: 1000)
	RetryEnabled    bool   // Enable retry queue
	RetryKey        string // Redis key prefix for retry queue
	MaxRetries      int    // Max retry attempts (default: 6)
	RetryIntervalSec int   // Retry worker interval in seconds (default: 30)
}

// DefaultBlastrManagerConfig returns sensible defaults
func DefaultBlastrManagerConfig() *BlastrManagerConfig {
	return &BlastrManagerConfig{
		WorkerCount:      10,
		QueueSize:        1000,
		RetryEnabled:     false,
		RetryKey:         "haven:blastr:user:",
		MaxRetries:       6,
		RetryIntervalSec: 30,
	}
}

// NewBlastrManager creates a new per-user blastr manager
func NewBlastrManager(cfg *BlastrManagerConfig, memberStore MemberStore, userSettings *UserSettingsStore) *BlastrManager {
	if cfg == nil {
		cfg = DefaultBlastrManagerConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &BlastrManager{
		config:       &Config{}, // Will be set via SetConfig
		memberStore:  memberStore,
		userSettings: userSettings,
		relayPool:    NewRelayPool(),
		jobQueue:     make(chan BlastrJob, cfg.QueueSize),
		workerCount:  cfg.WorkerCount,
		ctx:          ctx,
		cancel:       cancel,
		metrics:      GetMetrics(),
		retryKey:     cfg.RetryKey,
		maxRetries:   cfg.MaxRetries,
	}
}

// SetConfig sets the haven config (for fallback behavior)
func (m *BlastrManager) SetConfig(cfg *Config) {
	m.config = cfg
}

// SetRedisClient sets the Redis client for persistent retry queue
func (m *BlastrManager) SetRedisClient(rdb *redis.Client) {
	m.rdb = rdb
}

// Start begins the worker pool
func (m *BlastrManager) Start() {
	if m.memberStore == nil && m.userSettings == nil {
		log.Println("HAVEN BlastrManager: disabled (no stores configured)")
		return
	}

	// Start fixed worker pool
	for i := 0; i < m.workerCount; i++ {
		m.wg.Add(1)
		go m.worker(i)
	}

	log.Printf("HAVEN BlastrManager: started with %d workers", m.workerCount)
}

// Stop gracefully shuts down the manager
func (m *BlastrManager) Stop() {
	m.cancel()
	close(m.jobQueue)
	m.wg.Wait()
	m.relayPool.Close()
	log.Println("HAVEN BlastrManager: stopped")
}

// OnEventSaved returns a handler for relay.OnEventSaved
// This checks if the event author is a member with Blastr enabled
func (m *BlastrManager) OnEventSaved() func(context.Context, *nostr.Event) {
	return func(ctx context.Context, event *nostr.Event) {
		m.handleEvent(ctx, event)
	}
}

// handleEvent processes an event to determine if it should be broadcast
func (m *BlastrManager) handleEvent(ctx context.Context, event *nostr.Event) {
	// Check if the author is a member with HAVEN access
	if m.memberStore == nil {
		return
	}

	memberInfo, err := m.memberStore.GetMemberInfo(ctx, event.PubKey)
	if err != nil || memberInfo == nil {
		return
	}

	// Check tier allows Blastr
	if !memberInfo.HasBlastr {
		return
	}

	// Get user's haven settings
	if m.userSettings == nil {
		return
	}

	settings, err := m.userSettings.GetSettings(ctx, event.PubKey)
	if err != nil || settings == nil {
		return
	}

	// Check if blastr is enabled for this user
	if !settings.BlastrEnabled || len(settings.BlastrRelays) == 0 {
		return
	}

	// Apply tier-based relay limits
	relays := settings.BlastrRelays
	if memberInfo.MaxBlastrRelays > 0 && len(relays) > memberInfo.MaxBlastrRelays {
		relays = relays[:memberInfo.MaxBlastrRelays]
	}

	// Create and queue the job
	job := BlastrJob{
		Event:      event,
		UserPubkey: event.PubKey,
		Relays:     relays,
		Tier:       memberInfo.Tier,
		Priority:   tierPriority(memberInfo.Tier),
	}

	m.queueJob(job)
}

// queueJob adds a job to the queue
func (m *BlastrManager) queueJob(job BlastrJob) {
	select {
	case m.jobQueue <- job:
		m.mu.Lock()
		m.stats.JobsQueued++
		m.mu.Unlock()
		m.metrics.RecordBlastrManagerQueued(job.Tier)
	default:
		// Queue full
		m.mu.Lock()
		m.stats.JobsDropped++
		m.mu.Unlock()
		m.metrics.RecordBlastrManagerDropped(job.Tier)
		log.Printf("HAVEN BlastrManager: queue full, dropping event %s for user %s",
			truncateID(job.Event.ID), truncateID(job.UserPubkey))
	}
}

// worker processes jobs from the queue
func (m *BlastrManager) worker(id int) {
	defer m.wg.Done()

	for {
		select {
		case <-m.ctx.Done():
			return
		case job, ok := <-m.jobQueue:
			if !ok {
				return
			}
			m.processJob(job)
		}
	}
}

// processJob broadcasts an event to the user's configured relays
func (m *BlastrManager) processJob(job BlastrJob) {
	var wg sync.WaitGroup
	var successCount int32
	var failedRelays []string
	var mu sync.Mutex

	ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
	defer cancel()

	for _, relayURL := range job.Relays {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()

			err := m.relayPool.Publish(ctx, url, job.Event)
			if err != nil {
				log.Printf("HAVEN BlastrManager: failed to publish to %s for %s: %v",
					url, truncateID(job.UserPubkey), err)
				m.metrics.RecordBlastrManagerRelayPublish(url, job.Tier, false)
				mu.Lock()
				failedRelays = append(failedRelays, url)
				mu.Unlock()
				return
			}

			m.metrics.RecordBlastrManagerRelayPublish(url, job.Tier, true)
			mu.Lock()
			successCount++
			mu.Unlock()
		}(relayURL)
	}

	wg.Wait()

	m.mu.Lock()
	if successCount > 0 {
		m.stats.JobsProcessed++
		m.stats.LastBroadcast = time.Now()
		m.metrics.RecordBlastrManagerBroadcast(job.Tier)
	} else if len(failedRelays) == len(job.Relays) {
		m.stats.JobsFailed++
		m.metrics.RecordBlastrManagerFailed(job.Tier)
	}
	m.stats.RelaysConnected = m.relayPool.ConnectedCount()
	m.mu.Unlock()

	// Queue failed relays for retry
	if len(failedRelays) > 0 && m.rdb != nil {
		for _, relayURL := range failedRelays {
			m.queueForRetry(job.Event, job.UserPubkey, relayURL, job.Tier, "broadcast failed")
		}
	}

	if successCount > 0 || len(failedRelays) > 0 {
		log.Printf("HAVEN BlastrManager: broadcast event %s for %s to %d/%d relays",
			truncateID(job.Event.ID), truncateID(job.UserPubkey), successCount, len(job.Relays))
	}
}

// queueForRetry adds a failed broadcast to the per-user retry queue
func (m *BlastrManager) queueForRetry(event *nostr.Event, userPubkey, relayURL, tier, lastError string) {
	if m.rdb == nil {
		return
	}

	eventJSON, err := json.Marshal(event)
	if err != nil {
		log.Printf("HAVEN BlastrManager retry: failed to marshal event: %v", err)
		return
	}

	entry := UserRetryEntry{
		EventID:    event.ID,
		Event:      eventJSON,
		UserPubkey: userPubkey,
		RelayURL:   relayURL,
		Tier:       tier,
		Attempts:   1,
		AddedAt:    time.Now().Unix(),
		LastError:  lastError,
	}

	entryJSON, err := json.Marshal(entry)
	if err != nil {
		return
	}

	// Per-user retry key
	key := m.retryKey + userPubkey
	nextRetry := time.Now().Add(retryBackoff(entry.Attempts))
	score := float64(nextRetry.Unix())

	ctx := context.Background()
	err = m.rdb.ZAdd(ctx, key, redis.Z{
		Score:  score,
		Member: string(entryJSON),
	}).Err()

	if err != nil {
		log.Printf("HAVEN BlastrManager retry: failed to queue: %v", err)
	}
}

// UserRetryEntry represents a failed broadcast for retry
type UserRetryEntry struct {
	EventID    string          `json:"event_id"`
	Event      json.RawMessage `json:"event"`
	UserPubkey string          `json:"user_pubkey"`
	RelayURL   string          `json:"relay"`
	Tier       string          `json:"tier"`
	Attempts   int             `json:"attempts"`
	AddedAt    int64           `json:"added_at"`
	LastError  string          `json:"last_error,omitempty"`
}

// Stats returns current statistics
func (m *BlastrManager) Stats() BlastrManagerStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stats
}

// QueueSize returns current queue depth
func (m *BlastrManager) QueueSize() int {
	return len(m.jobQueue)
}

// tierPriority returns a priority value for the tier (higher = more urgent)
func tierPriority(tier string) int {
	switch tier {
	case "enterprise":
		return 100
	case "premium":
		return 75
	case "hybrid":
		return 50
	case "free":
		return 25
	default:
		return 10
	}
}

// BroadcastForUser manually broadcasts an event for a user (for testing/admin)
func (m *BlastrManager) BroadcastForUser(ctx context.Context, event *nostr.Event, userPubkey string, relays []string) error {
	job := BlastrJob{
		Event:      event,
		UserPubkey: userPubkey,
		Relays:     relays,
		Tier:       "manual",
		Priority:   100,
	}
	m.queueJob(job)
	return nil
}

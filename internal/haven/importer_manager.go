package haven

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

// ImporterManager manages per-user inbox importing with a shared worker pool.
// A scheduler periodically enqueues import jobs for all users with Importer enabled.
type ImporterManager struct {
	memberStore   MemberStore
	userSettings  *UserSettingsStore
	relayPool     *RelayPool
	storeFunc     func(context.Context, *nostr.Event, string) error // store(ctx, event, userPubkey)
	jobQueue      chan ImporterJob
	workerCount   int
	pollInterval  time.Duration
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	mu            sync.RWMutex
	stats         ImporterManagerStats
	seenEvents    map[string]bool // Global dedup (event ID -> seen)
	metrics       *Metrics
}

// ImporterJob represents a job to import events for a specific user
type ImporterJob struct {
	UserPubkey string
	Relays     []string
	Since      time.Time
	Tier       string
}

// ImporterManagerStats tracks import statistics across all users
type ImporterManagerStats struct {
	JobsQueued      int64
	JobsProcessed   int64
	EventsImported  int64
	EventsSkipped   int64
	FetchErrors     int64
	UsersWithImporter int
	RelaysPolled    int
	LastScheduleRun time.Time
}

// ImporterManagerConfig holds configuration for the ImporterManager
type ImporterManagerConfig struct {
	WorkerCount     int           // Number of worker goroutines (default: 5)
	QueueSize       int           // Job queue size (default: 500)
	PollInterval    time.Duration // How often to schedule import jobs (default: 5 min)
	LookbackDefault time.Duration // Default lookback for new users (default: 24h)
	MaxEventsPerJob int           // Max events per user per job (default: 100)
}

// DefaultImporterManagerConfig returns sensible defaults
func DefaultImporterManagerConfig() *ImporterManagerConfig {
	return &ImporterManagerConfig{
		WorkerCount:     5,
		QueueSize:       500,
		PollInterval:    5 * time.Minute,
		LookbackDefault: 24 * time.Hour,
		MaxEventsPerJob: 100,
	}
}

// NewImporterManager creates a new per-user importer manager
func NewImporterManager(cfg *ImporterManagerConfig, memberStore MemberStore, userSettings *UserSettingsStore) *ImporterManager {
	if cfg == nil {
		cfg = DefaultImporterManagerConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &ImporterManager{
		memberStore:  memberStore,
		userSettings: userSettings,
		relayPool:    NewRelayPool(),
		jobQueue:     make(chan ImporterJob, cfg.QueueSize),
		workerCount:  cfg.WorkerCount,
		pollInterval: cfg.PollInterval,
		ctx:          ctx,
		cancel:       cancel,
		seenEvents:   make(map[string]bool),
		metrics:      GetMetrics(),
	}
}

// SetStoreFunc sets the function used to store imported events
// The function receives the event and the target user's pubkey
func (m *ImporterManager) SetStoreFunc(fn func(context.Context, *nostr.Event, string) error) {
	m.storeFunc = fn
}

// Start begins the worker pool and scheduler
func (m *ImporterManager) Start() {
	if m.memberStore == nil || m.userSettings == nil {
		log.Println("HAVEN ImporterManager: disabled (no stores configured)")
		return
	}

	if m.storeFunc == nil {
		log.Println("HAVEN ImporterManager: disabled (no store function)")
		return
	}

	// Start worker pool
	for i := 0; i < m.workerCount; i++ {
		m.wg.Add(1)
		go m.worker(i)
	}

	// Start scheduler
	m.wg.Add(1)
	go m.scheduler()

	log.Printf("HAVEN ImporterManager: started with %d workers, polling every %v",
		m.workerCount, m.pollInterval)
}

// Stop gracefully shuts down the manager
func (m *ImporterManager) Stop() {
	m.cancel()
	close(m.jobQueue)
	m.wg.Wait()
	m.relayPool.Close()
	log.Println("HAVEN ImporterManager: stopped")
}

// scheduler periodically enqueues import jobs for all users
func (m *ImporterManager) scheduler() {
	defer m.wg.Done()

	// Initial run
	m.scheduleAllUsers()

	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.scheduleAllUsers()
		}
	}
}

// scheduleAllUsers creates import jobs for all users with importer enabled
func (m *ImporterManager) scheduleAllUsers() {
	ctx := context.Background()

	m.mu.Lock()
	m.stats.LastScheduleRun = time.Now()
	m.mu.Unlock()

	// Get all users with importer enabled
	users, err := m.userSettings.GetUsersForImport(ctx)
	if err != nil {
		log.Printf("HAVEN ImporterManager: failed to get users: %v", err)
		return
	}

	m.mu.Lock()
	m.stats.UsersWithImporter = len(users)
	m.mu.Unlock()

	m.metrics.SetImporterManagerUsersEnabled(len(users))

	for _, settings := range users {
		// Check if user is a member with importer access
		memberInfo, err := m.memberStore.GetMemberInfo(ctx, settings.Pubkey)
		if err != nil || memberInfo == nil || !memberInfo.HasImporter {
			continue
		}

		// Apply tier-based relay limits
		relays := settings.ImporterRelays
		if memberInfo.MaxImporterRelays > 0 && len(relays) > memberInfo.MaxImporterRelays {
			relays = relays[:memberInfo.MaxImporterRelays]
		}

		if len(relays) == 0 {
			continue
		}

		// Determine since time
		since := time.Now().Add(-24 * time.Hour) // Default lookback
		if settings.LastImportTime != nil {
			since = *settings.LastImportTime
		}

		job := ImporterJob{
			UserPubkey: settings.Pubkey,
			Relays:     relays,
			Since:      since,
			Tier:       memberInfo.Tier,
		}

		m.queueJob(job)
	}
}

// queueJob adds a job to the queue
func (m *ImporterManager) queueJob(job ImporterJob) {
	select {
	case m.jobQueue <- job:
		m.mu.Lock()
		m.stats.JobsQueued++
		m.mu.Unlock()
		m.metrics.RecordImporterManagerQueued(job.Tier)
	default:
		// Queue full, skip this job
		log.Printf("HAVEN ImporterManager: queue full, skipping import for %s",
			truncateID(job.UserPubkey))
	}
}

// worker processes jobs from the queue
func (m *ImporterManager) worker(id int) {
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

// processJob imports events for a user from their configured relays
func (m *ImporterManager) processJob(job ImporterJob) {
	var imported, skipped, errors int

	ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
	defer cancel()

	for _, relayURL := range job.Relays {
		i, s, e := m.importFromRelay(ctx, job, relayURL)
		imported += i
		skipped += s
		errors += e
	}

	m.mu.Lock()
	m.stats.JobsProcessed++
	m.stats.EventsImported += int64(imported)
	m.stats.EventsSkipped += int64(skipped)
	m.stats.FetchErrors += int64(errors)
	m.mu.Unlock()

	// Update last import time for user
	if imported > 0 && m.userSettings != nil {
		m.userSettings.UpdateLastImportTime(context.Background(), job.UserPubkey, time.Now())
	}

	if imported > 0 {
		log.Printf("HAVEN ImporterManager: imported %d events for %s (skipped %d, errors %d)",
			imported, truncateID(job.UserPubkey), skipped, errors)
	}

	m.metrics.RecordImporterManagerProcessed(job.Tier)
}

// importFromRelay fetches events for a user from a single relay
func (m *ImporterManager) importFromRelay(ctx context.Context, job ImporterJob, relayURL string) (imported, skipped, errors int) {
	// Connect to relay
	relay, err := m.relayPool.Connect(ctx, relayURL)
	if err != nil {
		log.Printf("HAVEN ImporterManager: failed to connect to %s: %v", relayURL, err)
		m.metrics.RecordImporterManagerRelayFetch(relayURL, job.Tier, false)
		return 0, 0, 1
	}

	sinceTimestamp := nostr.Timestamp(job.Since.Unix())

	// Build filter for events addressed to this user
	filter := nostr.Filter{
		Tags: nostr.TagMap{
			"p": []string{job.UserPubkey},
		},
		Since: &sinceTimestamp,
		Limit: 100,
	}

	// Query events
	events, err := relay.QuerySync(ctx, filter)
	if err != nil {
		log.Printf("HAVEN ImporterManager: query failed for %s: %v", relayURL, err)
		m.metrics.RecordImporterManagerRelayFetch(relayURL, job.Tier, false)
		return 0, 0, 1
	}

	m.metrics.RecordImporterManagerRelayFetch(relayURL, job.Tier, true)

	// Process events
	for _, event := range events {
		// Skip if we've seen this event globally (prevents duplication across users)
		m.mu.RLock()
		seen := m.seenEvents[event.ID]
		m.mu.RUnlock()

		if seen {
			skipped++
			continue
		}

		// Skip events from the user themselves (those are outbox, not inbox)
		if event.PubKey == job.UserPubkey {
			skipped++
			continue
		}

		// Store the event for this user
		if m.storeFunc != nil {
			if err := m.storeFunc(ctx, event, job.UserPubkey); err != nil {
				log.Printf("HAVEN ImporterManager: failed to store event %s: %v",
					truncateID(event.ID), err)
				errors++
				continue
			}
		}

		// Mark as seen
		m.mu.Lock()
		m.seenEvents[event.ID] = true
		m.mu.Unlock()

		imported++
		m.metrics.RecordImporterManagerImported(job.Tier)
	}

	return imported, skipped, errors
}

// Stats returns current statistics
func (m *ImporterManager) Stats() ImporterManagerStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stats
}

// QueueSize returns current queue depth
func (m *ImporterManager) QueueSize() int {
	return len(m.jobQueue)
}

// ForceSchedule triggers an immediate scheduling run
func (m *ImporterManager) ForceSchedule() {
	go m.scheduleAllUsers()
}

// CleanupSeenEvents removes old entries from the seen events map
func (m *ImporterManager) CleanupSeenEvents() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.seenEvents) > 10000 {
		// Clear the map - in production use LRU or TTL
		m.seenEvents = make(map[string]bool)
	}
}

// ImportForUser manually triggers an import for a specific user (for testing/admin)
func (m *ImporterManager) ImportForUser(ctx context.Context, userPubkey string, relays []string, since time.Time) {
	job := ImporterJob{
		UserPubkey: userPubkey,
		Relays:     relays,
		Since:      since,
		Tier:       "manual",
	}
	m.queueJob(job)
}

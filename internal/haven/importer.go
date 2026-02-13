package haven

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

// Importer fetches events addressed to the owner from other relays
type Importer struct {
	config      *Config
	router      *Router
	relayPool   *RelayPool
	storeFunc   func(context.Context, *nostr.Event) error
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	mu          sync.RWMutex
	stats       ImporterStats
	lastFetch   map[string]time.Time // Last successful fetch per relay
	seenEvents  map[string]bool      // Dedup recent events
}

// ImporterStats tracks import statistics
type ImporterStats struct {
	EventsImported  int64
	EventsSkipped   int64 // Duplicates or already have
	FetchErrors     int64
	RelaysPolled    int
	LastImport      time.Time
	LastPollTime    time.Time
}

// ImporterConfig holds importer-specific settings
type ImporterConfig struct {
	// PollInterval is how often to poll relays for new events
	PollInterval time.Duration
	// LookbackDuration is how far back to look for events on first poll
	LookbackDuration time.Duration
	// MaxEventsPerPoll limits events fetched per relay per poll
	MaxEventsPerPoll int
	// EventKinds are the kinds to fetch (empty = all inbox kinds)
	EventKinds []int
}

// DefaultImporterConfig returns sensible defaults
func DefaultImporterConfig() *ImporterConfig {
	return &ImporterConfig{
		PollInterval:     5 * time.Minute,
		LookbackDuration: 24 * time.Hour,
		MaxEventsPerPoll: 100,
		EventKinds:       nil, // Use default inbox kinds
	}
}

// NewImporter creates a new inbox event importer
func NewImporter(cfg *Config, storeFunc func(context.Context, *nostr.Event) error) *Importer {
	ctx, cancel := context.WithCancel(context.Background())

	return &Importer{
		config:     cfg,
		router:     NewRouter(cfg),
		relayPool:  NewRelayPool(),
		storeFunc:  storeFunc,
		ctx:        ctx,
		cancel:     cancel,
		lastFetch:  make(map[string]time.Time),
		seenEvents: make(map[string]bool),
	}
}

// Start begins the import polling loop
func (i *Importer) Start() {
	if !i.config.ImporterEnabled || len(i.config.ImporterRelays) == 0 {
		log.Println("HAVEN Importer: disabled (no relays configured)")
		return
	}

	if i.storeFunc == nil {
		log.Println("HAVEN Importer: disabled (no store function provided)")
		return
	}

	i.wg.Add(1)
	go i.worker()

	log.Printf("HAVEN Importer: started, polling %d relays", len(i.config.ImporterRelays))
}

// Stop gracefully shuts down the importer
func (i *Importer) Stop() {
	i.cancel()
	i.wg.Wait()
	i.relayPool.Close()
	log.Println("HAVEN Importer: stopped")
}

// worker runs the polling loop
func (i *Importer) worker() {
	defer i.wg.Done()

	// Initial fetch
	i.pollAllRelays()

	// Set up ticker for periodic polling
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-i.ctx.Done():
			return
		case <-ticker.C:
			i.pollAllRelays()
		}
	}
}

// pollAllRelays fetches events from all configured relays
func (i *Importer) pollAllRelays() {
	i.mu.Lock()
	i.stats.LastPollTime = time.Now()
	i.mu.Unlock()

	var wg sync.WaitGroup

	for _, relayURL := range i.config.ImporterRelays {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			i.pollRelay(url)
		}(relayURL)
	}

	wg.Wait()

	// Clean up old seen events (keep last 1000)
	i.cleanupSeenEvents()

	i.mu.Lock()
	i.stats.RelaysPolled = len(i.config.ImporterRelays)
	i.mu.Unlock()
}

// pollRelay fetches events from a single relay
func (i *Importer) pollRelay(relayURL string) {
	ctx, cancel := context.WithTimeout(i.ctx, 30*time.Second)
	defer cancel()

	// Connect to relay
	relay, err := i.relayPool.Connect(ctx, relayURL)
	if err != nil {
		log.Printf("HAVEN Importer: failed to connect to %s: %v", relayURL, err)
		i.mu.Lock()
		i.stats.FetchErrors++
		i.mu.Unlock()
		return
	}

	// Determine time range
	i.mu.RLock()
	since := i.lastFetch[relayURL]
	i.mu.RUnlock()

	if since.IsZero() {
		// First fetch - look back 24 hours
		since = time.Now().Add(-24 * time.Hour)
	}

	sinceTimestamp := nostr.Timestamp(since.Unix())

	// Build filter for events addressed to owner
	filter := nostr.Filter{
		Tags: nostr.TagMap{
			"p": []string{i.config.OwnerPubkey},
		},
		Since: &sinceTimestamp,
		Limit: 100,
	}

	// Add inbox kinds if we want to be specific
	// For now, fetch all kinds that tag the owner
	// filter.Kinds = DefaultInboxKinds

	// Subscribe and collect events
	events, err := relay.QuerySync(ctx, filter)
	if err != nil {
		log.Printf("HAVEN Importer: query failed for %s: %v", relayURL, err)
		i.mu.Lock()
		i.stats.FetchErrors++
		i.mu.Unlock()
		return
	}

	// Process events
	imported := 0
	skipped := 0

	for _, event := range events {
		// Skip if we've seen this event recently
		i.mu.RLock()
		seen := i.seenEvents[event.ID]
		i.mu.RUnlock()

		if seen {
			skipped++
			continue
		}

		// Skip events from the owner (those go to outbox, not inbox)
		if event.PubKey == i.config.OwnerPubkey {
			skipped++
			continue
		}

		// Verify event routes to inbox
		box := i.router.RouteEvent(event)
		if box != BoxInbox && box != BoxChat {
			skipped++
			continue
		}

		// Store the event
		if err := i.storeFunc(ctx, event); err != nil {
			log.Printf("HAVEN Importer: failed to store event %s: %v", truncateID(event.ID), err)
			continue
		}

		// Mark as seen
		i.mu.Lock()
		i.seenEvents[event.ID] = true
		i.mu.Unlock()

		imported++
	}

	// Update last fetch time
	i.mu.Lock()
	i.lastFetch[relayURL] = time.Now()
	i.stats.EventsImported += int64(imported)
	i.stats.EventsSkipped += int64(skipped)
	if imported > 0 {
		i.stats.LastImport = time.Now()
	}
	i.mu.Unlock()

	if imported > 0 {
		log.Printf("HAVEN Importer: imported %d events from %s (skipped %d)",
			imported, relayURL, skipped)
	}
}

// cleanupSeenEvents removes old entries from the seen events map
func (i *Importer) cleanupSeenEvents() {
	i.mu.Lock()
	defer i.mu.Unlock()

	if len(i.seenEvents) > 1000 {
		// Simple cleanup: clear the map
		// In production, you'd want LRU or time-based expiry
		i.seenEvents = make(map[string]bool)
	}
}

// Stats returns current import statistics
func (i *Importer) Stats() ImporterStats {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.stats
}

// ForcePoll triggers an immediate poll of all relays
func (i *Importer) ForcePoll() {
	go i.pollAllRelays()
}

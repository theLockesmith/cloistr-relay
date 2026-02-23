package haven

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

// Subscriber maintains real-time WebSocket subscriptions to import inbox events
type Subscriber struct {
	config    *Config
	router    *Router
	storeFunc func(context.Context, *nostr.Event) error
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	mu        sync.RWMutex

	// Subscription state per relay
	relays map[string]*relaySubscription

	// Deduplication
	seenEvents map[string]time.Time

	// Stats
	stats   SubscriberStats
	metrics *Metrics
}

// relaySubscription tracks a single relay's subscription state
type relaySubscription struct {
	url           string
	relay         *nostr.Relay
	sub           *nostr.Subscription
	connected     bool
	lastConnected time.Time
	lastEvent     time.Time
	reconnectAt   time.Time
	backoffSecs   int
	eventsRecv    int64
	errors        int64
	mu            sync.Mutex
}

// SubscriberStats tracks subscriber statistics
type SubscriberStats struct {
	EventsReceived   int64
	EventsImported   int64
	EventsSkipped    int64
	RelaysConnected  int
	RelaysTotal      int
	Reconnects       int64
	LastEventTime    time.Time
}

// NewSubscriber creates a new real-time event subscriber
func NewSubscriber(cfg *Config, storeFunc func(context.Context, *nostr.Event) error) *Subscriber {
	ctx, cancel := context.WithCancel(context.Background())

	return &Subscriber{
		config:     cfg,
		router:     NewRouter(cfg),
		storeFunc:  storeFunc,
		ctx:        ctx,
		cancel:     cancel,
		relays:     make(map[string]*relaySubscription),
		seenEvents: make(map[string]time.Time),
		metrics:    GetMetrics(),
	}
}

// Start begins the subscription manager
func (s *Subscriber) Start() {
	if !s.config.ImporterEnabled || len(s.config.ImporterRelays) == 0 {
		log.Println("HAVEN Subscriber: disabled (no relays configured)")
		return
	}

	if !s.config.ImporterRealtimeEnabled {
		log.Println("HAVEN Subscriber: disabled (realtime mode not enabled)")
		return
	}

	if s.storeFunc == nil {
		log.Println("HAVEN Subscriber: disabled (no store function provided)")
		return
	}

	// Initialize relay subscriptions
	for _, url := range s.config.ImporterRelays {
		s.relays[url] = &relaySubscription{
			url:         url,
			backoffSecs: 1,
		}
	}

	s.mu.Lock()
	s.stats.RelaysTotal = len(s.config.ImporterRelays)
	s.mu.Unlock()

	// Start connection manager
	s.wg.Add(1)
	go s.connectionManager()

	// Start dedup cleanup
	s.wg.Add(1)
	go s.dedupCleanup()

	log.Printf("HAVEN Subscriber: started real-time subscriptions to %d relays", len(s.config.ImporterRelays))
}

// Stop gracefully shuts down the subscriber
func (s *Subscriber) Stop() {
	s.cancel()
	s.wg.Wait()

	// Close all relay connections
	for _, rs := range s.relays {
		rs.mu.Lock()
		if rs.sub != nil {
			rs.sub.Close()
		}
		if rs.relay != nil {
			rs.relay.Close()
		}
		rs.mu.Unlock()
	}

	log.Println("HAVEN Subscriber: stopped")
}

// connectionManager maintains connections and subscriptions to all relays
func (s *Subscriber) connectionManager() {
	defer s.wg.Done()

	// Initial connection attempt for all relays
	for url, rs := range s.relays {
		s.wg.Add(1)
		go s.manageRelay(url, rs)
	}

	// Periodically check and report status
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.updateConnectedCount()
		}
	}
}

// manageRelay maintains connection and subscription for a single relay
func (s *Subscriber) manageRelay(url string, rs *relaySubscription) {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		// Check if we need to wait before reconnecting
		rs.mu.Lock()
		if !rs.reconnectAt.IsZero() && time.Now().Before(rs.reconnectAt) {
			waitDuration := time.Until(rs.reconnectAt)
			rs.mu.Unlock()
			select {
			case <-s.ctx.Done():
				return
			case <-time.After(waitDuration):
			}
			rs.mu.Lock()
		}
		rs.mu.Unlock()

		// Attempt to connect and subscribe
		err := s.connectAndSubscribe(url, rs)
		if err != nil {
			log.Printf("HAVEN Subscriber: connection to %s failed: %v", url, err)
			s.scheduleReconnect(rs)
			continue
		}

		// Connection successful, reset backoff
		rs.mu.Lock()
		rs.backoffSecs = 1
		rs.connected = true
		rs.lastConnected = time.Now()
		rs.mu.Unlock()

		s.updateConnectedCount()

		// Wait for subscription to end (disconnect or error)
		<-rs.sub.Context.Done()

		rs.mu.Lock()
		rs.connected = false
		rs.mu.Unlock()

		s.updateConnectedCount()

		log.Printf("HAVEN Subscriber: subscription to %s ended, will reconnect", url)
		s.scheduleReconnect(rs)
	}
}

// connectAndSubscribe establishes connection and creates subscription
func (s *Subscriber) connectAndSubscribe(url string, rs *relaySubscription) error {
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	// Connect to relay
	relay, err := nostr.RelayConnect(ctx, url)
	if err != nil {
		rs.mu.Lock()
		rs.errors++
		rs.mu.Unlock()
		s.metrics.RecordImporterFetchError()
		return err
	}

	// Create filter for events addressed to owner
	filters := nostr.Filters{{
		Tags: nostr.TagMap{
			"p": []string{s.config.OwnerPubkey},
		},
		// No Since - we want new events going forward
		// Polling handles historical events
	}}

	// Subscribe
	sub, err := relay.Subscribe(s.ctx, filters)
	if err != nil {
		relay.Close()
		rs.mu.Lock()
		rs.errors++
		rs.mu.Unlock()
		return err
	}

	rs.mu.Lock()
	rs.relay = relay
	rs.sub = sub
	rs.mu.Unlock()

	// Start event handler for this subscription
	s.wg.Add(1)
	go s.handleEvents(url, rs, sub)

	log.Printf("HAVEN Subscriber: connected and subscribed to %s", url)
	return nil
}

// handleEvents processes incoming events from a subscription
func (s *Subscriber) handleEvents(url string, rs *relaySubscription, sub *nostr.Subscription) {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-sub.Context.Done():
			return
		case event, ok := <-sub.Events:
			if !ok {
				return
			}
			s.processEvent(url, rs, event)
		}
	}
}

// processEvent handles a single incoming event
func (s *Subscriber) processEvent(url string, rs *relaySubscription, event *nostr.Event) {
	// Update stats
	rs.mu.Lock()
	rs.eventsRecv++
	rs.lastEvent = time.Now()
	rs.mu.Unlock()

	s.mu.Lock()
	s.stats.EventsReceived++
	s.stats.LastEventTime = time.Now()
	s.mu.Unlock()

	// Check deduplication
	s.mu.RLock()
	_, seen := s.seenEvents[event.ID]
	s.mu.RUnlock()

	if seen {
		s.mu.Lock()
		s.stats.EventsSkipped++
		s.mu.Unlock()
		s.metrics.RecordImporterSkipped()
		return
	}

	// Skip events from the owner (those go to outbox)
	if event.PubKey == s.config.OwnerPubkey {
		s.mu.Lock()
		s.stats.EventsSkipped++
		s.mu.Unlock()
		s.metrics.RecordImporterSkipped()
		return
	}

	// Verify event routes to inbox or chat
	box := s.router.RouteEvent(event)
	if box != BoxInbox && box != BoxChat {
		s.mu.Lock()
		s.stats.EventsSkipped++
		s.mu.Unlock()
		s.metrics.RecordImporterSkipped()
		return
	}

	// Store the event
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	if err := s.storeFunc(ctx, event); err != nil {
		log.Printf("HAVEN Subscriber: failed to store event %s from %s: %v",
			truncateID(event.ID), url, err)
		return
	}

	// Mark as seen
	s.mu.Lock()
	s.seenEvents[event.ID] = time.Now()
	s.stats.EventsImported++
	s.mu.Unlock()

	s.metrics.RecordImporterImported()

	log.Printf("HAVEN Subscriber: imported event %s (kind %d) from %s",
		truncateID(event.ID), event.Kind, url)
}

// scheduleReconnect sets up reconnection with exponential backoff
func (s *Subscriber) scheduleReconnect(rs *relaySubscription) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	// Exponential backoff: 1, 2, 4, 8, 16, 32, 60, 60, 60... seconds
	rs.backoffSecs *= 2
	if rs.backoffSecs > 60 {
		rs.backoffSecs = 60
	}

	rs.reconnectAt = time.Now().Add(time.Duration(rs.backoffSecs) * time.Second)

	s.mu.Lock()
	s.stats.Reconnects++
	s.mu.Unlock()
}

// updateConnectedCount updates the connected relay count
func (s *Subscriber) updateConnectedCount() {
	connected := 0
	for _, rs := range s.relays {
		rs.mu.Lock()
		if rs.connected {
			connected++
		}
		rs.mu.Unlock()
	}

	s.mu.Lock()
	s.stats.RelaysConnected = connected
	s.mu.Unlock()

	// Update metrics
	s.metrics.SetImporterRelaysPolled(connected)
}

// dedupCleanup periodically cleans old entries from the dedup map
func (s *Subscriber) dedupCleanup() {
	defer s.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.cleanupSeenEvents()
		}
	}
}

// cleanupSeenEvents removes entries older than 1 hour
func (s *Subscriber) cleanupSeenEvents() {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-1 * time.Hour)
	for id, seenAt := range s.seenEvents {
		if seenAt.Before(cutoff) {
			delete(s.seenEvents, id)
		}
	}
}

// Stats returns current subscriber statistics
func (s *Subscriber) Stats() SubscriberStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

// RelayStatus returns status for each relay
func (s *Subscriber) RelayStatus() map[string]RelaySubscriptionStatus {
	status := make(map[string]RelaySubscriptionStatus)

	for url, rs := range s.relays {
		rs.mu.Lock()
		status[url] = RelaySubscriptionStatus{
			URL:           url,
			Connected:     rs.connected,
			LastConnected: rs.lastConnected,
			LastEvent:     rs.lastEvent,
			EventsRecv:    rs.eventsRecv,
			Errors:        rs.errors,
		}
		rs.mu.Unlock()
	}

	return status
}

// RelaySubscriptionStatus represents status of a single relay subscription
type RelaySubscriptionStatus struct {
	URL           string
	Connected     bool
	LastConnected time.Time
	LastEvent     time.Time
	EventsRecv    int64
	Errors        int64
}

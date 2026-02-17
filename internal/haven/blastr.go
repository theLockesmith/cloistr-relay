package haven

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
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
}

// BlastrStats tracks broadcast statistics
type BlastrStats struct {
	EventsBroadcast int64
	EventsFailed    int64
	RelaysConnected int
	LastBroadcast   time.Time
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

	return &Blastr{
		config:     cfg,
		router:     NewRouter(cfg),
		relayPool:  NewRelayPool(),
		eventQueue: make(chan *nostr.Event, 100),
		ctx:        ctx,
		cancel:     cancel,
		metrics:    GetMetrics(),
	}
}

// Start begins the broadcast worker
func (b *Blastr) Start() {
	if !b.config.BlastrEnabled || len(b.config.BlastrRelays) == 0 {
		log.Println("HAVEN Blastr: disabled (no relays configured)")
		return
	}

	b.wg.Add(1)
	go b.worker()

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
				return
			}

			b.metrics.RecordBlastrRelayPublish(url, true)
			mu.Lock()
			successCount++
			mu.Unlock()
		}(relayURL)
	}

	wg.Wait()

	b.mu.Lock()
	if successCount > 0 {
		b.stats.EventsBroadcast++
		b.stats.LastBroadcast = time.Now()
		b.metrics.RecordBlastrBroadcast()
	} else {
		b.stats.EventsFailed++
		b.metrics.RecordBlastrFailed()
	}
	b.stats.RelaysConnected = b.relayPool.ConnectedCount()
	b.metrics.SetBlastrRelaysConnected(b.stats.RelaysConnected)
	b.mu.Unlock()

	log.Printf("HAVEN Blastr: broadcast event %s to %d/%d relays",
		truncateID(event.ID), successCount, len(b.config.BlastrRelays))
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

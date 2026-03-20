package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"
	"github.com/redis/go-redis/v9"
)

const (
	// ChannelName is the Redis pub/sub channel for relay events
	ChannelName = "relay:events"
	// DefaultRetryInterval is the interval between reconnection attempts
	DefaultRetryInterval = 5 * time.Second
	// PublishBufferSize is the size of the async publish channel
	PublishBufferSize = 1000
)

// Config holds pub/sub configuration
type Config struct {
	Enabled bool // Enable cross-pod event broadcasting
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled: true,
	}
}

// PubSub handles cross-pod event broadcasting via Redis/Dragonfly pub/sub
type PubSub struct {
	rdb    *redis.Client
	relay  *khatru.Relay
	config *Config
	podID  string // Unique identifier for this pod to avoid echo

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	publishCh chan *nostr.Event // Async publish channel
}

// eventMessage is the wire format for pub/sub messages
type eventMessage struct {
	PodID string       `json:"pod_id"`
	Event *nostr.Event `json:"event"`
}

// New creates a new PubSub instance
func New(rdb *redis.Client, relay *khatru.Relay, cfg *Config) *PubSub {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Generate unique pod ID for this instance
	podID := fmt.Sprintf("%d-%d", time.Now().UnixNano(), time.Now().Nanosecond())

	ctx, cancel := context.WithCancel(context.Background())

	return &PubSub{
		rdb:       rdb,
		relay:     relay,
		config:    cfg,
		podID:     podID,
		ctx:       ctx,
		cancel:    cancel,
		publishCh: make(chan *nostr.Event, PublishBufferSize),
	}
}

// Start begins the pub/sub subscription and publish loop
func (ps *PubSub) Start() {
	if ps.rdb == nil || !ps.config.Enabled {
		return
	}

	ps.wg.Add(2)
	go ps.subscribeLoop()
	go ps.publishLoop()

	log.Printf("Cross-pod pub/sub started (pod ID: %s...)", ps.podID[:16])
}

// Stop gracefully shuts down the pub/sub subscription
func (ps *PubSub) Stop() {
	ps.cancel()
	ps.wg.Wait()
	log.Println("Cross-pod pub/sub stopped")
}

// subscribeLoop maintains the Redis subscription with automatic reconnection
func (ps *PubSub) subscribeLoop() {
	defer ps.wg.Done()

	for {
		select {
		case <-ps.ctx.Done():
			return
		default:
			ps.subscribe()
			// If we get here, subscription ended - retry after delay
			select {
			case <-ps.ctx.Done():
				return
			case <-time.After(DefaultRetryInterval):
				log.Println("Reconnecting to pub/sub channel...")
			}
		}
	}
}

// subscribe handles the actual Redis subscription
func (ps *PubSub) subscribe() {
	pubsub := ps.rdb.Subscribe(ps.ctx, ChannelName)
	defer func() { _ = pubsub.Close() }()

	// Wait for subscription confirmation
	_, err := pubsub.Receive(ps.ctx)
	if err != nil {
		log.Printf("Pub/sub subscription error: %v", err)
		return
	}

	log.Println("Subscribed to pub/sub channel")

	ch := pubsub.Channel()
	for {
		select {
		case <-ps.ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				log.Println("Pub/sub channel closed")
				return
			}
			ps.handleMessage(msg)
		}
	}
}

// handleMessage processes incoming pub/sub messages
func (ps *PubSub) handleMessage(msg *redis.Message) {
	var em eventMessage
	if err := json.Unmarshal([]byte(msg.Payload), &em); err != nil {
		log.Printf("Pub/sub: failed to unmarshal message: %v", err)
		return
	}

	// Skip messages from ourselves (avoid echo)
	if em.PodID == ps.podID {
		return
	}

	// Validate event
	if em.Event == nil {
		return
	}

	// Broadcast to local subscribers
	n := ps.relay.BroadcastEvent(em.Event)
	if n > 0 {
		log.Printf("Pub/sub: broadcast event %s to %d local subscribers", em.Event.ID[:8], n)
	}
}

// publishLoop handles async publishing of events to Redis
func (ps *PubSub) publishLoop() {
	defer ps.wg.Done()

	for {
		select {
		case <-ps.ctx.Done():
			return
		case event, ok := <-ps.publishCh:
			if !ok {
				return
			}
			ps.doPublish(event)
		}
	}
}

// doPublish performs the actual Redis publish (called from publishLoop goroutine)
func (ps *PubSub) doPublish(event *nostr.Event) {
	em := eventMessage{
		PodID: ps.podID,
		Event: event,
	}

	data, err := json.Marshal(em)
	if err != nil {
		log.Printf("Pub/sub: failed to marshal event %s: %v", event.ID[:8], err)
		return
	}

	if err := ps.rdb.Publish(ps.ctx, ChannelName, data).Err(); err != nil {
		log.Printf("Pub/sub: failed to publish event %s: %v", event.ID[:8], err)
	}
}

// Publish queues an event for async publishing to other relay pods
func (ps *PubSub) Publish(ctx context.Context, event *nostr.Event) error {
	if ps.rdb == nil || !ps.config.Enabled {
		return nil
	}

	// Non-blocking send - drop if buffer is full (better than blocking)
	select {
	case ps.publishCh <- event:
	default:
		log.Printf("Pub/sub: publish buffer full, dropping event %s", event.ID[:8])
	}

	return nil
}

// CreateStoreEventHook returns a handler that publishes events to other pods after storage
// This should be registered with relay.StoreEvent
func (ps *PubSub) CreateStoreEventHook() func(context.Context, *nostr.Event) error {
	return func(ctx context.Context, event *nostr.Event) error {
		// Publish to other pods (fire and forget - don't fail the store if pub/sub fails)
		if err := ps.Publish(ctx, event); err != nil {
			log.Printf("Pub/sub: failed to publish event %s: %v", event.ID[:8], err)
		}
		return nil
	}
}

// CreateEphemeralEventHook returns a handler that publishes ephemeral events to other pods
// This should be registered with relay.OnEphemeralEvent for NIP-46 and other ephemeral kinds
func (ps *PubSub) CreateEphemeralEventHook() func(context.Context, *nostr.Event) {
	return func(ctx context.Context, event *nostr.Event) {
		// Publish to other pods (fire and forget)
		if err := ps.Publish(ctx, event); err != nil {
			log.Printf("Pub/sub: failed to publish ephemeral event %s: %v", event.ID[:8], err)
		}
	}
}

// Package eventcache provides a hot event cache using Redis/Dragonfly
//
// This caches frequently accessed events for fast retrieval without
// hitting PostgreSQL. Common use cases:
// - Profile metadata (kind 0)
// - Recent popular notes
// - Contact lists (kind 3)
// - Relay lists (kind 10002)
package eventcache

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/redis/go-redis/v9"
)

// CacheableKinds are event kinds that benefit most from caching
var CacheableKinds = map[int]bool{
	0:     true, // Profile metadata
	3:     true, // Contact list
	10002: true, // Relay list
}

// Config holds event cache configuration
type Config struct {
	// Enabled activates the event cache
	Enabled bool
	// DefaultTTL is the default cache duration for events
	DefaultTTL time.Duration
	// ProfileTTL is cache duration for kind 0 (profiles)
	ProfileTTL time.Duration
	// ContactsTTL is cache duration for kind 3 (contacts)
	ContactsTTL time.Duration
	// KeyPrefix is the Redis key prefix for cached events
	KeyPrefix string
	// MaxCacheSize is max events to cache (0 = unlimited)
	MaxCacheSize int
}

// DefaultConfig returns sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Enabled:      true,
		DefaultTTL:   5 * time.Minute,
		ProfileTTL:   15 * time.Minute,
		ContactsTTL:  10 * time.Minute,
		KeyPrefix:    "event:",
		MaxCacheSize: 100000,
	}
}

// Cache provides event caching via Redis/Dragonfly
type Cache struct {
	rdb    *redis.Client
	config *Config
}

// New creates a new event cache
func New(rdb *redis.Client, cfg *Config) *Cache {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Cache{
		rdb:    rdb,
		config: cfg,
	}
}

// eventKey generates the Redis key for an event by ID
func (c *Cache) eventKey(eventID string) string {
	return c.config.KeyPrefix + "id:" + eventID
}

// pubkeyKindKey generates the Redis key for a pubkey+kind lookup
// Used for replaceable events like profiles (kind 0)
func (c *Cache) pubkeyKindKey(pubkey string, kind int) string {
	return fmt.Sprintf("%spk:%s:%d", c.config.KeyPrefix, pubkey, kind)
}

// getTTL returns the appropriate TTL for an event kind
func (c *Cache) getTTL(kind int) time.Duration {
	switch kind {
	case 0:
		return c.config.ProfileTTL
	case 3:
		return c.config.ContactsTTL
	default:
		return c.config.DefaultTTL
	}
}

// Set caches an event
func (c *Cache) Set(ctx context.Context, event *nostr.Event) error {
	if c.rdb == nil || !c.config.Enabled {
		return nil
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	ttl := c.getTTL(event.Kind)

	// Store by event ID
	if err := c.rdb.Set(ctx, c.eventKey(event.ID), data, ttl).Err(); err != nil {
		log.Printf("Event cache error (set): %v", err)
		return nil // Fail open
	}

	// For replaceable events (kinds 0, 3, 10002, 30000-39999), also index by pubkey+kind
	if isReplaceable(event.Kind) {
		if err := c.rdb.Set(ctx, c.pubkeyKindKey(event.PubKey, event.Kind), data, ttl).Err(); err != nil {
			log.Printf("Event cache error (set pubkey index): %v", err)
		}
	}

	return nil
}

// Get retrieves an event by ID
func (c *Cache) Get(ctx context.Context, eventID string) (*nostr.Event, error) {
	if c.rdb == nil || !c.config.Enabled {
		return nil, nil
	}

	data, err := c.rdb.Get(ctx, c.eventKey(eventID)).Bytes()
	if err == redis.Nil {
		return nil, nil // Cache miss
	}
	if err != nil {
		log.Printf("Event cache error (get): %v", err)
		return nil, nil // Fail open
	}

	var event nostr.Event
	if err := json.Unmarshal(data, &event); err != nil {
		log.Printf("Event cache unmarshal error: %v", err)
		return nil, nil
	}

	return &event, nil
}

// GetByPubkeyKind retrieves a replaceable event by pubkey and kind
func (c *Cache) GetByPubkeyKind(ctx context.Context, pubkey string, kind int) (*nostr.Event, error) {
	if c.rdb == nil || !c.config.Enabled {
		return nil, nil
	}

	if !isReplaceable(kind) {
		return nil, nil
	}

	data, err := c.rdb.Get(ctx, c.pubkeyKindKey(pubkey, kind)).Bytes()
	if err == redis.Nil {
		return nil, nil // Cache miss
	}
	if err != nil {
		log.Printf("Event cache error (get by pubkey): %v", err)
		return nil, nil // Fail open
	}

	var event nostr.Event
	if err := json.Unmarshal(data, &event); err != nil {
		log.Printf("Event cache unmarshal error: %v", err)
		return nil, nil
	}

	return &event, nil
}

// GetProfile retrieves a cached profile (kind 0) for a pubkey
func (c *Cache) GetProfile(ctx context.Context, pubkey string) (*nostr.Event, error) {
	return c.GetByPubkeyKind(ctx, pubkey, 0)
}

// GetContacts retrieves a cached contact list (kind 3) for a pubkey
func (c *Cache) GetContacts(ctx context.Context, pubkey string) (*nostr.Event, error) {
	return c.GetByPubkeyKind(ctx, pubkey, 3)
}

// Delete removes an event from cache
func (c *Cache) Delete(ctx context.Context, eventID string) error {
	if c.rdb == nil || !c.config.Enabled {
		return nil
	}

	// First get the event to find its pubkey+kind for index cleanup
	event, _ := c.Get(ctx, eventID)

	if err := c.rdb.Del(ctx, c.eventKey(eventID)).Err(); err != nil {
		log.Printf("Event cache error (delete): %v", err)
	}

	// Clean up pubkey+kind index if it was a replaceable event
	if event != nil && isReplaceable(event.Kind) {
		// Only delete index if it points to this event
		indexKey := c.pubkeyKindKey(event.PubKey, event.Kind)
		cachedEvent, _ := c.GetByPubkeyKind(ctx, event.PubKey, event.Kind)
		if cachedEvent != nil && cachedEvent.ID == eventID {
			c.rdb.Del(ctx, indexKey)
		}
	}

	return nil
}

// MultiGet retrieves multiple events by ID
func (c *Cache) MultiGet(ctx context.Context, eventIDs []string) ([]*nostr.Event, error) {
	if c.rdb == nil || !c.config.Enabled || len(eventIDs) == 0 {
		return nil, nil
	}

	// Build keys
	keys := make([]string, len(eventIDs))
	for i, id := range eventIDs {
		keys[i] = c.eventKey(id)
	}

	// Batch get
	results, err := c.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		log.Printf("Event cache error (mget): %v", err)
		return nil, nil // Fail open
	}

	var events []*nostr.Event
	for _, result := range results {
		if result == nil {
			continue
		}
		str, ok := result.(string)
		if !ok {
			continue
		}
		var event nostr.Event
		if err := json.Unmarshal([]byte(str), &event); err == nil {
			events = append(events, &event)
		}
	}

	return events, nil
}

// Invalidate removes events matching certain criteria
// Used when receiving new replaceable events
func (c *Cache) Invalidate(ctx context.Context, pubkey string, kind int) error {
	if c.rdb == nil || !c.config.Enabled {
		return nil
	}

	if isReplaceable(kind) {
		c.rdb.Del(ctx, c.pubkeyKindKey(pubkey, kind))
	}

	return nil
}

// IsCacheable returns true if an event kind should be cached
func IsCacheable(kind int) bool {
	if CacheableKinds[kind] {
		return true
	}
	// Parameterized replaceable events (30000-39999) are cacheable
	if kind >= 30000 && kind < 40000 {
		return true
	}
	return false
}

// isReplaceable returns true if an event kind is replaceable (NIP-01)
func isReplaceable(kind int) bool {
	if kind == 0 || kind == 3 {
		return true
	}
	// NIP-01: kinds 10000-19999 are replaceable
	if kind >= 10000 && kind < 20000 {
		return true
	}
	// NIP-01: kinds 30000-39999 are parameterized replaceable
	if kind >= 30000 && kind < 40000 {
		return true
	}
	return false
}

// Stats returns cache statistics
type Stats struct {
	Hits       int64
	Misses     int64
	EventCount int64
}

// GetStats returns cache statistics (requires Redis INFO)
func (c *Cache) GetStats(ctx context.Context) (*Stats, error) {
	if c.rdb == nil || !c.config.Enabled {
		return &Stats{}, nil
	}

	// Count cached events
	var count int64
	var cursor uint64

	for {
		keys, nextCursor, err := c.rdb.Scan(ctx, cursor, c.config.KeyPrefix+"id:*", 100).Result()
		if err != nil {
			log.Printf("Event cache error (stats): %v", err)
			break
		}

		count += int64(len(keys))
		cursor = nextCursor

		if cursor == 0 {
			break
		}
	}

	return &Stats{
		EventCount: count,
	}, nil
}

// WarmCache pre-populates the cache with events from a channel
// Useful for startup or cache reconstruction
func (c *Cache) WarmCache(ctx context.Context, events <-chan *nostr.Event) (int, error) {
	if c.rdb == nil || !c.config.Enabled {
		return 0, nil
	}

	count := 0
	for event := range events {
		if IsCacheable(event.Kind) {
			if err := c.Set(ctx, event); err != nil {
				log.Printf("Warm cache error: %v", err)
			} else {
				count++
			}
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return count, ctx.Err()
		default:
		}
	}

	return count, nil
}

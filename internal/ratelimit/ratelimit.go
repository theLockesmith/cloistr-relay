// Package ratelimit provides Redis/Dragonfly-based rate limiting
//
// This replaces khatru's in-memory rate limiting with a distributed
// rate limiter that works across multiple relay replicas.
//
// Uses a sliding window algorithm with Redis sorted sets for accuracy
// and efficiency.
package ratelimit

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"
	"github.com/redis/go-redis/v9"
)

// Config holds rate limiter configuration
type Config struct {
	// Enabled activates distributed rate limiting
	Enabled bool
	// EventsPerSecond limits events per second per IP
	EventsPerSecond int
	// FiltersPerSecond limits filter queries per second per IP
	FiltersPerSecond int
	// ConnectionsPerSecond limits new connections per second per IP
	ConnectionsPerSecond int
	// BurstMultiplier allows short bursts (e.g., 5 means 5x the rate for bursts)
	BurstMultiplier int
	// WindowSize is the sliding window duration
	WindowSize time.Duration
}

// DefaultConfig returns sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Enabled:              true,
		EventsPerSecond:      10,
		FiltersPerSecond:     20,
		ConnectionsPerSecond: 5,
		BurstMultiplier:      5,
		WindowSize:           time.Second,
	}
}

// Limiter provides distributed rate limiting via Redis/Dragonfly
type Limiter struct {
	rdb    *redis.Client
	config *Config
}

// New creates a new distributed rate limiter
func New(rdb *redis.Client, cfg *Config) *Limiter {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Limiter{
		rdb:    rdb,
		config: cfg,
	}
}

// keyPrefix returns the Redis key prefix for a rate limit type
func (l *Limiter) keyPrefix(limitType string) string {
	return fmt.Sprintf("ratelimit:%s:", limitType)
}

// Allow checks if an action is allowed and records it
// Uses sliding window log algorithm with Redis sorted sets
func (l *Limiter) Allow(ctx context.Context, limitType, identifier string, limit int) (bool, error) {
	if l.rdb == nil || !l.config.Enabled {
		return true, nil // No rate limiting if Redis not available
	}

	key := l.keyPrefix(limitType) + identifier
	now := time.Now()
	windowStart := now.Add(-l.config.WindowSize)

	// Use a pipeline for atomic operations
	pipe := l.rdb.Pipeline()

	// Remove old entries outside the window
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart.UnixNano()))

	// Count current entries in window
	countCmd := pipe.ZCard(ctx, key)

	// Execute pipeline
	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		// On Redis error, allow the request (fail open)
		log.Printf("Rate limit Redis error: %v", err)
		return true, nil
	}

	count := countCmd.Val()
	burstLimit := int64(limit * l.config.BurstMultiplier)

	if count >= burstLimit {
		return false, nil
	}

	// Add this request to the window
	_, err = l.rdb.ZAdd(ctx, key, redis.Z{
		Score:  float64(now.UnixNano()),
		Member: fmt.Sprintf("%d", now.UnixNano()),
	}).Result()
	if err != nil {
		log.Printf("Rate limit Redis error on add: %v", err)
	}

	// Set expiry on the key (window size + buffer)
	if err := l.rdb.Expire(ctx, key, l.config.WindowSize*2).Err(); err != nil {
		log.Printf("Rate limit Redis error on expire: %v", err)
	}

	return true, nil
}

// RejectEventByRateLimit returns a handler that rate limits events by IP
func (l *Limiter) RejectEventByRateLimit() func(context.Context, *nostr.Event) (bool, string) {
	return func(ctx context.Context, event *nostr.Event) (bool, string) {
		if l.config.EventsPerSecond <= 0 {
			return false, ""
		}

		ip := khatru.GetIP(ctx)
		if ip == "" {
			return false, "" // Can't rate limit without IP
		}

		allowed, err := l.Allow(ctx, "events", ip, l.config.EventsPerSecond)
		if err != nil {
			log.Printf("Rate limit error: %v", err)
			return false, "" // Fail open
		}

		if !allowed {
			return true, "rate-limited: too many events, slow down"
		}

		return false, ""
	}
}

// RejectFilterByRateLimit returns a handler that rate limits filter queries by IP
func (l *Limiter) RejectFilterByRateLimit() func(context.Context, nostr.Filter) (bool, string) {
	return func(ctx context.Context, filter nostr.Filter) (bool, string) {
		if l.config.FiltersPerSecond <= 0 {
			return false, ""
		}

		ip := khatru.GetIP(ctx)
		if ip == "" {
			return false, ""
		}

		allowed, err := l.Allow(ctx, "filters", ip, l.config.FiltersPerSecond)
		if err != nil {
			log.Printf("Rate limit error: %v", err)
			return false, ""
		}

		if !allowed {
			return true, "rate-limited: too many queries, slow down"
		}

		return false, ""
	}
}

// RejectConnectionByRateLimit returns a handler that rate limits new connections by IP
func (l *Limiter) RejectConnectionByRateLimit() func(*http.Request) bool {
	return func(r *http.Request) bool {
		if l.config.ConnectionsPerSecond <= 0 {
			return false // Don't reject
		}

		ip := khatru.GetIPFromRequest(r)
		if ip == "" {
			return false
		}

		// Use background context for Redis operations since we don't have a request context
		ctx := context.Background()
		allowed, err := l.Allow(ctx, "connections", ip, l.config.ConnectionsPerSecond)
		if err != nil {
			log.Printf("Rate limit error: %v", err)
			return false
		}

		return !allowed // Return true to reject
	}
}

// GetStats returns current rate limit statistics for an identifier
func (l *Limiter) GetStats(ctx context.Context, limitType, identifier string) (int64, error) {
	if l.rdb == nil {
		return 0, nil
	}

	key := l.keyPrefix(limitType) + identifier
	windowStart := time.Now().Add(-l.config.WindowSize)

	// Count entries in the current window
	count, err := l.rdb.ZCount(ctx, key, fmt.Sprintf("%d", windowStart.UnixNano()), "+inf").Result()
	if err != nil && err != redis.Nil {
		return 0, err
	}

	return count, nil
}

// ClearLimit clears rate limit data for an identifier
func (l *Limiter) ClearLimit(ctx context.Context, limitType, identifier string) error {
	if l.rdb == nil {
		return nil
	}

	key := l.keyPrefix(limitType) + identifier
	return l.rdb.Del(ctx, key).Err()
}

// RegisterHandlers registers rate limiting handlers with the relay
// This replaces khatru's built-in rate limiting with distributed limits
func RegisterHandlers(relay *khatru.Relay, rdb *redis.Client, cfg *Config) *Limiter {
	limiter := New(rdb, cfg)

	if cfg.EventsPerSecond > 0 {
		relay.RejectEvent = append(relay.RejectEvent, limiter.RejectEventByRateLimit())
		log.Printf("Distributed rate limit: %d events/sec per IP (Redis)", cfg.EventsPerSecond)
	}

	if cfg.FiltersPerSecond > 0 {
		relay.RejectFilter = append(relay.RejectFilter, limiter.RejectFilterByRateLimit())
		log.Printf("Distributed rate limit: %d filters/sec per IP (Redis)", cfg.FiltersPerSecond)
	}

	if cfg.ConnectionsPerSecond > 0 {
		relay.RejectConnection = append(relay.RejectConnection, limiter.RejectConnectionByRateLimit())
		log.Printf("Distributed rate limit: %d connections/sec per IP (Redis)", cfg.ConnectionsPerSecond)
	}

	return limiter
}

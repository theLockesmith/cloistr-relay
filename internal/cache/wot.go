package cache

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

const (
	// Key prefixes
	prefixWoTScore    = "wot:score:"    // wot:score:{pubkey} -> trust level
	prefixWoTFollows  = "wot:follows:"  // wot:follows:{pubkey} -> set of followees
	prefixWoTPageRank = "wot:pagerank:" // wot:pagerank:{pubkey} -> PageRank score
)

// WoTCache provides WoT-specific caching operations
type WoTCache struct {
	client *Client
	ttl    time.Duration
}

// NewWoTCache creates a new WoT cache wrapper
func NewWoTCache(client *Client, ttl time.Duration) *WoTCache {
	if ttl == 0 {
		ttl = 5 * time.Minute
	}
	return &WoTCache{
		client: client,
		ttl:    ttl,
	}
}

// IsEnabled returns true if caching is enabled
func (w *WoTCache) IsEnabled() bool {
	return w != nil && w.client != nil
}

// GetTrustLevel retrieves a cached trust level for a pubkey
func (w *WoTCache) GetTrustLevel(ctx context.Context, pubkey string) (int, bool) {
	if !w.IsEnabled() {
		return 0, false
	}

	key := prefixWoTScore + pubkey
	val, err := w.client.Get(ctx, key)
	if err != nil {
		return 0, false
	}

	level, err := strconv.Atoi(val)
	if err != nil {
		return 0, false
	}

	return level, true
}

// SetTrustLevel caches a trust level for a pubkey
func (w *WoTCache) SetTrustLevel(ctx context.Context, pubkey string, level int) error {
	if !w.IsEnabled() {
		return nil
	}

	key := prefixWoTScore + pubkey
	return w.client.Set(ctx, key, strconv.Itoa(level), w.ttl)
}

// InvalidateTrustLevel removes a cached trust level
func (w *WoTCache) InvalidateTrustLevel(ctx context.Context, pubkey string) error {
	if !w.IsEnabled() {
		return nil
	}

	key := prefixWoTScore + pubkey
	return w.client.Delete(ctx, key)
}

// GetFollows retrieves cached follows for a pubkey
func (w *WoTCache) GetFollows(ctx context.Context, pubkey string) ([]string, bool) {
	if !w.IsEnabled() {
		return nil, false
	}

	key := prefixWoTFollows + pubkey
	follows, err := w.client.SMembers(ctx, key)
	if err != nil || len(follows) == 0 {
		return nil, false
	}

	return follows, true
}

// SetFollows caches follows for a pubkey
func (w *WoTCache) SetFollows(ctx context.Context, pubkey string, follows []string) error {
	if !w.IsEnabled() || len(follows) == 0 {
		return nil
	}

	key := prefixWoTFollows + pubkey

	// Delete existing set first
	_ = w.client.Delete(ctx, key)

	// Add all follows
	members := make([]interface{}, len(follows))
	for i, f := range follows {
		members[i] = f
	}
	if err := w.client.SAdd(ctx, key, members...); err != nil {
		return err
	}

	// Set TTL
	return w.client.Expire(ctx, key, w.ttl)
}

// IsFollowing checks if a follow relationship exists (cached)
func (w *WoTCache) IsFollowing(ctx context.Context, follower, followee string) (bool, bool) {
	if !w.IsEnabled() {
		return false, false
	}

	key := prefixWoTFollows + follower
	exists, err := w.client.Exists(ctx, key)
	if err != nil || !exists {
		return false, false // Cache miss
	}

	isFollowing, err := w.client.SIsMember(ctx, key, followee)
	if err != nil {
		return false, false
	}

	return isFollowing, true // Cache hit
}

// InvalidateFollows removes cached follows for a pubkey
func (w *WoTCache) InvalidateFollows(ctx context.Context, pubkey string) error {
	if !w.IsEnabled() {
		return nil
	}

	key := prefixWoTFollows + pubkey
	return w.client.Delete(ctx, key)
}

// GetPageRank retrieves a cached PageRank score for a pubkey
func (w *WoTCache) GetPageRank(ctx context.Context, pubkey string) (float64, bool) {
	if !w.IsEnabled() {
		return 0, false
	}

	key := prefixWoTPageRank + pubkey
	val, err := w.client.Get(ctx, key)
	if err != nil {
		return 0, false
	}

	score, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0, false
	}

	return score, true
}

// SetPageRank caches a PageRank score for a pubkey
func (w *WoTCache) SetPageRank(ctx context.Context, pubkey string, score float64) error {
	if !w.IsEnabled() {
		return nil
	}

	key := prefixWoTPageRank + pubkey
	return w.client.Set(ctx, key, fmt.Sprintf("%.10f", score), w.ttl)
}

// SetPageRankBatch caches multiple PageRank scores efficiently
func (w *WoTCache) SetPageRankBatch(ctx context.Context, scores map[string]float64, ttl time.Duration) error {
	if !w.IsEnabled() || len(scores) == 0 {
		return nil
	}

	if ttl == 0 {
		ttl = w.ttl
	}

	// Use pipeline for efficiency
	pipe := w.client.rdb.Pipeline()
	for pubkey, score := range scores {
		key := prefixWoTPageRank + pubkey
		pipe.Set(ctx, key, fmt.Sprintf("%.10f", score), ttl)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// ClearWoTCache removes all WoT-related cached data
func (w *WoTCache) ClearWoTCache(ctx context.Context) error {
	if !w.IsEnabled() {
		return nil
	}

	// Get all WoT keys (be careful with this in production with large datasets)
	patterns := []string{
		prefixWoTScore + "*",
		prefixWoTFollows + "*",
		prefixWoTPageRank + "*",
	}

	for _, pattern := range patterns {
		keys, err := w.client.Keys(ctx, pattern)
		if err != nil {
			continue
		}
		if len(keys) > 0 {
			if err := w.client.Delete(ctx, keys...); err != nil {
				return err
			}
		}
	}

	return nil
}

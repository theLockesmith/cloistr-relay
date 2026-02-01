package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client wraps a Redis/Dragonfly connection
type Client struct {
	rdb *redis.Client
}

// Config holds cache configuration
type Config struct {
	URL     string        // Redis/Dragonfly URL (e.g., redis://dragonfly:6379)
	Prefix  string        // Key prefix for this application
	Enabled bool          // Whether caching is enabled
	TTL     time.Duration // Default TTL for cached items
}

// New creates a new cache client from a URL
// URL format: redis://[password@]host:port[/db]
func New(cfg *Config) (*Client, error) {
	if !cfg.Enabled || cfg.URL == "" {
		return nil, nil
	}

	opts, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid cache URL: %w", err)
	}

	rdb := redis.NewClient(opts)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to cache: %w", err)
	}

	log.Printf("Connected to cache at %s", opts.Addr)
	return &Client{rdb: rdb}, nil
}

// Close closes the cache connection
func (c *Client) Close() error {
	if c == nil || c.rdb == nil {
		return nil
	}
	return c.rdb.Close()
}

// Ping checks if the cache is reachable
func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.rdb == nil {
		return fmt.Errorf("cache not connected")
	}
	return c.rdb.Ping(ctx).Err()
}

// Get retrieves a value from the cache
func (c *Client) Get(ctx context.Context, key string) (string, error) {
	if c == nil || c.rdb == nil {
		return "", fmt.Errorf("cache not connected")
	}
	return c.rdb.Get(ctx, key).Result()
}

// Set stores a value in the cache with TTL
func (c *Client) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if c == nil || c.rdb == nil {
		return fmt.Errorf("cache not connected")
	}
	return c.rdb.Set(ctx, key, value, ttl).Err()
}

// Delete removes a key from the cache
func (c *Client) Delete(ctx context.Context, keys ...string) error {
	if c == nil || c.rdb == nil {
		return fmt.Errorf("cache not connected")
	}
	return c.rdb.Del(ctx, keys...).Err()
}

// Exists checks if a key exists
func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	if c == nil || c.rdb == nil {
		return false, fmt.Errorf("cache not connected")
	}
	n, err := c.rdb.Exists(ctx, key).Result()
	return n > 0, err
}

// GetJSON retrieves and unmarshals a JSON value
func (c *Client) GetJSON(ctx context.Context, key string, dest interface{}) error {
	if c == nil || c.rdb == nil {
		return fmt.Errorf("cache not connected")
	}
	data, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

// SetJSON marshals and stores a value as JSON
func (c *Client) SetJSON(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if c == nil || c.rdb == nil {
		return fmt.Errorf("cache not connected")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, key, data, ttl).Err()
}

// HGet retrieves a field from a hash
func (c *Client) HGet(ctx context.Context, key, field string) (string, error) {
	if c == nil || c.rdb == nil {
		return "", fmt.Errorf("cache not connected")
	}
	return c.rdb.HGet(ctx, key, field).Result()
}

// HSet sets a field in a hash
func (c *Client) HSet(ctx context.Context, key string, values ...interface{}) error {
	if c == nil || c.rdb == nil {
		return fmt.Errorf("cache not connected")
	}
	return c.rdb.HSet(ctx, key, values...).Err()
}

// HGetAll retrieves all fields from a hash
func (c *Client) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	if c == nil || c.rdb == nil {
		return nil, fmt.Errorf("cache not connected")
	}
	return c.rdb.HGetAll(ctx, key).Result()
}

// SAdd adds members to a set
func (c *Client) SAdd(ctx context.Context, key string, members ...interface{}) error {
	if c == nil || c.rdb == nil {
		return fmt.Errorf("cache not connected")
	}
	return c.rdb.SAdd(ctx, key, members...).Err()
}

// SMembers returns all members of a set
func (c *Client) SMembers(ctx context.Context, key string) ([]string, error) {
	if c == nil || c.rdb == nil {
		return nil, fmt.Errorf("cache not connected")
	}
	return c.rdb.SMembers(ctx, key).Result()
}

// SIsMember checks if a value is in a set
func (c *Client) SIsMember(ctx context.Context, key string, member interface{}) (bool, error) {
	if c == nil || c.rdb == nil {
		return false, fmt.Errorf("cache not connected")
	}
	return c.rdb.SIsMember(ctx, key, member).Result()
}

// Expire sets a TTL on a key
func (c *Client) Expire(ctx context.Context, key string, ttl time.Duration) error {
	if c == nil || c.rdb == nil {
		return fmt.Errorf("cache not connected")
	}
	return c.rdb.Expire(ctx, key, ttl).Err()
}

// Keys returns all keys matching a pattern (use sparingly)
func (c *Client) Keys(ctx context.Context, pattern string) ([]string, error) {
	if c == nil || c.rdb == nil {
		return nil, fmt.Errorf("cache not connected")
	}
	return c.rdb.Keys(ctx, pattern).Result()
}

// Incr increments a counter
func (c *Client) Incr(ctx context.Context, key string) (int64, error) {
	if c == nil || c.rdb == nil {
		return 0, fmt.Errorf("cache not connected")
	}
	return c.rdb.Incr(ctx, key).Result()
}

// IncrBy increments a counter by a specific amount
func (c *Client) IncrBy(ctx context.Context, key string, value int64) (int64, error) {
	if c == nil || c.rdb == nil {
		return 0, fmt.Errorf("cache not connected")
	}
	return c.rdb.IncrBy(ctx, key, value).Result()
}

// IsNil returns true if the error is a cache miss
func IsNil(err error) bool {
	return err == redis.Nil
}

// RedisClient returns the underlying Redis client for direct access
// Used by packages that need raw Redis operations (e.g., rate limiting)
func (c *Client) RedisClient() *redis.Client {
	if c == nil {
		return nil
	}
	return c.rdb
}

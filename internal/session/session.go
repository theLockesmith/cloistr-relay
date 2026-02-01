// Package session provides distributed session state management via Redis/Dragonfly
//
// This enables relay replicas to share session state for features like:
// - Cross-replica subscription sync
// - NIP-42 auth state sharing
// - Connection-level rate limiting
package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// State represents a connection's session state
type State struct {
	// SessionID is a unique identifier for this connection
	SessionID string `json:"session_id"`
	// AuthPubkey is the authenticated user's pubkey (NIP-42)
	AuthPubkey string `json:"auth_pubkey,omitempty"`
	// AuthedAt is when the session was authenticated
	AuthedAt int64 `json:"authed_at,omitempty"`
	// ConnectedAt is when the connection was established
	ConnectedAt int64 `json:"connected_at"`
	// RemoteIP is the client's IP address
	RemoteIP string `json:"remote_ip"`
	// Subscriptions tracks active REQ subscription IDs
	Subscriptions []string `json:"subscriptions,omitempty"`
	// LastActivity is the timestamp of last activity
	LastActivity int64 `json:"last_activity"`
}

// Config holds session store configuration
type Config struct {
	// Enabled activates distributed session management
	Enabled bool
	// SessionTTL is how long sessions live without activity
	SessionTTL time.Duration
	// KeyPrefix is the Redis key prefix for session data
	KeyPrefix string
}

// DefaultConfig returns sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Enabled:    true,
		SessionTTL: 30 * time.Minute,
		KeyPrefix:  "session:",
	}
}

// Store manages session state in Redis/Dragonfly
type Store struct {
	rdb    *redis.Client
	config *Config
}

// New creates a new session store
func New(rdb *redis.Client, cfg *Config) *Store {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Store{
		rdb:    rdb,
		config: cfg,
	}
}

// key generates the Redis key for a session
func (s *Store) key(sessionID string) string {
	return s.config.KeyPrefix + sessionID
}

// Create creates a new session with initial state
func (s *Store) Create(ctx context.Context, sessionID, remoteIP string) (*State, error) {
	if s.rdb == nil || !s.config.Enabled {
		return &State{
			SessionID:   sessionID,
			RemoteIP:    remoteIP,
			ConnectedAt: time.Now().Unix(),
			LastActivity: time.Now().Unix(),
		}, nil
	}

	state := &State{
		SessionID:   sessionID,
		RemoteIP:    remoteIP,
		ConnectedAt: time.Now().Unix(),
		LastActivity: time.Now().Unix(),
	}

	data, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session state: %w", err)
	}

	if err := s.rdb.Set(ctx, s.key(sessionID), data, s.config.SessionTTL).Err(); err != nil {
		log.Printf("Session store error (create): %v", err)
		// Fail open - return state without persisting
		return state, nil
	}

	return state, nil
}

// Get retrieves a session's state
func (s *Store) Get(ctx context.Context, sessionID string) (*State, error) {
	if s.rdb == nil || !s.config.Enabled {
		return nil, nil
	}

	data, err := s.rdb.Get(ctx, s.key(sessionID)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		log.Printf("Session store error (get): %v", err)
		return nil, nil // Fail open
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		log.Printf("Session unmarshal error: %v", err)
		return nil, nil
	}

	return &state, nil
}

// SetAuthPubkey updates the session with an authenticated pubkey
func (s *Store) SetAuthPubkey(ctx context.Context, sessionID, pubkey string) error {
	if s.rdb == nil || !s.config.Enabled {
		return nil
	}

	state, err := s.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	if state == nil {
		// Session doesn't exist, create minimal state
		state = &State{
			SessionID:   sessionID,
			ConnectedAt: time.Now().Unix(),
		}
	}

	state.AuthPubkey = pubkey
	state.AuthedAt = time.Now().Unix()
	state.LastActivity = time.Now().Unix()

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal session state: %w", err)
	}

	if err := s.rdb.Set(ctx, s.key(sessionID), data, s.config.SessionTTL).Err(); err != nil {
		log.Printf("Session store error (setauth): %v", err)
		return nil // Fail open
	}

	return nil
}

// GetAuthPubkey retrieves the authenticated pubkey for a session
func (s *Store) GetAuthPubkey(ctx context.Context, sessionID string) string {
	state, _ := s.Get(ctx, sessionID)
	if state == nil {
		return ""
	}
	return state.AuthPubkey
}

// AddSubscription adds a subscription ID to the session
func (s *Store) AddSubscription(ctx context.Context, sessionID, subID string) error {
	if s.rdb == nil || !s.config.Enabled {
		return nil
	}

	state, err := s.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	if state == nil {
		return nil // Session doesn't exist
	}

	// Check if subscription already exists
	for _, existing := range state.Subscriptions {
		if existing == subID {
			return nil // Already subscribed
		}
	}

	state.Subscriptions = append(state.Subscriptions, subID)
	state.LastActivity = time.Now().Unix()

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal session state: %w", err)
	}

	if err := s.rdb.Set(ctx, s.key(sessionID), data, s.config.SessionTTL).Err(); err != nil {
		log.Printf("Session store error (addsub): %v", err)
		return nil // Fail open
	}

	return nil
}

// RemoveSubscription removes a subscription ID from the session
func (s *Store) RemoveSubscription(ctx context.Context, sessionID, subID string) error {
	if s.rdb == nil || !s.config.Enabled {
		return nil
	}

	state, err := s.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	if state == nil {
		return nil // Session doesn't exist
	}

	// Remove subscription from list
	var newSubs []string
	for _, existing := range state.Subscriptions {
		if existing != subID {
			newSubs = append(newSubs, existing)
		}
	}

	state.Subscriptions = newSubs
	state.LastActivity = time.Now().Unix()

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal session state: %w", err)
	}

	if err := s.rdb.Set(ctx, s.key(sessionID), data, s.config.SessionTTL).Err(); err != nil {
		log.Printf("Session store error (remsub): %v", err)
		return nil // Fail open
	}

	return nil
}

// Delete removes a session
func (s *Store) Delete(ctx context.Context, sessionID string) error {
	if s.rdb == nil || !s.config.Enabled {
		return nil
	}

	if err := s.rdb.Del(ctx, s.key(sessionID)).Err(); err != nil {
		log.Printf("Session store error (delete): %v", err)
		return nil // Fail open
	}

	return nil
}

// Touch updates the last activity timestamp and extends TTL
func (s *Store) Touch(ctx context.Context, sessionID string) error {
	if s.rdb == nil || !s.config.Enabled {
		return nil
	}

	state, err := s.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	if state == nil {
		return nil // Session doesn't exist
	}

	state.LastActivity = time.Now().Unix()

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal session state: %w", err)
	}

	if err := s.rdb.Set(ctx, s.key(sessionID), data, s.config.SessionTTL).Err(); err != nil {
		log.Printf("Session store error (touch): %v", err)
		return nil // Fail open
	}

	return nil
}

// ListSessions returns all active session IDs (use sparingly)
func (s *Store) ListSessions(ctx context.Context) ([]string, error) {
	if s.rdb == nil || !s.config.Enabled {
		return nil, nil
	}

	keys, err := s.rdb.Keys(ctx, s.config.KeyPrefix+"*").Result()
	if err != nil {
		log.Printf("Session store error (list): %v", err)
		return nil, nil // Fail open
	}

	// Strip prefix from keys
	var sessionIDs []string
	prefixLen := len(s.config.KeyPrefix)
	for _, key := range keys {
		if len(key) > prefixLen {
			sessionIDs = append(sessionIDs, key[prefixLen:])
		}
	}

	return sessionIDs, nil
}

// CountSessions returns the number of active sessions
func (s *Store) CountSessions(ctx context.Context) (int64, error) {
	if s.rdb == nil || !s.config.Enabled {
		return 0, nil
	}

	// Use SCAN for better performance with large datasets
	var count int64
	var cursor uint64

	for {
		keys, nextCursor, err := s.rdb.Scan(ctx, cursor, s.config.KeyPrefix+"*", 100).Result()
		if err != nil {
			log.Printf("Session store error (count): %v", err)
			return 0, nil // Fail open
		}

		count += int64(len(keys))
		cursor = nextCursor

		if cursor == 0 {
			break
		}
	}

	return count, nil
}

// IsAuthenticated checks if a session has an authenticated pubkey
func (s *Store) IsAuthenticated(ctx context.Context, sessionID string) bool {
	state, _ := s.Get(ctx, sessionID)
	return state != nil && state.AuthPubkey != ""
}

// GetSessionsByPubkey returns all sessions for a given pubkey
// Useful for broadcasting to all connections of a user
func (s *Store) GetSessionsByPubkey(ctx context.Context, pubkey string) ([]string, error) {
	if s.rdb == nil || !s.config.Enabled {
		return nil, nil
	}

	// This is an expensive operation - iterate all sessions
	// In production, consider a reverse index: pubkey -> []sessionID
	sessionIDs, err := s.ListSessions(ctx)
	if err != nil {
		return nil, err
	}

	var matching []string
	for _, sid := range sessionIDs {
		state, _ := s.Get(ctx, sid)
		if state != nil && state.AuthPubkey == pubkey {
			matching = append(matching, sid)
		}
	}

	return matching, nil
}

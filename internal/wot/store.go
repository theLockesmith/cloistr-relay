package wot

import (
	"database/sql"
	"sync"
	"time"
)

// Store handles database operations for WoT follow relationships
type Store struct {
	db    *sql.DB
	cache map[string]CachedTrust
	mu    sync.RWMutex
	ttl   time.Duration
}

// NewStore creates a new WoT store
func NewStore(db *sql.DB, cacheTTL time.Duration) *Store {
	return &Store{
		db:    db,
		cache: make(map[string]CachedTrust),
		ttl:   cacheTTL,
	}
}

// Init creates the WoT tables
func (s *Store) Init() error {
	schema := `
		-- Follow relationships extracted from kind 3 events
		CREATE TABLE IF NOT EXISTS wot_follows (
			follower TEXT NOT NULL,
			followee TEXT NOT NULL,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (follower, followee)
		);

		-- Index for efficient lookups
		CREATE INDEX IF NOT EXISTS idx_wot_follows_follower ON wot_follows(follower);
		CREATE INDEX IF NOT EXISTS idx_wot_follows_followee ON wot_follows(followee);
	`
	_, err := s.db.Exec(schema)
	return err
}

// UpdateFollows replaces all follow relationships for a follower
// This is called when we see a kind 3 (contact list) event
func (s *Store) UpdateFollows(follower string, followees []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete existing follows for this follower
	_, err = tx.Exec(`DELETE FROM wot_follows WHERE follower = $1`, follower)
	if err != nil {
		return err
	}

	// Insert new follows
	for _, followee := range followees {
		_, err = tx.Exec(
			`INSERT INTO wot_follows (follower, followee) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			follower, followee,
		)
		if err != nil {
			return err
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return err
	}

	// Invalidate cache for affected pubkeys
	s.invalidateCache(follower)
	for _, followee := range followees {
		s.invalidateCache(followee)
	}

	return nil
}

// GetFollows returns all pubkeys that a given pubkey follows
func (s *Store) GetFollows(pubkey string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT followee FROM wot_follows WHERE follower = $1`,
		pubkey,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var follows []string
	for rows.Next() {
		var followee string
		if err := rows.Scan(&followee); err != nil {
			return nil, err
		}
		follows = append(follows, followee)
	}
	return follows, rows.Err()
}

// IsFollowing checks if follower follows followee
func (s *Store) IsFollowing(follower, followee string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM wot_follows WHERE follower = $1 AND followee = $2)`,
		follower, followee,
	).Scan(&exists)
	return exists, err
}

// GetFollowersOf returns all pubkeys that follow a given pubkey
func (s *Store) GetFollowersOf(pubkey string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT follower FROM wot_follows WHERE followee = $1`,
		pubkey,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var followers []string
	for rows.Next() {
		var follower string
		if err := rows.Scan(&follower); err != nil {
			return nil, err
		}
		followers = append(followers, follower)
	}
	return followers, rows.Err()
}

// GetCachedTrust returns the cached trust level for a pubkey
func (s *Store) GetCachedTrust(pubkey string) (CachedTrust, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cached, ok := s.cache[pubkey]
	if !ok {
		return CachedTrust{}, false
	}

	// Check if cache has expired
	if time.Since(cached.CachedAt) > s.ttl {
		return CachedTrust{}, false
	}

	return cached, true
}

// SetCachedTrust sets the cached trust level for a pubkey
func (s *Store) SetCachedTrust(pubkey string, level TrustLevel) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cache[pubkey] = CachedTrust{
		Pubkey:     pubkey,
		TrustLevel: level,
		CachedAt:   time.Now(),
	}
}

// invalidateCache removes a pubkey from the cache
func (s *Store) invalidateCache(pubkey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cache, pubkey)
}

// ClearCache clears the entire trust cache
func (s *Store) ClearCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache = make(map[string]CachedTrust)
}

// GetFollowCount returns the number of follows for a pubkey
func (s *Store) GetFollowCount(pubkey string) (int, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM wot_follows WHERE follower = $1`,
		pubkey,
	).Scan(&count)
	return count, err
}

// GetFollowerCount returns the number of followers for a pubkey
func (s *Store) GetFollowerCount(pubkey string) (int, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM wot_follows WHERE followee = $1`,
		pubkey,
	).Scan(&count)
	return count, err
}

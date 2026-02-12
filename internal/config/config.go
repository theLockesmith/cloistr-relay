package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all relay configuration
type Config struct {
	Port         int
	RelayName    string
	RelayURL     string
	RelayPubkey  string
	RelayContact string
	DBHost       string
	DBPort       int
	DBName       string
	DBUser       string
	DBPassword   string

	// Authentication settings
	AuthPolicy     string   // "open", "auth-read", "auth-write", "auth-all"
	AllowedPubkeys []string // Whitelist of pubkeys allowed to write (if set)

	// NIP-22 Timestamp limits (in seconds)
	MaxCreatedAtFuture int64 // Max seconds into future for created_at (default: 300 = 5 min)
	MaxCreatedAtPast   int64 // Max seconds into past for created_at (0 = unlimited, default)

	// NIP-13 Proof of Work
	MinPoWDifficulty int // Minimum PoW difficulty required (0 = disabled, default)

	// Rate Limiting
	RateLimitEventsPerSec      int  // Events per second per IP (0 = disabled)
	RateLimitFiltersPerSec     int  // Filter queries per second per IP (0 = disabled)
	RateLimitConnectionsPerSec int  // New connections per second per IP (0 = disabled)
	RateLimitDistributed       bool // Use distributed rate limiting via Redis/Dragonfly

	// NIP-86 Management API
	AdminPubkeys []string // Pubkeys authorized to use management API

	// Web of Trust (WoT) Filtering
	WoTEnabled          bool   // Enable WoT filtering
	WoTOwnerPubkey      string // Owner pubkey (trust level 0)
	WoTUnknownPoWBits   int    // PoW bits required for unknown pubkeys (default 8)
	WoTUnknownRateLimit int    // Events/sec for unknown pubkeys (default 5)
	WoTUsePageRank      bool   // Use PageRank-based trust scoring (Tier 2)
	WoTPageRankInterval int    // PageRank recompute interval in minutes (default 60)

	// Cache (Redis/Dragonfly)
	CacheURL string // Redis/Dragonfly URL (e.g., redis://dragonfly:6379)

	// NIP-59 Gift Wrap
	GiftWrapEnabled     bool // Enable NIP-59 gift wrap support
	GiftWrapRequireAuth bool // Require auth to query gift wrap events (default true)

	// NIP-57 Zaps
	ZapsEnabled         bool // Enable NIP-57 zap support
	ZapsValidateReceipt bool // Validate zap receipt structure (default true)

	// Dragonfly-powered features
	WriteAheadEnabled   bool // Enable write-ahead logging (events to Dragonfly first)
	EventCacheEnabled   bool // Enable hot event caching in Dragonfly
	SessionStoreEnabled bool // Enable distributed session state in Dragonfly

	// NIP-70 Protected Events
	ProtectedEventsEnabled bool // Enable NIP-70 protected event handling (default true)
	ProtectedEventsAllow   bool // Allow protected events from authenticated authors (default true)

	// Database connection pool tuning
	DBMaxOpenConns    int           // Max open connections (default: 25)
	DBMaxIdleConns    int           // Max idle connections (default: 10)
	DBConnMaxLifetime time.Duration // Max connection lifetime (default: 5m)
	DBConnMaxIdleTime time.Duration // Max idle time before closing (default: 1m)

	// Profiling
	PProfEnabled bool // Enable pprof endpoints at /debug/pprof/
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		Port:                       3334,
		RelayName:                  "coldforge-relay",
		RelayURL:                   "ws://localhost:3334",
		DBHost:                     "localhost",
		DBPort:                     5432,
		DBName:                     "nostr",
		DBUser:                     "postgres",
		MaxCreatedAtFuture:         300, // 5 minutes (NIP-22)
		MaxCreatedAtPast:           0,   // Unlimited by default
		RateLimitEventsPerSec:      10,  // 10 events/sec per IP
		RateLimitFiltersPerSec:     20,  // 20 queries/sec per IP
		RateLimitConnectionsPerSec: 5,   // 5 connections/sec per IP
		// Database pool defaults (tuned for typical relay workload)
		DBMaxOpenConns:    25,               // Balance between throughput and DB load
		DBMaxIdleConns:    10,               // Keep connections warm
		DBConnMaxLifetime: 5 * time.Minute,  // Prevent stale connections
		DBConnMaxIdleTime: 1 * time.Minute,  // Release idle connections
	}

	// Override from environment variables
	if port := os.Getenv("RELAY_PORT"); port != "" {
		p, err := strconv.Atoi(port)
		if err != nil {
			return nil, fmt.Errorf("invalid RELAY_PORT: %w", err)
		}
		cfg.Port = p
	}

	if name := os.Getenv("RELAY_NAME"); name != "" {
		cfg.RelayName = name
	}

	if url := os.Getenv("RELAY_URL"); url != "" {
		cfg.RelayURL = url
	}

	if pubkey := os.Getenv("RELAY_PUBKEY"); pubkey != "" {
		cfg.RelayPubkey = pubkey
	}

	if contact := os.Getenv("RELAY_CONTACT"); contact != "" {
		cfg.RelayContact = contact
	}

	if host := os.Getenv("DB_HOST"); host != "" {
		cfg.DBHost = host
	}

	if port := os.Getenv("DB_PORT"); port != "" {
		p, err := strconv.Atoi(port)
		if err != nil {
			return nil, fmt.Errorf("invalid DB_PORT: %w", err)
		}
		cfg.DBPort = p
	}

	if name := os.Getenv("DB_NAME"); name != "" {
		cfg.DBName = name
	}

	if user := os.Getenv("DB_USER"); user != "" {
		cfg.DBUser = user
	}

	if password := os.Getenv("DB_PASSWORD"); password != "" {
		cfg.DBPassword = password
	}

	// Authentication settings
	if authPolicy := os.Getenv("AUTH_POLICY"); authPolicy != "" {
		cfg.AuthPolicy = authPolicy
	}

	if allowedPubkeys := os.Getenv("ALLOWED_PUBKEYS"); allowedPubkeys != "" {
		// Parse comma-separated list of pubkeys
		cfg.AllowedPubkeys = parseCommaSeparated(allowedPubkeys)
	}

	// NIP-22 timestamp limits
	if maxFuture := os.Getenv("MAX_CREATED_AT_FUTURE"); maxFuture != "" {
		if v, err := strconv.ParseInt(maxFuture, 10, 64); err == nil {
			cfg.MaxCreatedAtFuture = v
		}
	}

	if maxPast := os.Getenv("MAX_CREATED_AT_PAST"); maxPast != "" {
		if v, err := strconv.ParseInt(maxPast, 10, 64); err == nil {
			cfg.MaxCreatedAtPast = v
		}
	}

	// NIP-13 Proof of Work
	if minPow := os.Getenv("MIN_POW_DIFFICULTY"); minPow != "" {
		if v, err := strconv.Atoi(minPow); err == nil {
			cfg.MinPoWDifficulty = v
		}
	}

	// Rate Limiting
	if eventsPerSec := os.Getenv("RATE_LIMIT_EVENTS_PER_SEC"); eventsPerSec != "" {
		if v, err := strconv.Atoi(eventsPerSec); err == nil {
			cfg.RateLimitEventsPerSec = v
		}
	}

	if filtersPerSec := os.Getenv("RATE_LIMIT_FILTERS_PER_SEC"); filtersPerSec != "" {
		if v, err := strconv.Atoi(filtersPerSec); err == nil {
			cfg.RateLimitFiltersPerSec = v
		}
	}

	if connsPerSec := os.Getenv("RATE_LIMIT_CONNECTIONS_PER_SEC"); connsPerSec != "" {
		if v, err := strconv.Atoi(connsPerSec); err == nil {
			cfg.RateLimitConnectionsPerSec = v
		}
	}

	if distributed := os.Getenv("RATE_LIMIT_DISTRIBUTED"); distributed == "true" || distributed == "1" {
		cfg.RateLimitDistributed = true
	}

	// NIP-86 Management API
	if adminPubkeys := os.Getenv("ADMIN_PUBKEYS"); adminPubkeys != "" {
		cfg.AdminPubkeys = parseCommaSeparated(adminPubkeys)
	}

	// Web of Trust (WoT) Filtering
	if wotEnabled := os.Getenv("WOT_ENABLED"); wotEnabled == "true" || wotEnabled == "1" {
		cfg.WoTEnabled = true
	}
	if wotOwner := os.Getenv("WOT_OWNER_PUBKEY"); wotOwner != "" {
		cfg.WoTOwnerPubkey = wotOwner
	}
	if wotPoW := os.Getenv("WOT_UNKNOWN_POW_BITS"); wotPoW != "" {
		if v, err := strconv.Atoi(wotPoW); err == nil {
			cfg.WoTUnknownPoWBits = v
		}
	}
	if wotRate := os.Getenv("WOT_UNKNOWN_RATE_LIMIT"); wotRate != "" {
		if v, err := strconv.Atoi(wotRate); err == nil {
			cfg.WoTUnknownRateLimit = v
		}
	}
	if wotPageRank := os.Getenv("WOT_USE_PAGERANK"); wotPageRank == "true" || wotPageRank == "1" {
		cfg.WoTUsePageRank = true
	}
	if wotPRInterval := os.Getenv("WOT_PAGERANK_INTERVAL"); wotPRInterval != "" {
		if v, err := strconv.Atoi(wotPRInterval); err == nil {
			cfg.WoTPageRankInterval = v
		}
	}

	// Cache (Redis/Dragonfly)
	if cacheURL := os.Getenv("CACHE_URL"); cacheURL != "" {
		cfg.CacheURL = cacheURL
	}

	// NIP-59 Gift Wrap (enabled by default)
	cfg.GiftWrapEnabled = true
	cfg.GiftWrapRequireAuth = true
	if gwEnabled := os.Getenv("GIFTWRAP_ENABLED"); gwEnabled == "false" || gwEnabled == "0" {
		cfg.GiftWrapEnabled = false
	}
	if gwAuth := os.Getenv("GIFTWRAP_REQUIRE_AUTH"); gwAuth == "false" || gwAuth == "0" {
		cfg.GiftWrapRequireAuth = false
	}

	// NIP-57 Zaps (enabled by default)
	cfg.ZapsEnabled = true
	cfg.ZapsValidateReceipt = true
	if zapsEnabled := os.Getenv("ZAPS_ENABLED"); zapsEnabled == "false" || zapsEnabled == "0" {
		cfg.ZapsEnabled = false
	}
	if zapsValidate := os.Getenv("ZAPS_VALIDATE_RECEIPT"); zapsValidate == "false" || zapsValidate == "0" {
		cfg.ZapsValidateReceipt = false
	}

	// Dragonfly-powered features (disabled by default, require CACHE_URL)
	if walEnabled := os.Getenv("WRITE_AHEAD_ENABLED"); walEnabled == "true" || walEnabled == "1" {
		cfg.WriteAheadEnabled = true
	}
	if cacheEnabled := os.Getenv("EVENT_CACHE_ENABLED"); cacheEnabled == "true" || cacheEnabled == "1" {
		cfg.EventCacheEnabled = true
	}
	if sessionEnabled := os.Getenv("SESSION_STORE_ENABLED"); sessionEnabled == "true" || sessionEnabled == "1" {
		cfg.SessionStoreEnabled = true
	}

	// NIP-70 Protected Events (enabled by default)
	cfg.ProtectedEventsEnabled = true
	cfg.ProtectedEventsAllow = true
	if protectedEnabled := os.Getenv("PROTECTED_EVENTS_ENABLED"); protectedEnabled == "false" || protectedEnabled == "0" {
		cfg.ProtectedEventsEnabled = false
	}
	if protectedAllow := os.Getenv("PROTECTED_EVENTS_ALLOW"); protectedAllow == "false" || protectedAllow == "0" {
		cfg.ProtectedEventsAllow = false
	}

	// Database connection pool tuning
	if maxOpen := os.Getenv("DB_MAX_OPEN_CONNS"); maxOpen != "" {
		if v, err := strconv.Atoi(maxOpen); err == nil && v > 0 {
			cfg.DBMaxOpenConns = v
		}
	}
	if maxIdle := os.Getenv("DB_MAX_IDLE_CONNS"); maxIdle != "" {
		if v, err := strconv.Atoi(maxIdle); err == nil && v > 0 {
			cfg.DBMaxIdleConns = v
		}
	}
	if maxLifetime := os.Getenv("DB_CONN_MAX_LIFETIME"); maxLifetime != "" {
		if v, err := time.ParseDuration(maxLifetime); err == nil {
			cfg.DBConnMaxLifetime = v
		}
	}
	if maxIdleTime := os.Getenv("DB_CONN_MAX_IDLE_TIME"); maxIdleTime != "" {
		if v, err := time.ParseDuration(maxIdleTime); err == nil {
			cfg.DBConnMaxIdleTime = v
		}
	}

	// Profiling (disabled by default)
	if pprof := os.Getenv("PPROF_ENABLED"); pprof == "true" || pprof == "1" {
		cfg.PProfEnabled = true
	}

	return cfg, nil
}

// parseCommaSeparated splits a comma-separated string into a slice
func parseCommaSeparated(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

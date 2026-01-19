package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		Port:               3334,
		RelayName:          "coldforge-relay",
		RelayURL:           "ws://localhost:3334",
		DBHost:             "localhost",
		DBPort:             5432,
		DBName:             "nostr",
		DBUser:             "postgres",
		MaxCreatedAtFuture: 300,  // 5 minutes (NIP-22)
		MaxCreatedAtPast:   0,    // Unlimited by default
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

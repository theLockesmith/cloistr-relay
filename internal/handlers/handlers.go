package handlers

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/fiatjaf/khatru"
	"github.com/fiatjaf/khatru/policies"
	"github.com/nbd-wtf/go-nostr"
	"gitlab.com/coldforge/coldforge-relay/internal/config"
	"gitlab.com/coldforge/coldforge-relay/internal/metrics"
)

// RegisterHandlers registers all event handlers with the relay
// Set useDistributedRateLimit to true to skip in-memory rate limiting (when using distributed rate limiter)
func RegisterHandlers(relay *khatru.Relay, cfg *config.Config, useDistributedRateLimit bool) {
	// Reject events based on custom policies
	relay.RejectEvent = append(relay.RejectEvent, rejectInvalidEvents)

	// NIP-22: Reject events with timestamps outside limits
	relay.RejectEvent = append(relay.RejectEvent, rejectTimestampOutOfRange(cfg))

	// NIP-40: Reject expired events on publish
	relay.RejectEvent = append(relay.RejectEvent, rejectExpiredEvents)

	// NIP-13: Reject events that don't meet minimum PoW difficulty
	if cfg.MinPoWDifficulty > 0 {
		relay.RejectEvent = append(relay.RejectEvent, requirePoW(cfg))
		log.Printf("NIP-13 PoW requirement: %d leading zero bits", cfg.MinPoWDifficulty)
	}

	// Spam protection: Reject events with embedded base64 media
	relay.RejectEvent = append(relay.RejectEvent, policies.RejectEventsWithBase64Media)

	// Rate limiting for events (per IP) - skip if using distributed rate limiter
	if !useDistributedRateLimit && cfg.RateLimitEventsPerSec > 0 {
		relay.RejectEvent = append(relay.RejectEvent,
			policies.EventIPRateLimiter(cfg.RateLimitEventsPerSec, time.Second, cfg.RateLimitEventsPerSec*5))
		log.Printf("Rate limit (in-memory): %d events/sec per IP", cfg.RateLimitEventsPerSec)
	}

	// Reject filters based on custom policies
	relay.RejectFilter = append(relay.RejectFilter, rejectComplexFilters)

	// Additional filter protection from khatru policies
	relay.RejectFilter = append(relay.RejectFilter, policies.NoComplexFilters)

	// Rate limiting for filters/queries (per IP) - skip if using distributed rate limiter
	if !useDistributedRateLimit && cfg.RateLimitFiltersPerSec > 0 {
		relay.RejectFilter = append(relay.RejectFilter,
			policies.FilterIPRateLimiter(cfg.RateLimitFiltersPerSec, time.Second, cfg.RateLimitFiltersPerSec*5))
		log.Printf("Rate limit (in-memory): %d filters/sec per IP", cfg.RateLimitFiltersPerSec)
	}

	// Rate limiting for new connections (per IP) - skip if using distributed rate limiter
	if !useDistributedRateLimit && cfg.RateLimitConnectionsPerSec > 0 {
		relay.RejectConnection = append(relay.RejectConnection,
			policies.ConnectionRateLimiter(cfg.RateLimitConnectionsPerSec, time.Second, cfg.RateLimitConnectionsPerSec*5))
		log.Printf("Rate limit (in-memory): %d connections/sec per IP", cfg.RateLimitConnectionsPerSec)
	}

	// Log connections
	relay.OnConnect = append(relay.OnConnect, func(ctx context.Context) {
		log.Printf("Client connected")
	})

	// Log disconnections
	relay.OnDisconnect = append(relay.OnDisconnect, func(ctx context.Context) {
		log.Printf("Client disconnected")
	})

	// NIP-46: Handle ephemeral events (kinds 20000-30000) for Nostr Connect
	relay.OnEphemeralEvent = append(relay.OnEphemeralEvent, func(ctx context.Context, event *nostr.Event) {
		// Kind 24133 is used for NIP-46 Nostr Connect messages
		if event.Kind == 24133 {
			log.Printf("NIP-46 message relayed: %s -> %s", event.PubKey[:8], getRecipient(event))
			metrics.RecordNIP46Message()
		}
	})

	log.Println("Event handlers registered")
}

// getRecipient extracts the recipient pubkey from event tags
func getRecipient(event *nostr.Event) string {
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "p" {
			if len(tag[1]) >= 8 {
				return tag[1][:8] + "..."
			}
			return tag[1]
		}
	}
	return "unknown"
}

// rejectInvalidEvents validates events and rejects invalid ones
func rejectInvalidEvents(ctx context.Context, event *nostr.Event) (reject bool, msg string) {
	// Verify event signature
	ok, err := event.CheckSignature()
	if err != nil || !ok {
		return true, "invalid: signature verification failed"
	}

	// Verify event ID matches content hash
	if event.GetID() != event.ID {
		return true, "invalid: event id mismatch"
	}

	return false, ""
}

// NIP-22: rejectTimestampOutOfRange returns a handler that rejects events with timestamps outside configured limits
func rejectTimestampOutOfRange(cfg *config.Config) func(context.Context, *nostr.Event) (bool, string) {
	return func(ctx context.Context, event *nostr.Event) (reject bool, msg string) {
		now := time.Now().Unix()
		eventTime := event.CreatedAt.Time().Unix()

		// Check future limit
		if cfg.MaxCreatedAtFuture > 0 {
			maxAllowed := now + cfg.MaxCreatedAtFuture
			if eventTime > maxAllowed {
				return true, fmt.Sprintf("invalid: created_at is too far in the future (max %d seconds)", cfg.MaxCreatedAtFuture)
			}
		}

		// Check past limit (0 = unlimited)
		if cfg.MaxCreatedAtPast > 0 {
			minAllowed := now - cfg.MaxCreatedAtPast
			if eventTime < minAllowed {
				return true, fmt.Sprintf("invalid: created_at is too far in the past (max %d seconds)", cfg.MaxCreatedAtPast)
			}
		}

		return false, ""
	}
}

// rejectComplexFilters prevents resource-intensive queries
func rejectComplexFilters(ctx context.Context, filter nostr.Filter) (reject bool, msg string) {
	// Limit the number of authors in a single filter
	if len(filter.Authors) > 100 {
		return true, "error: too many authors in filter"
	}

	// Limit the number of IDs in a single filter
	if len(filter.IDs) > 500 {
		return true, "error: too many ids in filter"
	}

	// Limit the number of kinds in a single filter
	if len(filter.Kinds) > 20 {
		return true, "error: too many kinds in filter"
	}

	return false, ""
}

// NIP-40: rejectExpiredEvents rejects events that are already expired on publish
func rejectExpiredEvents(ctx context.Context, event *nostr.Event) (reject bool, msg string) {
	if IsExpired(event) {
		return true, "invalid: event has expired"
	}
	return false, ""
}

// IsExpired checks if an event has expired based on NIP-40 expiration tag
func IsExpired(event *nostr.Event) bool {
	expiration := GetExpiration(event)
	return expiration > 0 && expiration < time.Now().Unix()
}

// GetExpiration extracts the expiration timestamp from an event's tags (NIP-40)
func GetExpiration(event *nostr.Event) int64 {
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "expiration" {
			if ts, err := strconv.ParseInt(tag[1], 10, 64); err == nil {
				return ts
			}
		}
	}
	return 0 // No expiration
}

// NIP-13: requirePoW returns a handler that rejects events not meeting minimum PoW difficulty
func requirePoW(cfg *config.Config) func(context.Context, *nostr.Event) (bool, string) {
	return func(ctx context.Context, event *nostr.Event) (reject bool, msg string) {
		difficulty := countLeadingZeroBits(event.ID)
		if difficulty < cfg.MinPoWDifficulty {
			return true, fmt.Sprintf("pow: insufficient proof of work (got %d, need %d)", difficulty, cfg.MinPoWDifficulty)
		}
		return false, ""
	}
}

// countLeadingZeroBits counts the number of leading zero bits in a hex string (event ID)
func countLeadingZeroBits(hexID string) int {
	zeroBits := 0
	for _, c := range hexID {
		// Each hex character represents 4 bits
		var nibble int
		if c >= '0' && c <= '9' {
			nibble = int(c - '0')
		} else if c >= 'a' && c <= 'f' {
			nibble = int(c-'a') + 10
		} else if c >= 'A' && c <= 'F' {
			nibble = int(c-'A') + 10
		} else {
			break // Invalid character
		}

		if nibble == 0 {
			zeroBits += 4
		} else {
			// Count leading zeros in this nibble
			if nibble < 2 {
				zeroBits += 3
			} else if nibble < 4 {
				zeroBits += 2
			} else if nibble < 8 {
				zeroBits += 1
			}
			break // Found a 1 bit
		}
	}
	return zeroBits
}

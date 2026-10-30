package main

import (
	"log"
	"time"

	"github.com/fiatjaf/khatru"
	"github.com/redis/go-redis/v9"

	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/auth"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/config"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/handlers"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/management"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/ratelimit"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/storage"
)

// registerWriteGates registers the first per-event checks. khatru runs
// RejectEvent handlers in REGISTRATION ORDER and stops at the first rejection,
// so the order is the cost model:
//
//  1. Validation and the per-IP limiter (internal/handlers) -- in memory.
//  2. The distributed per-IP limiter, when enabled -- Redis, not Postgres, and
//     it must stay ahead of auth so an unauthenticated flood is still capped.
//  3. NIP-42 auth.
//  4. The tombstone check -- a Postgres lookup.
//  5. NIP-86 bans and kind restrictions -- in memory (management/access_cache.go).
//
// Auth sits ahead of every database-backed check. Until 2026-10-29 the
// tombstone lookup and five management queries ran BEFORE auth, so each of the
// ~31,000 events a day refused only for "auth-required" first paid six
// sequential Postgres round trips. Production logs showed those refusals taking
// up to 2.6s, and they were most of what tripped RelayHighEventLatency.
//
// Auth must also stay ahead of the WoT trust gate, which main registers later:
// an unauthenticated client told "pow: ..." instead of "auth-required" never
// authenticates, so WoT's authed-author exemption never fires (stash hung on
// "Connecting to your account…" for exactly that reason).
//
// Checks that main registers after this -- arbiter claims, WoT, zaps, groups,
// HAVEN -- run after auth too.
func registerWriteGates(
	r *khatru.Relay,
	cfg *config.Config,
	authCfg *auth.Config,
	tombstone storage.TombstoneLookup,
	mgmtStore *management.Store,
	redisClient *redis.Client,
) {
	// A nil mgmtStore must stay a nil interface, not a non-nil interface wrapping
	// a nil pointer, so build the interface value explicitly.
	useDistributedRateLimit := redisClient != nil && cfg.RateLimitDistributed
	var exemptChecker handlers.RateLimitExemptChecker
	if mgmtStore != nil {
		exemptChecker = mgmtStore
	}
	handlers.RegisterHandlers(r, cfg, useDistributedRateLimit, exemptChecker)

	if useDistributedRateLimit {
		rateLimitCfg := &ratelimit.Config{
			Enabled:              true,
			EventsPerSecond:      cfg.RateLimitEventsPerSec,
			FiltersPerSecond:     cfg.RateLimitFiltersPerSec,
			ConnectionsPerSecond: cfg.RateLimitConnectionsPerSec,
			BurstMultiplier:      5,
			WindowSize:           time.Second,
		}
		ratelimit.RegisterHandlers(r, redisClient, rateLimitCfg)
		log.Println("Distributed rate limiting enabled (Dragonfly/Redis)")
	}

	auth.RegisterAuthHandlers(r, authCfg)

	r.RejectEvent = append(r.RejectEvent, storage.RejectRepublishedDeletionEvent(tombstone))

	if mgmtStore != nil {
		management.RegisterBanHandlers(r, mgmtStore)
	}
}

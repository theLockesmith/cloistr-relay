package wot

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"
)

// TrustCalculator calculates trust levels for pubkeys
type TrustCalculator struct {
	store      *Store
	trustRoots map[string]struct{} // set of pubkeys at trust level 0
	maxDepth   int
}

// NewTrustCalculator creates a new trust calculator.
// trustRoots are the pubkeys that anchor the graph (trust level 0).
// For a single-owner relay, pass one; for multi-tenant, pass several.
func NewTrustCalculator(store *Store, trustRoots []string, maxDepth int) *TrustCalculator {
	if maxDepth <= 0 {
		maxDepth = 2 // Default to 2 levels (follows and follows-of-follows)
	}
	roots := make(map[string]struct{}, len(trustRoots))
	for _, r := range trustRoots {
		if r != "" {
			roots[r] = struct{}{}
		}
	}
	return &TrustCalculator{
		store:      store,
		trustRoots: roots,
		maxDepth:   maxDepth,
	}
}

// GetTrustLevel calculates the trust level for a pubkey
// Uses BFS to find the shortest path from owner to the pubkey through the follow graph
func (tc *TrustCalculator) GetTrustLevel(pubkey string) TrustLevel {
	if tc.store == nil {
		// No store: can still resolve root membership, nothing else.
		return tc.calculateTrustLevel(pubkey)
	}

	// Check cache first
	if cached, ok := tc.store.GetCachedTrust(pubkey); ok {
		return cached.TrustLevel
	}

	// Calculate trust level
	level := tc.calculateTrustLevel(pubkey)

	// Cache the result
	tc.store.SetCachedTrust(pubkey, level)

	return level
}

// calculateTrustLevel performs BFS from all trust roots to find trust level
func (tc *TrustCalculator) calculateTrustLevel(pubkey string) TrustLevel {
	// Any trust root is owner-level
	if _, isRoot := tc.trustRoots[pubkey]; isRoot {
		return TrustLevelOwner
	}

	// Check if directly followed by any trust root (trust level 1)
	for root := range tc.trustRoots {
		isFollowed, err := tc.store.IsFollowing(root, pubkey)
		if err != nil {
			log.Printf("WoT: error checking follow status for root %s: %v", truncatePubkey(root), err)
			continue
		}
		if isFollowed {
			return TrustLevelFollow
		}
	}

	// For depth 2, check if followed by anyone any root follows
	if tc.maxDepth >= 2 {
		for root := range tc.trustRoots {
			rootFollows, err := tc.store.GetFollows(root)
			if err != nil {
				log.Printf("WoT: error getting follows for root %s: %v", truncatePubkey(root), err)
				continue
			}

			for _, follow := range rootFollows {
				isFollowedByFollow, err := tc.store.IsFollowing(follow, pubkey)
				if err != nil {
					continue
				}
				if isFollowedByFollow {
					return TrustLevelFollowOfFollow
				}
			}
		}
	}

	// Not found in follow graph from any root
	return TrustLevelUnknown
}

// Handler holds the WoT configuration and provides relay handlers
type Handler struct {
	store          *Store
	calculator     *TrustCalculator
	pagerank       *PageRankCalculator
	usePageRank    bool
	policies       map[TrustLevel]TrustPolicy
	allowedPubkeys map[string]struct{} // Fast lookup for whitelisted pubkeys
	isPaidMember   func(string) bool   // nil when no membership integration
	rateBuckets    sync.Map            // pubkey (or collabBucketPrefix+pubkey) -> *rateBucket
	collabKinds    map[int]struct{}    // kinds that draw from the collab bucket
	collabRate     int                 // collab bucket floor rate; 0 = no separate bucket
}

// collabBucketPrefix keys a pubkey's collab bucket apart from its general
// bucket in rateBuckets, so the stale-bucket sweep covers both.
const collabBucketPrefix = "collab:"

// rateBucket is a token-bucket rate limiter for one pubkey. Tokens refill at
// `limit` per second up to a burst of 2× limit (two seconds' worth). A
// publish attempt costs one token; when the bucket is empty the event is
// rejected with "rate-limited".
type rateBucket struct {
	mu       sync.Mutex
	tokens   float64
	lastTime time.Time
	limit    float64
}

func (b *rateBucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(b.lastTime).Seconds()
	b.lastTime = now
	b.tokens += elapsed * b.limit
	burst := b.limit * 2 // two seconds' worth
	if b.tokens > burst {
		b.tokens = burst
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (h *Handler) getOrCreateBucket(key string, eventsPerSec int) *rateBucket {
	if v, ok := h.rateBuckets.Load(key); ok {
		return v.(*rateBucket)
	}
	b := &rateBucket{
		tokens:   float64(eventsPerSec), // start full
		lastTime: time.Now(),
		limit:    float64(eventsPerSec),
	}
	actual, _ := h.rateBuckets.LoadOrStore(key, b)
	return actual.(*rateBucket)
}

// sweepStaleBuckets removes rate-limit buckets that haven't been touched in
// maxIdle. Runs as a background goroutine started by RegisterHandlers.
func (h *Handler) sweepStaleBuckets(interval, maxIdle time.Duration) {
	for {
		time.Sleep(interval)
		cutoff := time.Now().Add(-maxIdle)
		h.rateBuckets.Range(func(key, value any) bool {
			b := value.(*rateBucket)
			b.mu.Lock()
			idle := b.lastTime.Before(cutoff)
			b.mu.Unlock()
			if idle {
				h.rateBuckets.Delete(key)
			}
			return true
		})
	}
}

// NewHandler creates a new WoT handler.
// trustRoots are the pubkeys at trust level 0 (see Config.TrustRoots).
func NewHandler(store *Store, trustRoots []string, cfg *Config) *Handler {
	policies := cfg.Policies
	if policies == nil {
		policies = DefaultPolicies()
	}

	// Build fast lookup map for allowed pubkeys
	allowedMap := make(map[string]struct{})
	for _, pk := range cfg.AllowedPubkeys {
		allowedMap[pk] = struct{}{}
	}

	collabKinds := make(map[int]struct{}, len(cfg.CollabKinds))
	for _, k := range cfg.CollabKinds {
		collabKinds[k] = struct{}{}
	}

	h := &Handler{
		store:          store,
		calculator:     NewTrustCalculator(store, trustRoots, cfg.MaxFollowDepth),
		usePageRank:    cfg.UsePageRank,
		policies:       policies,
		allowedPubkeys: allowedMap,
		isPaidMember:   cfg.IsPaidMember,
		collabKinds:    collabKinds,
		collabRate:     cfg.CollabEventsPerSecond,
	}

	// Initialize PageRank if enabled
	if cfg.UsePageRank {
		prInterval := cfg.PageRankInterval
		if prInterval == 0 {
			prInterval = 1 * time.Hour
		}
		prCfg := DefaultPageRankConfig()
		prCfg.ComputeInterval = prInterval
		h.pagerank = NewPageRankCalculator(store, trustRoots, prCfg)
	}

	return h
}

// RejectEventByTrust returns a handler that rejects events based on WoT trust level
func (h *Handler) RejectEventByTrust() func(context.Context, *nostr.Event) (bool, string) {
	return func(ctx context.Context, event *nostr.Event) (bool, string) {
		// NIP-46 Nostr Connect events (kind 24133) are exempt from all trust gates.
		if event.Kind == 24133 {
			return false, ""
		}

		// Allowed pubkeys bypass all WoT requirements (rate, PoW, trust level).
		if _, allowed := h.allowedPubkeys[event.PubKey]; allowed {
			return false, ""
		}

		level := h.getTrustLevel(event.PubKey)
		policy := h.policies[level]

		// Per-pubkey rate limiting based on trust level. Applies to ALL writers
		// including authenticated ones: the auth bypass below is specifically
		// for PoW, not rate limits. Per-pubkey complements the per-IP limiter
		// in handlers.go — one caps an address, the other caps an identity.
		//
		// Live-collaboration kinds draw from a separate bucket. An editor
		// publishes one sync event per keystroke, so with one shared bucket a
		// new (unknown-trust) user typing at an ordinary pace drains it and
		// the document's snapshot save is refused. Splitting them means typing
		// can never starve the save. Collab events never touch the general
		// bucket; that is safe because the default collab kinds are ephemeral
		// (never stored), so the general bucket still caps everything stored.
		//
		// EventsPerSecond == 0 means unlimited on both paths.
		if _, isCollab := h.collabKinds[event.Kind]; isCollab && h.collabRate > 0 {
			if policy.EventsPerSecond > 0 {
				rate := max(policy.EventsPerSecond, h.collabRate)
				bucket := h.getOrCreateBucket(collabBucketPrefix+event.PubKey, rate)
				if !bucket.allow() {
					return true, fmt.Sprintf("rate-limited: trust level %s allows %d collaboration events/sec",
						level, rate)
				}
			}
		} else if policy.EventsPerSecond > 0 {
			bucket := h.getOrCreateBucket(event.PubKey, policy.EventsPerSecond)
			if !bucket.allow() {
				return true, fmt.Sprintf("rate-limited: trust level %s allows %d events/sec",
					level, policy.EventsPerSecond)
			}
		}

		// An author who has AUTHENTICATED (NIP-42) as this very pubkey is not
		// unknown for PoW purposes. PoW exists to price anonymous spam; it
		// buys nothing against someone who has already proven they hold the
		// key. The relay runs AUTH_POLICY=auth-write so every write is
		// authenticated anyway.
		//
		// This bypass is for PoW only. Rate limiting (above) still applies.
		if authorIsAuthenticated(khatru.GetAuthed(ctx), event.PubKey) {
			return false, ""
		}

		// Check PoW requirement. The rejection carries the required difficulty
		// so a client can mine to the exact target and retry (NIP-13 nonce).
		// Format matches the global gate (handlers.go) so clients parse one
		// shape: "pow: ... (got N, need M)".
		if policy.RequirePoW && policy.MinPoWDifficulty > 0 {
			difficulty := countLeadingZeroBits(event.ID)
			if difficulty < policy.MinPoWDifficulty {
				return true, fmt.Sprintf("pow: low trust requires proof of work (got %d, need %d)",
					difficulty, policy.MinPoWDifficulty)
			}
		}

		return false, ""
	}
}

// OnEventSaved returns a handler that extracts follow relationships from kind 3 events
func (h *Handler) OnEventSaved() func(context.Context, *nostr.Event) {
	return func(ctx context.Context, event *nostr.Event) {
		// Kind 3 is the contact list (follow list)
		if event.Kind != 3 {
			return
		}

		// Extract all 'p' tags (followed pubkeys)
		var followees []string
		for _, tag := range event.Tags {
			if len(tag) >= 2 && tag[0] == "p" {
				followees = append(followees, tag[1])
			}
		}

		// Update follow relationships
		if len(followees) > 0 {
			if err := h.store.UpdateFollows(event.PubKey, followees); err != nil {
				log.Printf("WoT: error updating follows for %s: %v", event.PubKey[:8], err)
			} else {
				log.Printf("WoT: updated %d follows for %s", len(followees), event.PubKey[:8])
			}
		}
	}
}

// GetTrustContext returns the trust level for a pubkey (useful for logging/metrics)
func (h *Handler) GetTrustContext(pubkey string) TrustLevel {
	return h.getTrustLevel(pubkey)
}

// getTrustLevel returns the trust level, checking paid status first, then
// dispatching to PageRank or simple follow distance.
//
// Paid membership is an independent axis that short-circuits the graph:
// "Even if someone is 100 steps removed from me, if they pay for the relay,
// they should absolutely not be limited anymore." On lapse the graph takes
// over with no grace period — GetEffectiveTier returns Free the moment
// tier_expires_at passes.
func (h *Handler) getTrustLevel(pubkey string) TrustLevel {
	// Trust roots are always owner-level, regardless of paid status.
	// Checked here so a root that also holds a paid tier is not demoted
	// from Owner (100 ev/s) to Paid (50 ev/s).
	if _, isRoot := h.calculator.trustRoots[pubkey]; isRoot {
		return TrustLevelOwner
	}

	// Paid members short-circuit the graph entirely
	if h.isPaidMember != nil && h.isPaidMember(pubkey) {
		return TrustLevelPaid
	}
	if h.usePageRank && h.pagerank != nil {
		return h.pagerank.GetTrustLevelFromPageRank(pubkey)
	}
	return h.calculator.GetTrustLevel(pubkey)
}

// RegisterHandlers registers WoT handlers with the relay
func RegisterHandlers(relay *khatru.Relay, store *Store, cfg *Config) *Handler {
	// Build trust roots: prefer explicit list, fall back to single owner pubkey
	trustRoots := cfg.TrustRoots
	if len(trustRoots) == 0 && cfg.OwnerPubkey != "" {
		trustRoots = []string{cfg.OwnerPubkey}
	}

	handler := NewHandler(store, trustRoots, cfg)

	// Add trust-based event rejection
	relay.RejectEvent = append(relay.RejectEvent, handler.RejectEventByTrust())

	// Add handler to extract follow relationships from kind 3 events
	relay.OnEventSaved = append(relay.OnEventSaved, handler.OnEventSaved())

	// Start PageRank background computation if enabled
	if handler.pagerank != nil {
		handler.pagerank.Start(context.Background())
		log.Printf("WoT PageRank enabled (recompute every %v)", cfg.PageRankInterval)
	}

	// Log trust root configuration
	if len(trustRoots) == 1 {
		log.Printf("WoT filtering enabled for owner %s (mode: %s)", truncatePubkey(trustRoots[0]), handler.getMode())
	} else {
		log.Printf("WoT filtering enabled with %d trust roots (mode: %s)", len(trustRoots), handler.getMode())
	}
	if len(cfg.AllowedPubkeys) > 0 {
		log.Printf("WoT allowed pubkeys (bypass PoW): %d pubkeys configured", len(cfg.AllowedPubkeys))
	}
	if cfg.IsPaidMember != nil {
		log.Printf("WoT paid-member bypass enabled (independent trust axis)")
	}
	log.Printf("WoT policies: paid=%d/s, owner=%d/s, follow=%d/s, follow2=%d/s, unknown=%d/s (PoW: %d bits)",
		handler.policies[TrustLevelPaid].EventsPerSecond,
		handler.policies[TrustLevelOwner].EventsPerSecond,
		handler.policies[TrustLevelFollow].EventsPerSecond,
		handler.policies[TrustLevelFollowOfFollow].EventsPerSecond,
		handler.policies[TrustLevelUnknown].EventsPerSecond,
		handler.policies[TrustLevelUnknown].MinPoWDifficulty,
	)
	if handler.collabRate > 0 && len(handler.collabKinds) > 0 {
		log.Printf("WoT collab bucket: kinds %v at >= %d/s per pubkey, separate from the general bucket",
			cfg.CollabKinds, handler.collabRate)
	}
	log.Println("WoT per-pubkey rate limiting active (complements per-IP limiter in handlers)")

	// Sweep stale rate-limit buckets every 5 minutes; discard any that have
	// been idle for 10 minutes. This keeps memory bounded without affecting
	// active publishers.
	go handler.sweepStaleBuckets(5*time.Minute, 10*time.Minute)

	return handler
}

// getMode returns a string describing the current WoT mode
func (h *Handler) getMode() string {
	if h.usePageRank {
		return "pagerank"
	}
	return "follow-distance"
}

// Stop stops the PageRank background computation
func (h *Handler) Stop() {
	if h.pagerank != nil {
		h.pagerank.Stop()
	}
}

// countLeadingZeroBits counts the number of leading zero bits in a hex string (event ID)
func countLeadingZeroBits(hexID string) int {
	zeroBits := 0
	for _, c := range hexID {
		var nibble int
		if c >= '0' && c <= '9' {
			nibble = int(c - '0')
		} else if c >= 'a' && c <= 'f' {
			nibble = int(c-'a') + 10
		} else if c >= 'A' && c <= 'F' {
			nibble = int(c-'A') + 10
		} else {
			break
		}

		if nibble == 0 {
			zeroBits += 4
		} else {
			if nibble < 2 {
				zeroBits += 3
			} else if nibble < 4 {
				zeroBits += 2
			} else if nibble < 8 {
				zeroBits += 1
			}
			break
		}
	}
	return zeroBits
}

// authorIsAuthenticated reports whether the connection has proven ownership of
// the key that signed this event.
//
// Deliberately requires an EXACT match rather than "any authenticated
// connection". A client authenticated as one pubkey must not be able to relay
// unlimited unmined events on behalf of every other pubkey — that would turn
// one account into an open spam relay.
//
// Pure so it can be tested: khatru.GetAuthed reads the authenticated key off an
// unexported WebSocket context key, which cannot be constructed from a test.
func authorIsAuthenticated(authedPubkey, eventPubkey string) bool {
	return authedPubkey != "" && authedPubkey == eventPubkey
}

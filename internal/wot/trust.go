package wot

import (
	"context"
	"log"
	"time"

	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"
)

// TrustCalculator calculates trust levels for pubkeys
type TrustCalculator struct {
	store       *Store
	ownerPubkey string
	maxDepth    int
}

// NewTrustCalculator creates a new trust calculator
func NewTrustCalculator(store *Store, ownerPubkey string, maxDepth int) *TrustCalculator {
	if maxDepth <= 0 {
		maxDepth = 2 // Default to 2 levels (follows and follows-of-follows)
	}
	return &TrustCalculator{
		store:       store,
		ownerPubkey: ownerPubkey,
		maxDepth:    maxDepth,
	}
}

// GetTrustLevel calculates the trust level for a pubkey
// Uses BFS to find the shortest path from owner to the pubkey through the follow graph
func (tc *TrustCalculator) GetTrustLevel(pubkey string) TrustLevel {
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

// calculateTrustLevel performs BFS to find trust level
func (tc *TrustCalculator) calculateTrustLevel(pubkey string) TrustLevel {
	// Owner is always trust level 0
	if pubkey == tc.ownerPubkey {
		return TrustLevelOwner
	}

	// Check if directly followed by owner (trust level 1)
	isFollowed, err := tc.store.IsFollowing(tc.ownerPubkey, pubkey)
	if err != nil {
		log.Printf("WoT: error checking follow status: %v", err)
		return TrustLevelUnknown
	}
	if isFollowed {
		return TrustLevelFollow
	}

	// For depth 2, check if followed by anyone the owner follows
	if tc.maxDepth >= 2 {
		ownerFollows, err := tc.store.GetFollows(tc.ownerPubkey)
		if err != nil {
			log.Printf("WoT: error getting owner follows: %v", err)
			return TrustLevelUnknown
		}

		for _, follow := range ownerFollows {
			isFollowedByFollow, err := tc.store.IsFollowing(follow, pubkey)
			if err != nil {
				continue
			}
			if isFollowedByFollow {
				return TrustLevelFollowOfFollow
			}
		}
	}

	// Not found in follow graph
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
}

// NewHandler creates a new WoT handler
func NewHandler(store *Store, ownerPubkey string, cfg *Config) *Handler {
	policies := cfg.Policies
	if policies == nil {
		policies = DefaultPolicies()
	}

	// Build fast lookup map for allowed pubkeys
	allowedMap := make(map[string]struct{})
	for _, pk := range cfg.AllowedPubkeys {
		allowedMap[pk] = struct{}{}
	}

	h := &Handler{
		store:          store,
		calculator:     NewTrustCalculator(store, ownerPubkey, cfg.MaxFollowDepth),
		usePageRank:    cfg.UsePageRank,
		policies:       policies,
		allowedPubkeys: allowedMap,
	}

	// Initialize PageRank if enabled
	if cfg.UsePageRank {
		prInterval := cfg.PageRankInterval
		if prInterval == 0 {
			prInterval = 1 * time.Hour
		}
		prCfg := DefaultPageRankConfig()
		prCfg.ComputeInterval = prInterval
		h.pagerank = NewPageRankCalculator(store, ownerPubkey, prCfg)
	}

	return h
}

// RejectEventByTrust returns a handler that rejects events based on WoT trust level
func (h *Handler) RejectEventByTrust() func(context.Context, *nostr.Event) (bool, string) {
	return func(ctx context.Context, event *nostr.Event) (bool, string) {
		// NIP-46 Nostr Connect events (kind 24133) are exempt from POW requirements
		// These are ephemeral events used for remote signer communication
		if event.Kind == 24133 {
			return false, ""
		}

		// Allowed pubkeys bypass all WoT requirements
		if _, allowed := h.allowedPubkeys[event.PubKey]; allowed {
			return false, ""
		}

		level := h.getTrustLevel(event.PubKey)
		policy := h.policies[level]

		// Check PoW requirement
		if policy.RequirePoW && policy.MinPoWDifficulty > 0 {
			difficulty := countLeadingZeroBits(event.ID)
			if difficulty < policy.MinPoWDifficulty {
				return true, "pow: low trust requires proof of work"
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

// getTrustLevel returns the trust level using either PageRank or simple follow distance
func (h *Handler) getTrustLevel(pubkey string) TrustLevel {
	if h.usePageRank && h.pagerank != nil {
		return h.pagerank.GetTrustLevelFromPageRank(pubkey)
	}
	return h.calculator.GetTrustLevel(pubkey)
}

// RegisterHandlers registers WoT handlers with the relay
func RegisterHandlers(relay *khatru.Relay, store *Store, cfg *Config) *Handler {
	handler := NewHandler(store, cfg.OwnerPubkey, cfg)

	// Add trust-based event rejection
	relay.RejectEvent = append(relay.RejectEvent, handler.RejectEventByTrust())

	// Add handler to extract follow relationships from kind 3 events
	relay.OnEventSaved = append(relay.OnEventSaved, handler.OnEventSaved())

	// Start PageRank background computation if enabled
	if handler.pagerank != nil {
		handler.pagerank.Start(context.Background())
		log.Printf("WoT PageRank enabled (recompute every %v)", cfg.PageRankInterval)
	}

	log.Printf("WoT filtering enabled for owner %s (mode: %s)", cfg.OwnerPubkey[:8], handler.getMode())
	if len(cfg.AllowedPubkeys) > 0 {
		log.Printf("WoT allowed pubkeys (bypass PoW): %d pubkeys configured", len(cfg.AllowedPubkeys))
	}
	log.Printf("WoT policies: owner=%d/s, follow=%d/s, follow2=%d/s, unknown=%d/s (PoW: %d bits)",
		handler.policies[TrustLevelOwner].EventsPerSecond,
		handler.policies[TrustLevelFollow].EventsPerSecond,
		handler.policies[TrustLevelFollowOfFollow].EventsPerSecond,
		handler.policies[TrustLevelUnknown].EventsPerSecond,
		handler.policies[TrustLevelUnknown].MinPoWDifficulty,
	)

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

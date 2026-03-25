package haven

import (
	"context"
	"fmt"
	"log"

	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"
)

// Handler manages HAVEN box routing for events and queries
type Handler struct {
	router      *Router
	config      *Config
	metrics     *Metrics
	memberStore MemberStore
}

// truncateID safely truncates an ID/pubkey for logging
func truncateID(s string) string {
	if len(s) >= 8 {
		return s[:8]
	}
	if s == "" {
		return "<empty>"
	}
	return s
}

// NewHandler creates a new HAVEN handler
func NewHandler(cfg *Config) *Handler {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Handler{
		router:  NewRouter(cfg),
		config:  cfg,
		metrics: GetMetrics(),
	}
}

// SetEventLookup sets the event lookup interface for e-tag routing.
// This enables routing reactions/reposts to inbox when they reference owner's events.
func (h *Handler) SetEventLookup(lookup EventLookup) {
	h.router.SetEventLookup(lookup)
}

// SetMemberStore sets the member store for tier-based routing.
// This enables per-user HAVEN boxes based on membership tiers.
func (h *Handler) SetMemberStore(store MemberStore) {
	h.memberStore = store
	h.router.SetMemberStore(store)
}

// GetMemberInfo returns tier information for a pubkey if MemberStore is configured.
// Returns nil if no MemberStore is set or the pubkey is not a member.
func (h *Handler) GetMemberInfo(ctx context.Context, pubkey string) (*MemberInfo, error) {
	if h.memberStore == nil {
		return nil, nil
	}
	return h.memberStore.GetMemberInfo(ctx, pubkey)
}

// RejectEvent validates events against box access policies
// Returns (reject, reason) - if reject is true, event is rejected
func (h *Handler) RejectEvent() func(context.Context, *nostr.Event) (bool, string) {
	return func(ctx context.Context, event *nostr.Event) (bool, string) {
		if !h.config.Enabled {
			return false, ""
		}

		// Determine which box this event belongs to
		box := h.router.RouteEvent(event)

		// Log routing decision
		log.Printf("HAVEN: event %s (kind %d from %s) -> %s",
			truncateID(event.ID), event.Kind, truncateID(event.PubKey), box)

		// If box is unknown and we're in strict mode, reject
		if box == BoxUnknown {
			// Allow chat kinds from anyone (they're filtered by WoT)
			if h.router.chatKinds[event.Kind] {
				box = BoxChat
			} else {
				h.metrics.RecordEventRejected(BoxUnknown, "no_box")
				return true, "restricted: event does not belong to any HAVEN box"
			}
		}

		// Get authenticated pubkey
		authedPubkey := khatru.GetAuthed(ctx)

		// Record access attempt
		h.metrics.RecordAccessAttempt(box, "write")

		// Check write access
		if !h.router.CanAccessBox(box, authedPubkey, true) {
			switch box {
			case BoxPrivate:
				if authedPubkey == "" {
					h.metrics.RecordAccessDenied(box, "write", "auth_required")
					h.metrics.RecordEventRejected(box, "auth_required")
					return true, "auth-required: authentication required for private box"
				}
				h.metrics.RecordAccessDenied(box, "write", "not_owner")
				h.metrics.RecordEventRejected(box, "not_owner")
				return true, "restricted: only owner can write to private box"
			case BoxChat:
				if authedPubkey == "" {
					h.metrics.RecordAccessDenied(box, "write", "auth_required")
					h.metrics.RecordEventRejected(box, "auth_required")
					return true, "auth-required: authentication required for chat"
				}
				// Allowed (WoT filtering happens separately)
			case BoxInbox:
				// Public write allowed by default
			case BoxOutbox:
				if authedPubkey != h.config.OwnerPubkey {
					h.metrics.RecordAccessDenied(box, "write", "not_owner")
					h.metrics.RecordEventRejected(box, "not_owner")
					return true, "restricted: only owner can write to outbox"
				}
			}
		}

		// Verify event author matches authenticated pubkey for write operations
		if authedPubkey != "" && event.PubKey != authedPubkey {
			// This is handled by khatru, but double-check
			h.metrics.RecordEventRejected(box, "pubkey_mismatch")
			return true, "restricted: can only publish events as yourself"
		}

		// Record successful routing
		h.metrics.RecordEventRouted(box)

		return false, ""
	}
}

// RejectFilter validates filter access against box policies
func (h *Handler) RejectFilter() func(context.Context, nostr.Filter) (bool, string) {
	return func(ctx context.Context, filter nostr.Filter) (bool, string) {
		if !h.config.Enabled {
			return false, ""
		}

		authedPubkey := khatru.GetAuthed(ctx)

		// Determine which boxes this filter targets
		boxes := h.router.RouteFilter(filter, authedPubkey)

		// Check read access for each targeted box
		for _, box := range boxes {
			// Record access attempt
			h.metrics.RecordAccessAttempt(box, "read")

			if !h.router.CanAccessBox(box, authedPubkey, false) {
				switch box {
				case BoxPrivate:
					if authedPubkey == "" {
						h.metrics.RecordAccessDenied(box, "read", "auth_required")
						h.metrics.RecordFilterRejected(box, "auth_required")
						return true, "auth-required: authentication required for private box"
					}
					h.metrics.RecordAccessDenied(box, "read", "not_owner")
					h.metrics.RecordFilterRejected(box, "not_owner")
					return true, "restricted: only owner can read private box"
				case BoxChat:
					if authedPubkey == "" {
						h.metrics.RecordAccessDenied(box, "read", "auth_required")
						h.metrics.RecordFilterRejected(box, "auth_required")
						return true, "auth-required: authentication required for chat"
					}
					// Chat access is WoT filtered, allow for now
				case BoxInbox:
					if authedPubkey != h.config.OwnerPubkey {
						h.metrics.RecordAccessDenied(box, "read", "not_owner")
						h.metrics.RecordFilterRejected(box, "not_owner")
						// Use auth-required: prefix so khatru sends NIP-42 AUTH challenge
						return true, "auth-required: only owner can read inbox"
					}
				case BoxOutbox:
					// Public read allowed
				}
			}

			// Record successful filter routing
			h.metrics.RecordFilterRouted(box)
		}

		return false, ""
	}
}

// OverwriteFilter modifies filters to enforce box boundaries
func (h *Handler) OverwriteFilter() func(context.Context, *nostr.Filter) {
	return func(ctx context.Context, filter *nostr.Filter) {
		if !h.config.Enabled {
			return
		}

		authedPubkey := khatru.GetAuthed(ctx)

		// For non-owner, restrict to outbox (owner's public posts)
		if authedPubkey != h.config.OwnerPubkey {
			// If querying by author, ensure it's only the owner for outbox
			// (inbox/chat access already rejected by RejectFilter)

			// Remove private kinds from filter
			if len(filter.Kinds) > 0 {
				allowed := make([]int, 0, len(filter.Kinds))
				for _, kind := range filter.Kinds {
					if !h.router.privateKinds[kind] {
						allowed = append(allowed, kind)
					}
				}
				filter.Kinds = allowed
			}
		}
	}
}

// OnEventSaved logs box routing for monitoring
func (h *Handler) OnEventSaved() func(context.Context, *nostr.Event) {
	return func(ctx context.Context, event *nostr.Event) {
		if !h.config.Enabled {
			return
		}

		box := h.router.RouteEvent(event)
		log.Printf("HAVEN: stored event %s in %s box", truncateID(event.ID), box)
	}
}

// RegisterHandlers registers HAVEN handlers with the relay
func RegisterHandlers(relay *khatru.Relay, cfg *Config) *Handler {
	metrics := GetMetrics()

	if cfg == nil || !cfg.Enabled {
		log.Println("HAVEN: disabled")
		metrics.SetHavenEnabled(false)
		return nil
	}

	handler := NewHandler(cfg)

	// Register event rejection handler
	relay.RejectEvent = append(relay.RejectEvent, handler.RejectEvent())

	// Register filter rejection handler
	relay.RejectFilter = append(relay.RejectFilter, handler.RejectFilter())

	// Register filter overwrite handler
	relay.OverwriteFilter = append(relay.OverwriteFilter, handler.OverwriteFilter())

	// Register event saved handler for logging
	relay.OnEventSaved = append(relay.OnEventSaved, handler.OnEventSaved())

	// Set metrics
	metrics.SetHavenEnabled(true)

	log.Printf("HAVEN: enabled for owner %s", truncateID(cfg.OwnerPubkey))
	log.Printf("HAVEN: boxes - private (owner), chat (WoT), inbox (public write), outbox (public read)")

	return handler
}

// HavenSystem holds all HAVEN components
type HavenSystem struct {
	Handler    *Handler
	Blastr     *Blastr
	Importer   *Importer
	Subscriber *Subscriber
}

// RegisterFullSystem registers HAVEN handlers plus Blastr and Importer
// storeFunc is used by the Importer to store fetched events
func RegisterFullSystem(relay *khatru.Relay, cfg *Config, storeFunc func(context.Context, *nostr.Event) error) *HavenSystem {
	metrics := GetMetrics()

	if cfg == nil || !cfg.Enabled {
		log.Println("HAVEN: disabled")
		metrics.SetHavenEnabled(false)
		metrics.SetBlastrEnabled(false)
		metrics.SetImporterEnabled(false)
		return nil
	}

	system := &HavenSystem{}

	// Register base handlers
	system.Handler = RegisterHandlers(relay, cfg)

	// Initialize Blastr (broadcasts outbox events)
	if cfg.BlastrEnabled && len(cfg.BlastrRelays) > 0 {
		system.Blastr = NewBlastr(cfg)
		// Register Blastr's OnEventSaved handler
		relay.OnEventSaved = append(relay.OnEventSaved, system.Blastr.OnEventSaved())
		system.Blastr.Start()
		metrics.SetBlastrEnabled(true)
		log.Printf("HAVEN Blastr: broadcasting to %d relays", len(cfg.BlastrRelays))
	} else {
		metrics.SetBlastrEnabled(false)
	}

	// Initialize Importer (fetches inbox events via polling)
	if cfg.ImporterEnabled && len(cfg.ImporterRelays) > 0 && storeFunc != nil {
		system.Importer = NewImporter(cfg, storeFunc)
		system.Importer.Start()
		metrics.SetImporterEnabled(true)
		log.Printf("HAVEN Importer: polling %d relays", len(cfg.ImporterRelays))
	} else {
		metrics.SetImporterEnabled(false)
	}

	// Initialize Subscriber (real-time WebSocket subscriptions)
	if cfg.ImporterRealtimeEnabled && len(cfg.ImporterRelays) > 0 && storeFunc != nil {
		system.Subscriber = NewSubscriber(cfg, storeFunc)
		system.Subscriber.Start()
		log.Printf("HAVEN Subscriber: real-time subscriptions to %d relays", len(cfg.ImporterRelays))
	}

	return system
}

// Stop gracefully shuts down Blastr, Importer, and Subscriber
func (s *HavenSystem) Stop() {
	if s == nil {
		return
	}
	if s.Blastr != nil {
		s.Blastr.Stop()
	}
	if s.Importer != nil {
		s.Importer.Stop()
	}
	if s.Subscriber != nil {
		s.Subscriber.Stop()
	}
}

// SetEventLookup sets the event lookup interface for e-tag routing.
// This enables routing reactions/reposts to inbox when they reference owner's events.
func (s *HavenSystem) SetEventLookup(lookup EventLookup) {
	if s == nil || s.Handler == nil {
		return
	}
	s.Handler.SetEventLookup(lookup)
}

// SetMemberStore sets the member store for tier-based routing.
// This enables per-user HAVEN boxes based on membership tiers.
func (s *HavenSystem) SetMemberStore(store MemberStore) {
	if s == nil || s.Handler == nil {
		return
	}
	s.Handler.SetMemberStore(store)
	log.Println("HAVEN: member store configured for tier-based routing")
}

// Stats returns combined statistics
func (s *HavenSystem) Stats() map[string]interface{} {
	stats := make(map[string]interface{})
	if s.Blastr != nil {
		stats["blastr"] = s.Blastr.Stats()
	}
	if s.Importer != nil {
		stats["importer"] = s.Importer.Stats()
	}
	if s.Subscriber != nil {
		stats["subscriber"] = s.Subscriber.Stats()
	}
	return stats
}

// BoxStats returns statistics about events in each box
// This is a placeholder for future metrics integration
type BoxStats struct {
	Private int64
	Chat    int64
	Inbox   int64
	Outbox  int64
}

// String returns a formatted string representation
func (s BoxStats) String() string {
	return fmt.Sprintf("private=%d chat=%d inbox=%d outbox=%d",
		s.Private, s.Chat, s.Inbox, s.Outbox)
}

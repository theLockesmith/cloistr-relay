package haven

import (
	"context"

	"github.com/nbd-wtf/go-nostr"
)

// Router determines which box an event belongs to
type Router struct {
	config       *Config
	privateKinds map[int]bool
	chatKinds    map[int]bool
	inboxKinds   map[int]bool
	outboxKinds  map[int]bool
	eventLookup  EventLookup // Optional: for e-tag routing
}

// NewRouter creates a new box router
func NewRouter(cfg *Config) *Router {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	r := &Router{
		config:       cfg,
		privateKinds: make(map[int]bool),
		chatKinds:    make(map[int]bool),
		inboxKinds:   make(map[int]bool),
		outboxKinds:  make(map[int]bool),
	}

	// Build kind lookup maps
	for _, k := range DefaultPrivateKinds {
		r.privateKinds[k] = true
	}
	for _, k := range cfg.PrivateKinds {
		r.privateKinds[k] = true
	}
	for _, k := range DefaultChatKinds {
		r.chatKinds[k] = true
	}
	for _, k := range DefaultInboxKinds {
		r.inboxKinds[k] = true
	}
	for _, k := range DefaultOutboxKinds {
		r.outboxKinds[k] = true
	}

	return r
}

// SetEventLookup sets the event lookup interface for e-tag routing.
// When set, reactions (kind 7) and reposts (kind 6) will be routed to inbox
// if they reference one of the owner's events.
func (r *Router) SetEventLookup(lookup EventLookup) {
	r.eventLookup = lookup
}

// RouteEvent determines which box an event should be stored in
// Logic:
// 1. Private kinds always go to private box (owner only)
// 2. Chat kinds (DMs, gift wraps) go to chat box
// 3. Events FROM owner go to outbox
// 4. Events TO owner (tagged) go to inbox
// 5. Everything else goes to inbox (default for external events)
func (r *Router) RouteEvent(event *nostr.Event) Box {
	// Check if HAVEN is enabled
	if !r.config.Enabled {
		return BoxUnknown
	}

	// 1. Private kinds always go to private box
	if r.privateKinds[event.Kind] {
		// Only if from owner
		if event.PubKey == r.config.OwnerPubkey {
			return BoxPrivate
		}
		// Private kinds from others are rejected (handled by handler)
		return BoxUnknown
	}

	// 2. Chat kinds go to chat box
	if r.chatKinds[event.Kind] {
		return BoxChat
	}

	// 3. Events from owner go to outbox
	if event.PubKey == r.config.OwnerPubkey {
		return BoxOutbox
	}

	// 4. Check if event is addressed to owner (inbox)
	if r.isAddressedToOwner(event) {
		return BoxInbox
	}

	// 5. Default: external events not addressed to owner
	// These might be accepted based on relay policy (community relay mode)
	// For strict HAVEN mode, we'd reject these
	return BoxUnknown
}

// isAddressedToOwner checks if an event is addressed to the owner
// via p-tags, e-tags referencing owner's events, or other mechanisms
func (r *Router) isAddressedToOwner(event *nostr.Event) bool {
	ownerPubkey := r.config.OwnerPubkey
	if ownerPubkey == "" {
		return false
	}

	// Check p-tags (mentions, recipients)
	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}
		// p-tag: ["p", "<pubkey>", ...]
		if tag[0] == "p" && tag[1] == ownerPubkey {
			return true
		}
	}

	// For reactions (kind 7) and reposts (kind 6), check e-tags
	// to see if they reference the owner's events
	if (event.Kind == 6 || event.Kind == 7) && r.eventLookup != nil {
		if r.referencesOwnerEvent(event) {
			return true
		}
	}

	return false
}

// referencesOwnerEvent checks if an event's e-tags reference any of the owner's events
func (r *Router) referencesOwnerEvent(event *nostr.Event) bool {
	if r.eventLookup == nil {
		return false
	}

	ownerPubkey := r.config.OwnerPubkey

	// Extract all e-tags (event references)
	for _, tag := range event.Tags {
		if len(tag) < 2 || tag[0] != "e" {
			continue
		}

		eventID := tag[1]
		if eventID == "" {
			continue
		}

		// Look up the referenced event
		ctx := context.Background()
		referencedEvent, err := r.eventLookup.GetEventByID(ctx, eventID)
		if err != nil || referencedEvent == nil {
			continue
		}

		// Check if the referenced event is from the owner
		if referencedEvent.PubKey == ownerPubkey {
			return true
		}
	}

	return false
}

// RouteFilter determines which box(es) a filter should query
// Returns the boxes that should be queried and whether access is allowed
func (r *Router) RouteFilter(filter nostr.Filter, authedPubkey string) []Box {
	if !r.config.Enabled {
		return nil
	}

	boxes := make(map[Box]bool)

	// If filter specifies kinds, use them to determine boxes
	if len(filter.Kinds) > 0 {
		for _, kind := range filter.Kinds {
			if r.privateKinds[kind] {
				boxes[BoxPrivate] = true
			}
			if r.chatKinds[kind] {
				boxes[BoxChat] = true
			}
			if r.inboxKinds[kind] || r.outboxKinds[kind] {
				// Could be inbox or outbox depending on author filter
				if len(filter.Authors) > 0 {
					for _, author := range filter.Authors {
						if author == r.config.OwnerPubkey {
							boxes[BoxOutbox] = true
						}
					}
				}
				// Check p-tags for inbox
				if pTags, ok := filter.Tags["p"]; ok {
					for _, p := range pTags {
						if p == r.config.OwnerPubkey {
							boxes[BoxInbox] = true
						}
					}
				}
			}
		}
	}

	// If filter specifies authors as owner, likely outbox query
	if len(filter.Authors) > 0 {
		for _, author := range filter.Authors {
			if author == r.config.OwnerPubkey {
				boxes[BoxOutbox] = true
			}
		}
	}

	// If filter has p-tag for owner, likely inbox query
	if pTags, ok := filter.Tags["p"]; ok {
		for _, p := range pTags {
			if p == r.config.OwnerPubkey {
				boxes[BoxInbox] = true
			}
		}
	}

	// Convert map to slice
	result := make([]Box, 0, len(boxes))
	for box := range boxes {
		result = append(result, box)
	}

	// If no specific boxes determined, default behavior
	if len(result) == 0 {
		// If filter explicitly specifies authors that don't include owner,
		// don't default to outbox (outbox only contains owner's events)
		if len(filter.Authors) > 0 {
			hasOwner := false
			for _, author := range filter.Authors {
				if author == r.config.OwnerPubkey {
					hasOwner = true
					break
				}
			}
			if !hasOwner {
				// Filter asks for non-owner authors - return empty
				// (or could return inbox if those authors might have sent to owner)
				return []Box{}
			}
		}

		// For authenticated owner, allow all boxes
		if authedPubkey == r.config.OwnerPubkey {
			return []Box{BoxPrivate, BoxChat, BoxInbox, BoxOutbox}
		}
		// For others, only outbox (public content)
		return []Box{BoxOutbox}
	}

	return result
}

// CanAccessBox checks if a pubkey can access a specific box for read/write
func (r *Router) CanAccessBox(box Box, pubkey string, isWrite bool) bool {
	policies := BoxPolicies(r.config)
	policy, ok := policies[box]
	if !ok {
		return false
	}

	// Check if owner-only box
	if policy.OwnerOnly && pubkey != r.config.OwnerPubkey {
		return false
	}

	// Check auth requirements
	if isWrite && policy.WriteRequiresAuth && pubkey == "" {
		return false
	}
	if !isWrite && policy.ReadRequiresAuth && pubkey == "" {
		return false
	}

	// Special case: outbox write is owner-only
	if box == BoxOutbox && isWrite && pubkey != r.config.OwnerPubkey {
		return false
	}

	// Special case: outbox read with restricted access is owner-only
	// When AllowPublicOutboxRead is false, only owner can read outbox
	if box == BoxOutbox && !isWrite && policy.ReadRequiresAuth && pubkey != r.config.OwnerPubkey {
		return false
	}

	// Special case: inbox read is owner-only
	if box == BoxInbox && !isWrite && pubkey != r.config.OwnerPubkey {
		return false
	}

	return true
}

// GetBoxForKind returns the primary box for a given kind
func (r *Router) GetBoxForKind(kind int) Box {
	if r.privateKinds[kind] {
		return BoxPrivate
	}
	if r.chatKinds[kind] {
		return BoxChat
	}
	// Inbox/outbox depends on author, return unknown
	return BoxUnknown
}

// IsOwner checks if the pubkey is the relay owner
func (r *Router) IsOwner(pubkey string) bool {
	return pubkey == r.config.OwnerPubkey && pubkey != ""
}

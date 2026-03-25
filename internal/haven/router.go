package haven

import (
	"context"

	"github.com/nbd-wtf/go-nostr"
)

// RoutingResult represents the outcome of per-user routing
type RoutingResult struct {
	// Box is the destination box
	Box Box
	// OwnerPubkey is the user whose box this belongs to
	OwnerPubkey string
	// Tier is the user's membership tier (empty if single-owner mode)
	Tier string
}

// Router determines which box an event belongs to
type Router struct {
	config            *Config
	privateKinds      map[int]bool
	chatKinds         map[int]bool
	inboxKinds        map[int]bool
	outboxKinds       map[int]bool
	eventLookup       EventLookup       // Optional: for e-tag routing
	memberStore       MemberStore       // Optional: for per-user tier-based routing
	userSettingsStore *UserSettingsStore // Optional: for per-user haven settings
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

// SetMemberStore sets the member store for tier-based routing.
// When set, enables per-user HAVEN boxes based on membership tiers.
// This is the foundation for Phase 3 per-user routing.
func (r *Router) SetMemberStore(store MemberStore) {
	r.memberStore = store
}

// HasMemberStore returns true if a member store is configured
func (r *Router) HasMemberStore() bool {
	return r.memberStore != nil
}

// SetUserSettingsStore sets the per-user haven settings store.
// When set, enables per-user blastr/importer/privacy settings.
func (r *Router) SetUserSettingsStore(store *UserSettingsStore) {
	r.userSettingsStore = store
}

// HasUserSettingsStore returns true if a user settings store is configured
func (r *Router) HasUserSettingsStore() bool {
	return r.userSettingsStore != nil
}

// IsMultiUserMode returns true if the router is configured for multi-user operation
// (has a member store and no single owner configured)
func (r *Router) IsMultiUserMode() bool {
	return r.memberStore != nil && r.config.OwnerPubkey == ""
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

// RouteEventForUser determines routing for multi-user mode.
// Returns the box, owner pubkey, and tier information.
// This is the primary routing method for per-user HAVEN.
func (r *Router) RouteEventForUser(ctx context.Context, event *nostr.Event, authedPubkey string) RoutingResult {
	// If not enabled, return unknown
	if !r.config.Enabled {
		return RoutingResult{Box: BoxUnknown}
	}

	// If no member store (single-owner mode), use legacy routing
	if r.memberStore == nil {
		box := r.RouteEvent(event)
		return RoutingResult{
			Box:         box,
			OwnerPubkey: r.config.OwnerPubkey,
		}
	}

	// Multi-user mode: check membership and tier

	// 1. Private kinds go to the authenticated user's private box
	if r.privateKinds[event.Kind] {
		if authedPubkey != "" && event.PubKey == authedPubkey {
			member, err := r.memberStore.GetMemberInfo(ctx, authedPubkey)
			if err == nil && member != nil && member.HasHavenBoxes {
				return RoutingResult{
					Box:         BoxPrivate,
					OwnerPubkey: authedPubkey,
					Tier:        member.Tier,
				}
			}
		}
		// Private kinds from non-members or unauthenticated users are rejected
		return RoutingResult{Box: BoxUnknown}
	}

	// 2. Chat kinds go to chat box (participants determined elsewhere)
	if r.chatKinds[event.Kind] {
		// For chat, we need to identify participants
		// The sender might be authed, or we extract recipients from tags
		if authedPubkey != "" {
			member, err := r.memberStore.GetMemberInfo(ctx, authedPubkey)
			if err == nil && member != nil && member.HasHavenBoxes {
				return RoutingResult{
					Box:         BoxChat,
					OwnerPubkey: authedPubkey,
					Tier:        member.Tier,
				}
			}
		}
		// Chat from non-members still goes to chat (for recipients who are members)
		return RoutingResult{Box: BoxChat}
	}

	// 3. Events FROM authenticated member go to their outbox
	if authedPubkey != "" && event.PubKey == authedPubkey {
		member, err := r.memberStore.GetMemberInfo(ctx, authedPubkey)
		if err == nil && member != nil && member.HasHavenBoxes {
			return RoutingResult{
				Box:         BoxOutbox,
				OwnerPubkey: authedPubkey,
				Tier:        member.Tier,
			}
		}
	}

	// 4. Events TO a member (p-tagged) go to their inbox
	for _, tag := range event.Tags {
		if len(tag) < 2 || tag[0] != "p" {
			continue
		}
		targetPubkey := tag[1]
		if targetPubkey == "" {
			continue
		}

		member, err := r.memberStore.GetMemberInfo(ctx, targetPubkey)
		if err == nil && member != nil && member.HasHavenBoxes {
			return RoutingResult{
				Box:         BoxInbox,
				OwnerPubkey: targetPubkey,
				Tier:        member.Tier,
			}
		}
	}

	// 5. For reactions and reposts, check e-tags
	if (event.Kind == 6 || event.Kind == 7) && r.eventLookup != nil {
		targetOwner := r.findReferencedMemberEvent(ctx, event)
		if targetOwner != "" {
			member, err := r.memberStore.GetMemberInfo(ctx, targetOwner)
			if err == nil && member != nil && member.HasHavenBoxes {
				return RoutingResult{
					Box:         BoxInbox,
					OwnerPubkey: targetOwner,
					Tier:        member.Tier,
				}
			}
		}
	}

	// 6. Not addressed to any member with HAVEN boxes
	return RoutingResult{Box: BoxUnknown}
}

// findReferencedMemberEvent checks e-tags for events owned by members
func (r *Router) findReferencedMemberEvent(ctx context.Context, event *nostr.Event) string {
	if r.eventLookup == nil {
		return ""
	}

	for _, tag := range event.Tags {
		if len(tag) < 2 || tag[0] != "e" {
			continue
		}

		eventID := tag[1]
		if eventID == "" {
			continue
		}

		referencedEvent, err := r.eventLookup.GetEventByID(ctx, eventID)
		if err != nil || referencedEvent == nil {
			continue
		}

		// Check if the referenced event author is a member
		isMember, err := r.memberStore.IsMember(ctx, referencedEvent.PubKey)
		if err == nil && isMember {
			return referencedEvent.PubKey
		}
	}

	return ""
}

// RouteFilterForUser determines which boxes and users a filter should query
// Returns multiple routing results for multi-user queries
func (r *Router) RouteFilterForUser(ctx context.Context, filter nostr.Filter, authedPubkey string) []RoutingResult {
	if !r.config.Enabled {
		return nil
	}

	// Single-owner mode: use legacy routing
	if r.memberStore == nil {
		boxes := r.RouteFilter(filter, authedPubkey)
		results := make([]RoutingResult, len(boxes))
		for i, box := range boxes {
			results[i] = RoutingResult{
				Box:         box,
				OwnerPubkey: r.config.OwnerPubkey,
			}
		}
		return results
	}

	// Multi-user mode
	var results []RoutingResult

	// If filter specifies authors, check if they're members
	if len(filter.Authors) > 0 {
		for _, author := range filter.Authors {
			member, err := r.memberStore.GetMemberInfo(ctx, author)
			if err == nil && member != nil && member.HasHavenBoxes {
				// Author is a member - their outbox is relevant
				results = append(results, RoutingResult{
					Box:         BoxOutbox,
					OwnerPubkey: author,
					Tier:        member.Tier,
				})
			}
		}
	}

	// If filter has p-tags, check if targets are members
	if pTags, ok := filter.Tags["p"]; ok {
		for _, p := range pTags {
			member, err := r.memberStore.GetMemberInfo(ctx, p)
			if err == nil && member != nil && member.HasHavenBoxes {
				// Target is a member - their inbox is relevant
				results = append(results, RoutingResult{
					Box:         BoxInbox,
					OwnerPubkey: p,
					Tier:        member.Tier,
				})
			}
		}
	}

	// If authenticated user is querying with no specific targets, include their boxes
	if len(results) == 0 && authedPubkey != "" {
		member, err := r.memberStore.GetMemberInfo(ctx, authedPubkey)
		if err == nil && member != nil && member.HasHavenBoxes {
			// Include all of authed user's boxes
			for _, box := range []Box{BoxOutbox, BoxInbox, BoxPrivate, BoxChat} {
				results = append(results, RoutingResult{
					Box:         box,
					OwnerPubkey: authedPubkey,
					Tier:        member.Tier,
				})
			}
		}
	}

	return results
}

// CanAccessUserBox checks if a pubkey can access another user's box
func (r *Router) CanAccessUserBox(ctx context.Context, box Box, requesterPubkey, ownerPubkey string, isWrite bool) bool {
	// Owner always has access
	if requesterPubkey == ownerPubkey {
		return true
	}

	// Get owner's settings to check privacy preferences
	if r.userSettingsStore != nil {
		settings, err := r.userSettingsStore.GetSettings(ctx, ownerPubkey)
		if err == nil && settings != nil {
			switch box {
			case BoxOutbox:
				// Outbox write is always owner-only
				if isWrite {
					return false
				}
				// Check if public outbox read is allowed
				return settings.PublicOutboxRead
			case BoxInbox:
				// Inbox read is owner-only
				if !isWrite {
					return false
				}
				// Check if public inbox write is allowed
				return settings.PublicInboxWrite
			case BoxChat:
				// Chat requires auth
				if requesterPubkey == "" {
					return false
				}
				// Non-owner can read/write chat if they're a participant
				// This would need conversation participant checking
				return true
			case BoxPrivate:
				// Private is always owner-only
				return false
			}
		}
	}

	// Default policies (no user settings found)
	switch box {
	case BoxOutbox:
		return !isWrite // Read allowed, write denied
	case BoxInbox:
		return isWrite // Write allowed, read denied
	case BoxChat:
		return requesterPubkey != "" // Auth required
	case BoxPrivate:
		return false // Owner only
	}

	return false
}

// GetUserSettings returns haven settings for a user (nil if not configured)
func (r *Router) GetUserSettings(ctx context.Context, pubkey string) (*UserSettings, error) {
	if r.userSettingsStore == nil {
		return nil, nil
	}
	return r.userSettingsStore.GetSettings(ctx, pubkey)
}

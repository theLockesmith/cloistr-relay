// Package haven implements HAVEN-style relay separation
//
// HAVEN (High Availability Vault for Events on Nostr) implements the
// Outbox Model with four distinct "boxes" for event routing:
//
// - Private: Drafts, eCash, personal notes (owner only, auth required)
// - Chat: DMs and group chats (WoT-filtered, auth required)
// - Inbox: Events addressed to owner (public write, owner read)
// - Outbox: Owner's public notes (owner write, public read)
//
// Reference: https://github.com/bitvora/haven
package haven

import (
	"context"

	"github.com/nbd-wtf/go-nostr"
)

// EventLookup is an interface for looking up events by ID
// This is used for E-tag routing to determine if a reaction/repost
// is targeting one of the owner's events
type EventLookup interface {
	// GetEventByID returns an event by its ID, or nil if not found
	GetEventByID(ctx context.Context, id string) (*nostr.Event, error)
}

// Box represents the type of storage box for event routing
type Box int

const (
	// BoxUnknown is used when a box cannot be determined
	BoxUnknown Box = iota
	// BoxPrivate stores drafts, eCash, and personal notes (owner only)
	BoxPrivate
	// BoxChat stores DMs and gift wraps (WoT-filtered)
	BoxChat
	// BoxInbox stores events addressed to owner (mentions, replies, zaps)
	BoxInbox
	// BoxOutbox stores owner's public notes (readable by anyone)
	BoxOutbox
)

// String returns the human-readable name of the box
func (b Box) String() string {
	switch b {
	case BoxPrivate:
		return "private"
	case BoxChat:
		return "chat"
	case BoxInbox:
		return "inbox"
	case BoxOutbox:
		return "outbox"
	default:
		return "unknown"
	}
}

// DefaultPrivateKinds are event kinds that go to the private box by default
// These are personal/sensitive kinds that should only be accessible to owner
var DefaultPrivateKinds = []int{
	// Drafts and private notes
	30024, // Draft long-form content
	31234, // Draft (generic)
	// eCash
	7375, // Cashu wallet event
	7376, // Cashu spending history
	// Application-specific private data
	30078, // Application-specific data
	// Bookmarks (private by default)
	10003, // Bookmark list
	30003, // Bookmark sets
}

// DefaultChatKinds are event kinds that go to the chat box
// These require authentication and WoT filtering
var DefaultChatKinds = []int{
	4,    // Encrypted direct message (NIP-04, deprecated but still used)
	13,   // Seal (NIP-59)
	1059, // Gift wrap (NIP-59)
	1060, // Gift wrap (alternative)
}

// DefaultInboxKinds are event kinds that can go to the inbox
// These are events from others addressed to the owner
var DefaultInboxKinds = []int{
	1,     // Text note (when tagged)
	6,     // Repost (when reposting owner)
	7,     // Reaction (when reacting to owner)
	9735,  // Zap receipt (when zapping owner)
	1111,  // Comment (when commenting on owner's content)
	30023, // Long-form content (when referencing owner)
}

// DefaultOutboxKinds are event kinds for the outbox
// These are the owner's public posts
var DefaultOutboxKinds = []int{
	0,     // Metadata (profile)
	1,     // Text note
	3,     // Contact list (follows)
	6,     // Repost
	7,     // Reaction
	10002, // Relay list metadata (NIP-65)
	30023, // Long-form content
}

// Config holds HAVEN configuration
type Config struct {
	// Enabled activates HAVEN-style box routing
	Enabled bool
	// OwnerPubkey is the relay owner's pubkey (hex)
	// Events from this pubkey go to outbox, events to this pubkey go to inbox
	OwnerPubkey string
	// PrivateKinds are additional kinds to store in private box
	PrivateKinds []int
	// AllowPublicOutboxRead allows unauthenticated reads from outbox
	AllowPublicOutboxRead bool
	// AllowPublicInboxWrite allows unauthenticated writes to inbox
	AllowPublicInboxWrite bool
	// RequireAuthForChat requires authentication for chat box access
	RequireAuthForChat bool
	// RequireAuthForPrivate requires authentication for private box access
	RequireAuthForPrivate bool

	// Blastr settings (outgoing relay broadcast)
	BlastrEnabled bool     // Enable broadcasting outbox events to other relays
	BlastrRelays  []string // Relays to broadcast outbox events to

	// Blastr retry settings
	BlastrRetryEnabled  bool   // Enable persistent retry queue (requires Redis/Dragonfly)
	BlastrRetryKey      string // Redis key for retry queue (default: "haven:blastr:retry")
	BlastrMaxRetries    int    // Maximum retry attempts per event/relay (default: 6)
	BlastrRetryInterval int    // Retry worker interval in seconds (default: 30)

	// Importer settings (incoming event fetching)
	ImporterEnabled         bool     // Enable fetching inbox events from other relays
	ImporterRelays          []string // Relays to fetch inbox events from
	ImporterRealtimeEnabled bool     // Enable real-time WebSocket subscriptions (vs polling)
}

// DefaultConfig returns sensible defaults for HAVEN
func DefaultConfig() *Config {
	return &Config{
		Enabled:               false, // Disabled by default
		AllowPublicOutboxRead: true,  // Public can read owner's posts
		AllowPublicInboxWrite: true,  // Anyone can mention/tag the owner
		RequireAuthForChat:    true,  // DMs require authentication
		RequireAuthForPrivate: true,  // Private box requires authentication
		BlastrRetryKey:        "haven:blastr:retry",
		BlastrMaxRetries:      6,
		BlastrRetryInterval:   30,
	}
}

// AccessPolicy defines who can read/write to a box
type AccessPolicy struct {
	// ReadRequiresAuth if true, reading requires authentication
	ReadRequiresAuth bool
	// WriteRequiresAuth if true, writing requires authentication
	WriteRequiresAuth bool
	// OwnerOnly if true, only the owner pubkey can access
	OwnerOnly bool
	// WoTFiltered if true, applies Web of Trust filtering
	WoTFiltered bool
}

// BoxPolicies returns access policies for each box
func BoxPolicies(cfg *Config) map[Box]AccessPolicy {
	return map[Box]AccessPolicy{
		BoxPrivate: {
			ReadRequiresAuth:  cfg.RequireAuthForPrivate,
			WriteRequiresAuth: cfg.RequireAuthForPrivate,
			OwnerOnly:         true,
			WoTFiltered:       false,
		},
		BoxChat: {
			ReadRequiresAuth:  cfg.RequireAuthForChat,
			WriteRequiresAuth: cfg.RequireAuthForChat,
			OwnerOnly:         false,
			WoTFiltered:       true,
		},
		BoxInbox: {
			ReadRequiresAuth:  true, // Only owner reads inbox
			WriteRequiresAuth: !cfg.AllowPublicInboxWrite,
			OwnerOnly:         false, // Others write, owner reads
			WoTFiltered:       false,
		},
		BoxOutbox: {
			ReadRequiresAuth:  !cfg.AllowPublicOutboxRead,
			WriteRequiresAuth: true, // Only owner writes
			OwnerOnly:         false,
			WoTFiltered:       false,
		},
	}
}

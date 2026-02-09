// Package protected implements NIP-70 Protected Events
//
// NIP-70 defines a mechanism to prevent unauthorized republication of events.
// Events with a "-" tag are "protected" and relays should:
// 1. Reject protected events by default
// 2. Only accept them from authenticated users who match the event author
//
// This prevents third parties from republishing events to relays where
// the author doesn't want them stored.
package protected

import (
	"context"
	"log"

	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"
)

// Config holds NIP-70 configuration
type Config struct {
	// Enabled activates NIP-70 protected event handling
	Enabled bool
	// AllowProtectedEvents determines if the relay accepts protected events at all
	// If false, all protected events are rejected (default)
	// If true, protected events are accepted only from their authenticated authors
	AllowProtectedEvents bool
}

// DefaultConfig returns sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Enabled:              true,
		AllowProtectedEvents: true, // Accept protected events from authenticated authors
	}
}

// Handler manages NIP-70 protected event handling
type Handler struct {
	config *Config
}

// NewHandler creates a new NIP-70 handler
func NewHandler(cfg *Config) *Handler {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Handler{config: cfg}
}

// IsProtected checks if an event has the "-" tag marking it as protected
func IsProtected(event *nostr.Event) bool {
	for _, tag := range event.Tags {
		if len(tag) >= 1 && tag[0] == "-" {
			return true
		}
	}
	return false
}

// RejectProtectedEvent returns a handler that enforces NIP-70 protected event rules
// Protected events (those with a "-" tag) are:
// - Rejected if AllowProtectedEvents is false
// - Rejected if user is not authenticated
// - Rejected if authenticated user doesn't match event author
// - Accepted if authenticated user is the event author
func (h *Handler) RejectProtectedEvent() func(context.Context, *nostr.Event) (bool, string) {
	return func(ctx context.Context, event *nostr.Event) (bool, string) {
		if !h.config.Enabled {
			return false, ""
		}

		// Check if event is protected
		if !IsProtected(event) {
			return false, "" // Not protected, allow
		}

		// Protected events require special handling
		log.Printf("NIP-70: Protected event received from %s", event.PubKey[:16])

		// If relay doesn't accept protected events at all, reject
		if !h.config.AllowProtectedEvents {
			return true, "blocked: this relay does not accept protected events"
		}

		// Protected events require authentication
		authedPubkey := khatru.GetAuthed(ctx)
		if authedPubkey == "" {
			return true, "auth-required: authentication required to publish protected events"
		}

		// Verify the authenticated user is the event author
		if event.PubKey != authedPubkey {
			return true, "blocked: protected events can only be published by their author"
		}

		log.Printf("NIP-70: Accepting protected event from authenticated author %s", authedPubkey[:16])
		return false, ""
	}
}

// RegisterHandlers registers NIP-70 handlers with the relay
func RegisterHandlers(relay *khatru.Relay, cfg *Config) *Handler {
	handler := NewHandler(cfg)

	relay.RejectEvent = append(relay.RejectEvent, handler.RejectProtectedEvent())

	mode := "reject all"
	if cfg.AllowProtectedEvents {
		mode = "accept from authenticated authors"
	}
	log.Printf("NIP-70 protected events enabled (mode: %s)", mode)

	return handler
}

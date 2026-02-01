// Package giftwrap implements NIP-59 Gift Wrap event handling
//
// NIP-59 defines encrypted event wrapping with three layers:
// - Rumor: The original unsigned event
// - Seal (kind 13): Wraps the rumor with sender's key
// - Gift Wrap (kind 1059): Wraps sealed event with ephemeral key
//
// Relay responsibilities:
// - Accept kind 13 and kind 1059 events
// - Only serve kind 1059 events to tagged recipients (auth required)
// - Support deletion by the original signer
package giftwrap

import (
	"context"
	"log"

	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"
)

const (
	// KindSeal is the kind number for sealed events (NIP-59)
	KindSeal = 13
	// KindGiftWrap is the kind number for gift-wrapped events (NIP-59)
	KindGiftWrap = 1059
)

// Config holds NIP-59 configuration
type Config struct {
	// Enabled activates NIP-59 gift wrap support
	Enabled bool
	// RequireAuthForGiftWrap requires NIP-42 auth to query gift wrap events
	RequireAuthForGiftWrap bool
}

// DefaultConfig returns sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Enabled:                true,
		RequireAuthForGiftWrap: true,
	}
}

// Handler manages NIP-59 gift wrap event handling
type Handler struct {
	config *Config
}

// NewHandler creates a new NIP-59 handler
func NewHandler(cfg *Config) *Handler {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Handler{config: cfg}
}

// RejectGiftWrapFilter restricts access to gift wrap events
// Only allows queries for kind 1059 if:
// 1. The user is authenticated (NIP-42)
// 2. The filter includes a #p tag matching the authenticated pubkey
func (h *Handler) RejectGiftWrapFilter() func(context.Context, nostr.Filter) (bool, string) {
	return func(ctx context.Context, filter nostr.Filter) (bool, string) {
		// Check if filter includes gift wrap kind
		hasGiftWrap := false
		for _, k := range filter.Kinds {
			if k == KindGiftWrap {
				hasGiftWrap = true
				break
			}
		}

		// If not querying gift wrap, allow
		if !hasGiftWrap {
			return false, ""
		}

		// Gift wrap queries require authentication
		if !h.config.RequireAuthForGiftWrap {
			return false, ""
		}

		// Get authenticated pubkey from context
		authedPubkey := khatru.GetAuthed(ctx)
		if authedPubkey == "" {
			return true, "auth-required: authentication required to query gift wrap events"
		}

		// Check if filter includes the authenticated pubkey in #p tags
		pTags := filter.Tags["p"]
		if len(pTags) == 0 {
			return true, "restricted: must filter by your pubkey when querying gift wrap events"
		}

		// Verify the authenticated pubkey is in the filter
		found := false
		for _, p := range pTags {
			if p == authedPubkey {
				found = true
				break
			}
		}

		if !found {
			return true, "restricted: can only query gift wrap events addressed to you"
		}

		return false, ""
	}
}

// OverwriteGiftWrapFilter ensures kind 1059 queries are always filtered by recipient
// This adds the authenticated pubkey to the #p filter if missing
func (h *Handler) OverwriteGiftWrapFilter() func(context.Context, *nostr.Filter) {
	return func(ctx context.Context, filter *nostr.Filter) {
		// Check if filter includes gift wrap kind
		hasGiftWrap := false
		for _, k := range filter.Kinds {
			if k == KindGiftWrap {
				hasGiftWrap = true
				break
			}
		}

		if !hasGiftWrap {
			return
		}

		// Get authenticated pubkey
		authedPubkey := khatru.GetAuthed(ctx)
		if authedPubkey == "" {
			return
		}

		// Ensure #p tag filter includes only the authenticated pubkey
		if filter.Tags == nil {
			filter.Tags = make(nostr.TagMap)
		}
		filter.Tags["p"] = []string{authedPubkey}
	}
}

// OnEventSaved logs gift wrap events for monitoring
func (h *Handler) OnEventSaved() func(context.Context, *nostr.Event) {
	return func(ctx context.Context, event *nostr.Event) {
		switch event.Kind {
		case KindSeal:
			log.Printf("NIP-59: Seal stored from %s", event.PubKey[:8])
		case KindGiftWrap:
			recipient := getRecipient(event)
			log.Printf("NIP-59: Gift wrap stored for recipient %s", recipient)
		}
	}
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

// RegisterHandlers registers NIP-59 handlers with the relay
func RegisterHandlers(relay *khatru.Relay, cfg *Config) *Handler {
	handler := NewHandler(cfg)

	// Restrict gift wrap queries to authenticated recipients
	relay.RejectFilter = append(relay.RejectFilter, handler.RejectGiftWrapFilter())

	// Overwrite gift wrap filters to enforce recipient matching
	relay.OverwriteFilter = append(relay.OverwriteFilter, handler.OverwriteGiftWrapFilter())

	// Log gift wrap events
	relay.OnEventSaved = append(relay.OnEventSaved, handler.OnEventSaved())

	log.Printf("NIP-59 gift wrap enabled (auth required: %v)", cfg.RequireAuthForGiftWrap)

	return handler
}

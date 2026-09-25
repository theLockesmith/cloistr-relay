// Package giftwrap implements NIP-59 Gift Wrap event handling
//
// NIP-59 defines encrypted event wrapping with three layers:
// - Rumor: The original unsigned event
// - Seal (kind 13): Wraps the rumor with sender's key
// - Gift Wrap (kind 1059): Wraps sealed event with ephemeral key
//
// Relay responsibilities, in one of TWO modes:
//
//   RequireAuthForGiftWrap = true  (the NIP-59 recommendation)
//     Serve a wrap only to the key named in its p tag, and only to an
//     authenticated connection.
//
//   RequireAuthForGiftWrap = false (what Cloistr threads require)
//     Treat 1059 as an ordinary kind: serve it by any tag filter, to any
//     reader, with no login.
//
// The second mode exists because a thread message under the bucketed-mailbox
// design carries NO p tag at all. It is addressed to a thread key, not to a
// person, and members find it by a bucket tag shared with a crowd of unrelated
// threads. Under mode one such an event is accepted, stored, and served to
// nobody, not even the key that published it. Privacy comes from the encryption
// and the crowd, never from the relay choosing who may read, because a relay we
// do not run will not make that choice for us and the one we do run can read
// everything anyway.
//
// - Accept kind 13 and kind 1059 events
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
			// Deliberately NOT logging the recipient. A log line naming who
			// received a wrap is the same metadata the design removes from the
			// event, recorded somewhere an operator reads casually.
			log.Printf("NIP-59: Gift wrap stored")
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

	// BOTH restrictions are registered together or not at all. Gating only the
	// reject handler on the config would leave the overwrite handler rewriting
	// every authenticated reader's #p filter to their own pubkey, so anonymous
	// readers would work and logged-in ones would silently get nothing. That
	// half-state passes a casual check and is the exact shape of bug this
	// codebase keeps shipping, so the two are bound here deliberately.
	if cfg.RequireAuthForGiftWrap {
		// Restrict gift wrap queries to authenticated recipients
		relay.RejectFilter = append(relay.RejectFilter, handler.RejectGiftWrapFilter())

		// Overwrite gift wrap filters to enforce recipient matching
		relay.OverwriteFilter = append(relay.OverwriteFilter, handler.OverwriteGiftWrapFilter())
	}

	// Log gift wrap events
	relay.OnEventSaved = append(relay.OnEventSaved, handler.OnEventSaved())

	if cfg.RequireAuthForGiftWrap {
		log.Printf("NIP-59 gift wrap enabled: recipient-gated, auth required to query")
	} else {
		log.Printf("NIP-59 gift wrap enabled: served by tag to any reader, no auth (threads mode)")
	}

	return handler
}

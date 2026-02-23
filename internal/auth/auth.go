package auth

import (
	"context"
	"log"

	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"
)

// Policy defines the authentication policy for the relay
type Policy int

const (
	// PolicyOpen allows all reads and writes without authentication
	PolicyOpen Policy = iota
	// PolicyAuthRead requires authentication for reading events
	PolicyAuthRead
	// PolicyAuthWrite requires authentication for writing events
	PolicyAuthWrite
	// PolicyAuthAll requires authentication for all operations
	PolicyAuthAll
)

// String returns the string representation of the policy
func (p Policy) String() string {
	switch p {
	case PolicyOpen:
		return "open"
	case PolicyAuthRead:
		return "auth-read"
	case PolicyAuthWrite:
		return "auth-write"
	case PolicyAuthAll:
		return "auth-all"
	default:
		return "unknown"
	}
}

// Config holds authentication configuration
type Config struct {
	Policy         Policy
	AllowedPubkeys []string // If set, only these pubkeys can write (whitelist mode)
	ExemptKinds    []int    // Event kinds exempt from auth (e.g., 24133 for NIP-46)
}

// RegisterAuthHandlers adds NIP-42 authentication handlers to the relay
func RegisterAuthHandlers(relay *khatru.Relay, cfg *Config) {
	if cfg == nil {
		cfg = &Config{Policy: PolicyOpen}
	}

	// Register filter rejection based on auth policy
	if cfg.Policy == PolicyAuthRead || cfg.Policy == PolicyAuthAll {
		relay.RejectFilter = append(relay.RejectFilter, requireAuthForRead)
	}

	// Register event rejection based on auth policy
	if cfg.Policy == PolicyAuthWrite || cfg.Policy == PolicyAuthAll {
		relay.RejectEvent = append(relay.RejectEvent, requireAuthForWrite(cfg))
	}

	log.Printf("NIP-42 authentication handlers registered (policy: %v)", cfg.Policy)
}

// requireAuthForRead rejects filter requests from unauthenticated clients
func requireAuthForRead(ctx context.Context, filter nostr.Filter) (reject bool, msg string) {
	pubkey := khatru.GetAuthed(ctx)
	if pubkey == "" {
		// Return auth-required prefix - khatru will send AUTH challenge automatically
		return true, "auth-required: authentication required to read from this relay"
	}
	log.Printf("Authenticated read from %s", pubkey[:16])
	return false, ""
}

// requireAuthForWrite returns a handler that rejects events from unauthenticated clients
func requireAuthForWrite(cfg *Config) func(context.Context, *nostr.Event) (bool, string) {
	// Build exempt kinds set for O(1) lookup
	exemptKinds := make(map[int]bool)
	for _, kind := range cfg.ExemptKinds {
		exemptKinds[kind] = true
	}

	return func(ctx context.Context, event *nostr.Event) (reject bool, msg string) {
		// Allow AUTH events (kind 22242) through without authentication
		// These are handled specially by khatru for NIP-42
		if event.Kind == 22242 {
			return false, ""
		}

		// Allow exempt kinds (e.g., 24133 for NIP-46) without authentication
		if exemptKinds[event.Kind] {
			log.Printf("Allowing auth-exempt kind %d from %s", event.Kind, event.PubKey[:16])
			return false, ""
		}

		pubkey := khatru.GetAuthed(ctx)

		// Check if authenticated
		if pubkey == "" {
			return true, "auth-required: authentication required to publish events"
		}

		// Verify the event is from the authenticated user
		if event.PubKey != pubkey {
			return true, "restricted: you can only publish events as your authenticated identity"
		}

		// Check whitelist if configured
		if len(cfg.AllowedPubkeys) > 0 {
			allowed := false
			for _, allowed_pk := range cfg.AllowedPubkeys {
				if pubkey == allowed_pk {
					allowed = true
					break
				}
			}
			if !allowed {
				return true, "restricted: your pubkey is not on the whitelist"
			}
		}

		log.Printf("Authenticated write from %s", pubkey[:16])
		return false, ""
	}
}

// GetAuthenticatedPubkey is a helper to get the authenticated pubkey from context
func GetAuthenticatedPubkey(ctx context.Context) string {
	return khatru.GetAuthed(ctx)
}

// IsAuthenticated returns true if the context has an authenticated user
func IsAuthenticated(ctx context.Context) bool {
	return khatru.GetAuthed(ctx) != ""
}

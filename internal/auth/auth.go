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
	Policy Policy

	// WriteWhitelist is RESTRICTIVE: when non-empty, only these pubkeys may write
	// and every other authenticated pubkey is rejected. Empty means unrestricted.
	//
	// It is deliberately NOT the same list as config.AllowedPubkeys, which is a
	// PRIVILEGE grant consumed by the WoT gate, where the field is documented as
	// "AllowedPubkeys bypasses WoT requirements (treated as trusted)"
	// (internal/wot/types.go).
	//
	// They were the SAME value until 2026-09-04, and the two meanings cancelled
	// each other out: adding a key to ALLOWED_PUBKEYS to exempt it from WoT
	// simultaneously locked out every pubkey NOT on the list. Production ran that
	// way with four entries, so a fresh user completed NIP-42 AUTH and was then
	// told
	//     "restricted: your pubkey is not on the whitelist"
	// and could not save a profile, a follow, or a file. Auth is registered
	// BEFORE WoT (cmd/relay/main.go) and khatru runs RejectEvent handlers in
	// registration order, so this gate answered first and WoT's authed-author
	// exemption -- the fix written specifically to open the relay up -- was never
	// reached.
	//
	// Populated from WRITE_WHITELIST_PUBKEYS, which defaults to empty. A
	// single-user or invite-only deployment sets it; the hosted relay does not.
	WriteWhitelist []string

	ExemptKinds []int // Event kinds exempt from auth (e.g., 24133 for NIP-46)
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

	// Same for the write whitelist, so a long list does not cost a linear scan per
	// event.
	whitelist := make(map[string]struct{}, len(cfg.WriteWhitelist))
	for _, pk := range cfg.WriteWhitelist {
		whitelist[pk] = struct{}{}
	}

	return func(ctx context.Context, event *nostr.Event) (reject bool, msg string) {
		authed := khatru.GetAuthed(ctx)
		reject, msg = evaluateWrite(exemptKinds, whitelist, authed, event)
		if reject {
			return reject, msg
		}
		// Logging stays out here so evaluateWrite has no side effects and can be
		// called directly from tests. Same two lines as before the split.
		switch {
		case exemptKinds[event.Kind]:
			log.Printf("Allowing auth-exempt kind %d from %s", event.Kind, shortKey(event.PubKey))
		case authed != "":
			log.Printf("Authenticated write from %s", shortKey(authed))
		}
		return false, ""
	}
}

// evaluateWrite is the entire write-side auth decision, with the authenticated
// pubkey passed IN rather than read from a khatru context.
//
// It is split out because it is the gate that decides whether the relay accepts
// any user at all, and every test that covered it was a t.Skip("Requires khatru
// context") -- four of them, naming the exact behaviours that later shipped
// broken. A decision that cannot be called without a live websocket does not get
// tested, so this one takes its inputs as arguments.
func evaluateWrite(
	exemptKinds map[int]bool,
	whitelist map[string]struct{},
	authed string,
	event *nostr.Event,
) (reject bool, msg string) {
	// Allow AUTH events (kind 22242) through without authentication
	// These are handled specially by khatru for NIP-42
	if event.Kind == 22242 {
		return false, ""
	}

	// Allow exempt kinds (e.g., 24133 for NIP-46) without authentication
	if exemptKinds[event.Kind] {
		return false, ""
	}

	// Check if authenticated
	if authed == "" {
		return true, "auth-required: authentication required to publish events"
	}

	// Verify the event is from the authenticated user
	if event.PubKey != authed {
		return true, "restricted: you can only publish events as your authenticated identity"
	}

	// Check the restrictive write whitelist, if one is configured. Empty means
	// unrestricted -- see the Config.WriteWhitelist comment for why that default
	// matters and what happened when this list was shared with the WoT bypass.
	if len(whitelist) > 0 {
		if _, ok := whitelist[authed]; !ok {
			return true, "restricted: your pubkey is not on the whitelist"
		}
	}

	return false, ""
}

// GetAuthenticatedPubkey is a helper to get the authenticated pubkey from context
func GetAuthenticatedPubkey(ctx context.Context) string {
	return khatru.GetAuthed(ctx)
}

// IsAuthenticated returns true if the context has an authenticated user
func IsAuthenticated(ctx context.Context) bool {
	return khatru.GetAuthed(ctx) != ""
}

// shortKey truncates a pubkey for logging without panicking on a malformed one.
// The previous code sliced [:16] unconditionally.
func shortKey(pk string) string {
	if len(pk) > 16 {
		return pk[:16]
	}
	return pk
}

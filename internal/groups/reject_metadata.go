package groups

import (
	"context"
	"fmt"
	"regexp"

	"github.com/nbd-wtf/go-nostr"
)

// ownerPattern matches the Space group identifier format:
//
//	{slug}-{16-hex-pubkey-prefix}-{8-hex-random}
//
// The captured group is the 16-character hex prefix of the owner's pubkey.
// Groups created before this scheme have no pubkey in their d-tag and will
// not match; those are allowed through (see RejectExternalMetadata).
var ownerPattern = regexp.MustCompile(`^[a-z0-9-]+-([0-9a-f]{16})-[0-9a-f]{8}$`)

// RejectExternalMetadata is a khatru RejectEvent handler that refuses
// externally-submitted NIP-29 group metadata events (kinds 39000-39009)
// unless the author is the group's owner.
//
// Per NIP-29, these kinds are relay-generated: the relay publishes them in
// response to moderation actions, and external clients should never submit
// them. Without this handler, a relay that has GROUPS_ENABLED=false (or
// unset) stores them as ordinary addressable events with no ownership check,
// letting anyone claim admin status for any group.
//
// Ownership is derived from the event's d-tag, which encodes a 16-character
// hex prefix of the owner's pubkey (the Space identifier format). If the
// event author's pubkey starts with that prefix, the event is allowed.
// Forging this requires finding a secp256k1 keypair whose pubkey shares a
// 16-hex (64-bit) prefix with the target, which is computationally infeasible.
//
// Legacy groups whose d-tag does not match the owner pattern are allowed
// through. The three groups on production today (test-project-t9mn5b1,
// my-group-3cxnejl, test-77qwa6p) predate the scheme and have no derivable
// owner. Refusing writes to groups a user can still see is a worse failure
// than the status-quo gap on test fixtures.
//
// relayPubkey is the relay's own public key. Events signed by the relay
// itself are always exempt (relay29 publishes metadata under the relay key
// when GROUPS_ENABLED is true).
func RejectExternalMetadata(relayPubkey string) func(context.Context, *nostr.Event) (bool, string) {
	return func(_ context.Context, event *nostr.Event) (bool, string) {
		if !IsGroupMetadataKind(event.Kind) {
			return false, ""
		}

		// The relay's own events are always allowed.
		if relayPubkey != "" && event.PubKey == relayPubkey {
			return false, ""
		}

		// Derive the owner from the d-tag.
		dTag := event.Tags.GetD()
		if dTag == "" {
			// No d-tag at all: reject. Addressable events without a d-tag
			// are malformed, and letting them through would store a metadata
			// event with no group identity.
			return true, fmt.Sprintf("blocked: kind %d events require a d-tag", event.Kind)
		}

		match := ownerPattern.FindStringSubmatch(dTag)
		if match == nil {
			// Legacy identifier format with no embedded pubkey. Allow it
			// through: refusing writes to a group the user can still see
			// is worse than the status-quo gap on these test fixtures.
			return false, ""
		}

		// The 16-hex prefix in the d-tag must match the event author.
		ownerPrefix := match[1]
		if len(event.PubKey) >= 16 && event.PubKey[:16] == ownerPrefix {
			return false, ""
		}

		return true, fmt.Sprintf("blocked: kind %d can only be published by the group owner", event.Kind)
	}
}

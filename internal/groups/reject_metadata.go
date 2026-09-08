package groups

import (
	"context"
	"fmt"

	"github.com/nbd-wtf/go-nostr"
)

// RejectExternalMetadata is a khatru RejectEvent handler that refuses
// externally-submitted NIP-29 group metadata events (kinds 39000-39009)
// unless the author is authorized to write them.
//
// Per NIP-29, these kinds are relay-generated: the relay publishes them in
// response to moderation actions, and external clients should never submit
// them. Without this handler, a relay that has GROUPS_ENABLED=false (or
// unset) stores them as ordinary addressable events with no ownership check,
// letting anyone claim admin status for any group.
//
// Authorization follows Space's trust model (trustedWriters.ts):
//   - 39000 (metadata), 39001 (admin list): current owner only.
//   - 39002 (member list): current owner, or a delegated admin with
//     add-user or remove-user permission in the latest owner-signed 39001.
//   - 39003-39009: current owner only (conservative default).
//
// Ownership is resolved from the d-tag's embedded pubkey prefix, then by
// walking the kind-39000 transfer chain in the store. Legacy groups whose
// d-tag has no embedded prefix are allowed through.
//
// relayPubkey is the relay's own public key. Events signed by the relay
// itself are always exempt (relay29 publishes metadata under the relay key
// when GROUPS_ENABLED is true).
//
// store may be nil. When nil, the handler falls back to prefix-only
// ownership (no transfer chain, no delegated admin check). This keeps the
// handler functional in tests and during startup before the store is ready.
func RejectExternalMetadata(relayPubkey string, store EventQuerier) func(context.Context, *nostr.Event) (bool, string) {
	return func(ctx context.Context, event *nostr.Event) (bool, string) {
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

		// If we have a store, use the full ownership resolver (transfer chain
		// + delegated admin check for 39002).
		if store != nil {
			authorized, err := IsAuthorizedMetadataWriter(ctx, store, dTag, event.Kind, event.PubKey)
			if err != nil {
				// Store error: reject with a generic message rather than
				// allowing potentially unauthorized writes.
				return true, fmt.Sprintf("blocked: kind %d authorization check failed", event.Kind)
			}
			if authorized {
				return false, ""
			}
			return true, fmt.Sprintf("blocked: kind %d can only be published by an authorized writer", event.Kind)
		}

		// Fallback: no store available. Use prefix-only matching (genesis
		// owner only, no transfer chain, no delegated admin).
		match := ownerPattern.FindStringSubmatch(dTag)
		if match == nil {
			// Legacy identifier format with no embedded pubkey. Allow it
			// through: refusing writes to a group the user can still see
			// is worse than the status-quo gap on these test fixtures.
			return false, ""
		}

		ownerPrefix := match[1]
		if len(event.PubKey) >= 16 && event.PubKey[:16] == ownerPrefix {
			return false, ""
		}

		return true, fmt.Sprintf("blocked: kind %d can only be published by the group owner", event.Kind)
	}
}

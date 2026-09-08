package groups

import (
	"context"
	"fmt"

	"github.com/nbd-wtf/go-nostr"
)

// RejectExternalMetadata is a khatru RejectEvent handler that refuses
// externally-submitted NIP-29 group metadata events (kinds 39000-39009).
//
// Per NIP-29, these kinds are relay-generated: the relay publishes them in
// response to moderation actions, and external clients should never submit
// them. Without this handler, a relay that has GROUPS_ENABLED=false (or
// unset) stores them as ordinary addressable events with no ownership check,
// letting anyone claim admin status for any group.
//
// This handler runs unconditionally, outside the GROUPS_ENABLED gate. When
// groups ARE enabled, relay29 handles these kinds internally and never
// exposes them to the RejectEvent chain, so this handler is a no-op in
// that path.
//
// relayPubkey is the relay's own public key. Events signed by the relay
// itself are exempt (relay29 publishes metadata under the relay key).
func RejectExternalMetadata(relayPubkey string) func(context.Context, *nostr.Event) (bool, string) {
	return func(_ context.Context, event *nostr.Event) (bool, string) {
		if !IsGroupMetadataKind(event.Kind) {
			return false, ""
		}
		// The relay's own events are allowed (relay29 self-publishes metadata).
		if relayPubkey != "" && event.PubKey == relayPubkey {
			return false, ""
		}
		return true, fmt.Sprintf("blocked: kind %d events are relay-generated (NIP-29)", event.Kind)
	}
}

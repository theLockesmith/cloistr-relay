package groups

import (
	"context"
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

func TestRejectExternalMetadata_BlocksMetadataKinds(t *testing.T) {
	reject := RejectExternalMetadata("relay-pubkey-hex")

	metadataKinds := []int{39000, 39001, 39002, 39003, 39009}
	for _, kind := range metadataKinds {
		event := &nostr.Event{Kind: kind, PubKey: "external-user"}
		blocked, msg := reject(context.Background(), event)
		if !blocked {
			t.Errorf("kind %d from external user should be rejected", kind)
		}
		if msg == "" {
			t.Errorf("kind %d rejection should include a message", kind)
		}
	}
}

func TestRejectExternalMetadata_AllowsRelayPubkey(t *testing.T) {
	relayPK := "relay-pubkey-hex"
	reject := RejectExternalMetadata(relayPK)

	metadataKinds := []int{39000, 39001, 39002, 39003}
	for _, kind := range metadataKinds {
		event := &nostr.Event{Kind: kind, PubKey: relayPK}
		blocked, _ := reject(context.Background(), event)
		if blocked {
			t.Errorf("kind %d from relay's own pubkey should be allowed", kind)
		}
	}
}

func TestRejectExternalMetadata_IgnoresNonMetadataKinds(t *testing.T) {
	reject := RejectExternalMetadata("relay-pubkey-hex")

	normalKinds := []int{1, 9, 9000, 9007, 30023, 38999, 39010}
	for _, kind := range normalKinds {
		event := &nostr.Event{Kind: kind, PubKey: "anyone"}
		blocked, _ := reject(context.Background(), event)
		if blocked {
			t.Errorf("kind %d should not be blocked by metadata rejection", kind)
		}
	}
}

func TestRejectExternalMetadata_EmptyRelayPubkey(t *testing.T) {
	// When no relay pubkey is configured, all metadata kinds are rejected.
	reject := RejectExternalMetadata("")

	event := &nostr.Event{Kind: 39000, PubKey: "anyone"}
	blocked, _ := reject(context.Background(), event)
	if !blocked {
		t.Error("kind 39000 should be rejected even with empty relay pubkey")
	}
}

package groups

import (
	"context"
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

func TestRejectExternalMetadata_BlocksMetadataKinds(t *testing.T) {
	reject := RejectExternalMetadata("relay-pubkey-hex")

	// Events with no d-tag are rejected (malformed addressable event).
	metadataKinds := []int{39000, 39001, 39002, 39003, 39009}
	for _, kind := range metadataKinds {
		event := &nostr.Event{Kind: kind, PubKey: "external-user"}
		blocked, msg := reject(context.Background(), event)
		if !blocked {
			t.Errorf("kind %d from external user (no d-tag) should be rejected", kind)
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
	// When no relay pubkey is configured, metadata events with no d-tag
	// are still rejected (malformed).
	reject := RejectExternalMetadata("")

	event := &nostr.Event{Kind: 39000, PubKey: "anyone"}
	blocked, _ := reject(context.Background(), event)
	if !blocked {
		t.Error("kind 39000 should be rejected even with empty relay pubkey")
	}
}

func TestRejectExternalMetadata_OwnerAllowed(t *testing.T) {
	reject := RejectExternalMetadata("relay-pubkey-hex")

	// d-tag encodes a 16-hex prefix of the owner's pubkey.
	// Author whose pubkey starts with that prefix -> allowed.
	ownerPubkey := "abcdef0123456789" + "0000111122223333"
	event := &nostr.Event{
		Kind:   39000,
		PubKey: ownerPubkey,
		Tags:   nostr.Tags{nostr.Tag{"d", "my-space-abcdef0123456789-a1b2c3d4"}},
	}
	blocked, msg := reject(context.Background(), event)
	if blocked {
		t.Errorf("owner should be allowed to publish kind 39000, got blocked: %s", msg)
	}
}

func TestRejectExternalMetadata_NonOwnerBlocked(t *testing.T) {
	reject := RejectExternalMetadata("relay-pubkey-hex")

	// Author's pubkey does NOT start with the prefix in the d-tag -> blocked.
	attackerPubkey := "ffff000000000000" + "aaaaaaaaaaaaaaaa"
	event := &nostr.Event{
		Kind:   39000,
		PubKey: attackerPubkey,
		Tags:   nostr.Tags{nostr.Tag{"d", "my-space-abcdef0123456789-a1b2c3d4"}},
	}
	blocked, msg := reject(context.Background(), event)
	if !blocked {
		t.Error("non-owner should be blocked from publishing kind 39000")
	}
	if msg == "" {
		t.Error("rejection message should not be empty")
	}
}

func TestRejectExternalMetadata_LegacyDtagAllowed(t *testing.T) {
	reject := RejectExternalMetadata("relay-pubkey-hex")

	// Legacy d-tags that don't match the owner pattern are allowed through.
	legacyTags := []string{
		"test-project-t9mn5b1",
		"my-group-3cxnejl",
		"test-77qwa6p",
		"simple",
	}
	for _, dtag := range legacyTags {
		event := &nostr.Event{
			Kind:   39000,
			PubKey: "anyone",
			Tags:   nostr.Tags{nostr.Tag{"d", dtag}},
		}
		blocked, msg := reject(context.Background(), event)
		if blocked {
			t.Errorf("legacy d-tag %q should be allowed through, got blocked: %s", dtag, msg)
		}
	}
}

func TestRejectExternalMetadata_NoDtagRejected(t *testing.T) {
	reject := RejectExternalMetadata("relay-pubkey-hex")

	// An empty Tags slice means GetD() returns "" -> rejected.
	event := &nostr.Event{
		Kind:   39001,
		PubKey: "anyone",
		Tags:   nostr.Tags{},
	}
	blocked, msg := reject(context.Background(), event)
	if !blocked {
		t.Error("metadata event with no d-tag should be rejected")
	}
	if msg == "" {
		t.Error("rejection message should not be empty")
	}
}

func TestRejectExternalMetadata_AllMetadataKindsOwnerChecked(t *testing.T) {
	reject := RejectExternalMetadata("relay-pubkey-hex")

	ownerPubkey := "1234567890abcdef" + "fedcba0987654321"
	attackerPubkey := "eeeeeeeeeeeeeeee" + "dddddddddddddddd"
	dtag := "chat-room-1234567890abcdef-deadbeef"

	metadataKinds := []int{39000, 39001, 39002, 39003, 39009}
	for _, kind := range metadataKinds {
		// Owner allowed for every metadata kind.
		ownerEvent := &nostr.Event{
			Kind:   kind,
			PubKey: ownerPubkey,
			Tags:   nostr.Tags{nostr.Tag{"d", dtag}},
		}
		blocked, msg := reject(context.Background(), ownerEvent)
		if blocked {
			t.Errorf("kind %d: owner should be allowed, got blocked: %s", kind, msg)
		}

		// Non-owner blocked for every metadata kind.
		attackerEvent := &nostr.Event{
			Kind:   kind,
			PubKey: attackerPubkey,
			Tags:   nostr.Tags{nostr.Tag{"d", dtag}},
		}
		blocked, msg = reject(context.Background(), attackerEvent)
		if !blocked {
			t.Errorf("kind %d: non-owner should be blocked", kind)
		}
	}
}

func TestOwnerPattern_Matches(t *testing.T) {
	cases := []struct {
		dtag   string
		match  bool
		prefix string
	}{
		// Standard Space format.
		{"my-space-abcdef0123456789-a1b2c3d4", true, "abcdef0123456789"},
		// Multi-segment slug.
		{"a-b-c-1234567890abcdef-deadbeef", true, "1234567890abcdef"},
		// Legacy: no 16-hex + 8-hex suffix.
		{"test-project-t9mn5b1", false, ""},
		{"my-group-3cxnejl", false, ""},
		{"test-77qwa6p", false, ""},
		{"simple", false, ""},
		// Empty string.
		{"", false, ""},
		// Almost matches but hex prefix too short (14 chars, not 16).
		{"space-abcdef01234567-a1b2c3d4", false, ""},
	}

	for _, c := range cases {
		match := ownerPattern.FindStringSubmatch(c.dtag)
		if c.match && match == nil {
			t.Errorf("ownerPattern should match %q but did not", c.dtag)
		} else if !c.match && match != nil {
			t.Errorf("ownerPattern should NOT match %q but got %v", c.dtag, match)
		} else if c.match && match[1] != c.prefix {
			t.Errorf("ownerPattern on %q: expected prefix %q, got %q", c.dtag, c.prefix, match[1])
		}
	}
}

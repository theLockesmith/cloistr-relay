package groups

import (
	"context"
	"fmt"
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

// mockStore is a test double for EventQuerier. It returns events from an
// in-memory map keyed by "{kind}:{d-tag}".
type mockStore struct {
	events map[string][]*nostr.Event
}

func newMockStore() *mockStore {
	return &mockStore{events: make(map[string][]*nostr.Event)}
}

func (m *mockStore) addEvent(evt *nostr.Event) {
	dTag := evt.Tags.GetD()
	key := fmt.Sprintf("%d:%s", evt.Kind, dTag)
	m.events[key] = append(m.events[key], evt)
}

func (m *mockStore) QueryEvents(_ context.Context, filter nostr.Filter) (chan *nostr.Event, error) {
	ch := make(chan *nostr.Event)
	go func() {
		defer close(ch)
		for _, kind := range filter.Kinds {
			dTags := filter.Tags["d"]
			for _, dTag := range dTags {
				key := fmt.Sprintf("%d:%s", kind, dTag)
				for _, evt := range m.events[key] {
					ch <- evt
				}
			}
		}
	}()
	return ch, nil
}

// --- Helper to build test events ---

func metadataEvent(kind int, pubkey string, dTag string, createdAt nostr.Timestamp, extraTags ...nostr.Tag) *nostr.Event {
	tags := nostr.Tags{nostr.Tag{"d", dTag}}
	tags = append(tags, extraTags...)
	return &nostr.Event{
		Kind:      kind,
		PubKey:    pubkey,
		CreatedAt: createdAt,
		Tags:      tags,
		ID:        pubkey[:8] + fmt.Sprintf("-%d-%d", kind, createdAt), // deterministic for tiebreak
	}
}

// ============================================================
// Tests with nil store (prefix-only fallback)
// ============================================================

func TestRejectExternalMetadata_BlocksMetadataKinds(t *testing.T) {
	reject := RejectExternalMetadata("relay-pubkey-hex", nil)

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
	reject := RejectExternalMetadata(relayPK, nil)

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
	reject := RejectExternalMetadata("relay-pubkey-hex", nil)

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
	reject := RejectExternalMetadata("", nil)

	event := &nostr.Event{Kind: 39000, PubKey: "anyone"}
	blocked, _ := reject(context.Background(), event)
	if !blocked {
		t.Error("kind 39000 should be rejected even with empty relay pubkey")
	}
}

func TestRejectExternalMetadata_OwnerAllowed(t *testing.T) {
	reject := RejectExternalMetadata("relay-pubkey-hex", nil)

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
	reject := RejectExternalMetadata("relay-pubkey-hex", nil)

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
	reject := RejectExternalMetadata("relay-pubkey-hex", nil)

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
	reject := RejectExternalMetadata("relay-pubkey-hex", nil)

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
	reject := RejectExternalMetadata("relay-pubkey-hex", nil)

	ownerPubkey := "1234567890abcdef" + "fedcba0987654321"
	attackerPubkey := "eeeeeeeeeeeeeeee" + "dddddddddddddddd"
	dtag := "chat-room-1234567890abcdef-deadbeef"

	metadataKinds := []int{39000, 39001, 39002, 39003, 39009}
	for _, kind := range metadataKinds {
		ownerEvent := &nostr.Event{
			Kind:   kind,
			PubKey: ownerPubkey,
			Tags:   nostr.Tags{nostr.Tag{"d", dtag}},
		}
		blocked, msg := reject(context.Background(), ownerEvent)
		if blocked {
			t.Errorf("kind %d: owner should be allowed, got blocked: %s", kind, msg)
		}

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
		{"my-space-abcdef0123456789-a1b2c3d4", true, "abcdef0123456789"},
		{"a-b-c-1234567890abcdef-deadbeef", true, "1234567890abcdef"},
		{"test-project-t9mn5b1", false, ""},
		{"my-group-3cxnejl", false, ""},
		{"test-77qwa6p", false, ""},
		{"simple", false, ""},
		{"", false, ""},
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

// ============================================================
// Tests with mock store (full ownership resolution)
// ============================================================

// REQUIRED TEST 1: Delegated admin's 39002 is ACCEPTED when owner's
// 39001 grants add-user.
func TestRejectExternalMetadata_DelegatedAdminMemberWrite(t *testing.T) {
	store := newMockStore()
	ownerPubkey := "abcdef0123456789" + "0000111122223333"
	adminPubkey := "dddd000000000000" + "aaaa111122223333"
	dtag := "my-space-abcdef0123456789-a1b2c3d4"

	// Owner's 39000 (establishes ownership).
	store.addEvent(metadataEvent(39000, ownerPubkey, dtag, 1000))

	// Owner's 39001 (admin list) grants add-user and remove-user (Space format, no role label).
	store.addEvent(metadataEvent(39001, ownerPubkey, dtag, 1001,
		nostr.Tag{"p", adminPubkey, "add-user", "remove-user"},
	))

	reject := RejectExternalMetadata("relay-pubkey", store)

	// Delegated admin publishing 39002 should be ACCEPTED.
	event := &nostr.Event{
		Kind:   39002,
		PubKey: adminPubkey,
		Tags:   nostr.Tags{nostr.Tag{"d", dtag}},
	}
	blocked, msg := reject(context.Background(), event)
	if blocked {
		t.Errorf("delegated admin with add-user should be allowed to publish 39002, got blocked: %s", msg)
	}
}

// REQUIRED TEST 2: Transferred owner's 39001 is ACCEPTED.
func TestRejectExternalMetadata_TransferredOwnerAccepted(t *testing.T) {
	store := newMockStore()
	originalOwner := "abcdef0123456789" + "0000111122223333"
	successor := "bbbb000000000000" + "cccc111122223333"
	dtag := "my-space-abcdef0123456789-a1b2c3d4"

	// Original owner's 39000 with transfer-to tag.
	store.addEvent(metadataEvent(39000, originalOwner, dtag, 1000,
		nostr.Tag{"transfer-to", successor},
	))

	reject := RejectExternalMetadata("relay-pubkey", store)

	// Successor publishing 39001 should be ACCEPTED.
	event := &nostr.Event{
		Kind:   39001,
		PubKey: successor,
		Tags:   nostr.Tags{nostr.Tag{"d", dtag}},
	}
	blocked, msg := reject(context.Background(), event)
	if blocked {
		t.Errorf("transferred owner should be allowed to publish 39001, got blocked: %s", msg)
	}
}

// REQUIRED TEST 3: Original owner's 39001 is REFUSED after transferring away.
func TestRejectExternalMetadata_OriginalOwnerRefusedAfterTransfer(t *testing.T) {
	store := newMockStore()
	originalOwner := "abcdef0123456789" + "0000111122223333"
	successor := "bbbb000000000000" + "cccc111122223333"
	dtag := "my-space-abcdef0123456789-a1b2c3d4"

	// Original owner's 39000 with transfer-to tag.
	store.addEvent(metadataEvent(39000, originalOwner, dtag, 1000,
		nostr.Tag{"transfer-to", successor},
	))

	reject := RejectExternalMetadata("relay-pubkey", store)

	// Original owner publishing 39001 after transfer should be REFUSED.
	event := &nostr.Event{
		Kind:   39001,
		PubKey: originalOwner,
		Tags:   nostr.Tags{nostr.Tag{"d", dtag}},
	}
	blocked, _ := reject(context.Background(), event)
	if !blocked {
		t.Error("original owner should be refused 39001 after transferring ownership")
	}
}

// REQUIRED TEST 4: A pubkey with no grant publishing 39002 is REFUSED.
func TestRejectExternalMetadata_UnauthorizedMemberWriteRefused(t *testing.T) {
	store := newMockStore()
	ownerPubkey := "abcdef0123456789" + "0000111122223333"
	randomPubkey := "eeee000000000000" + "ffff111122223333"
	dtag := "my-space-abcdef0123456789-a1b2c3d4"

	// Owner's 39000 (establishes ownership).
	store.addEvent(metadataEvent(39000, ownerPubkey, dtag, 1000))

	// Owner's 39001 with NO grant for randomPubkey.
	store.addEvent(metadataEvent(39001, ownerPubkey, dtag, 1001,
		nostr.Tag{"p", "someotherpubkey1" + "2222333344445555", "admin", "add-user"},
	))

	reject := RejectExternalMetadata("relay-pubkey", store)

	// Random pubkey publishing 39002 should be REFUSED.
	event := &nostr.Event{
		Kind:   39002,
		PubKey: randomPubkey,
		Tags:   nostr.Tags{nostr.Tag{"d", dtag}},
	}
	blocked, _ := reject(context.Background(), event)
	if !blocked {
		t.Error("pubkey with no grant should be refused 39002")
	}
}

// ============================================================
// Additional store-backed tests
// ============================================================

func TestRejectExternalMetadata_MultiHopTransfer(t *testing.T) {
	store := newMockStore()
	owner1 := "abcdef0123456789" + "0000111122223333"
	owner2 := "bbbb000000000000" + "cccc111122223333"
	owner3 := "dddd000000000000" + "eeee111122223333"
	dtag := "my-space-abcdef0123456789-a1b2c3d4"

	// owner1 -> owner2
	store.addEvent(metadataEvent(39000, owner1, dtag, 1000,
		nostr.Tag{"transfer-to", owner2},
	))
	// owner2 -> owner3
	store.addEvent(metadataEvent(39000, owner2, dtag, 2000,
		nostr.Tag{"transfer-to", owner3},
	))

	reject := RejectExternalMetadata("relay-pubkey", store)

	// owner3 is the current owner.
	event := &nostr.Event{Kind: 39000, PubKey: owner3, Tags: nostr.Tags{nostr.Tag{"d", dtag}}}
	blocked, msg := reject(context.Background(), event)
	if blocked {
		t.Errorf("owner3 (end of transfer chain) should be allowed, got blocked: %s", msg)
	}

	// owner1 and owner2 are no longer owners.
	for _, pk := range []string{owner1, owner2} {
		event := &nostr.Event{Kind: 39000, PubKey: pk, Tags: nostr.Tags{nostr.Tag{"d", dtag}}}
		blocked, _ := reject(context.Background(), event)
		if !blocked {
			t.Errorf("previous owner %s should be refused after transfer chain", pk[:16])
		}
	}
}

func TestRejectExternalMetadata_TransferCycleDetection(t *testing.T) {
	store := newMockStore()
	ownerA := "abcdef0123456789" + "0000111122223333"
	ownerB := "bbbb000000000000" + "cccc111122223333"
	dtag := "my-space-abcdef0123456789-a1b2c3d4"

	// A -> B
	store.addEvent(metadataEvent(39000, ownerA, dtag, 1000,
		nostr.Tag{"transfer-to", ownerB},
	))
	// B -> A (cycle)
	store.addEvent(metadataEvent(39000, ownerB, dtag, 2000,
		nostr.Tag{"transfer-to", ownerA},
	))

	reject := RejectExternalMetadata("relay-pubkey", store)

	// The chain A->B->A stops at B (cycle detected). B is the current owner.
	eventB := &nostr.Event{Kind: 39000, PubKey: ownerB, Tags: nostr.Tags{nostr.Tag{"d", dtag}}}
	blocked, msg := reject(context.Background(), eventB)
	if blocked {
		t.Errorf("ownerB should be current owner (cycle stops here), got blocked: %s", msg)
	}

	eventA := &nostr.Event{Kind: 39000, PubKey: ownerA, Tags: nostr.Tags{nostr.Tag{"d", dtag}}}
	blocked, _ = reject(context.Background(), eventA)
	if !blocked {
		t.Error("ownerA should be refused (transferred to B, cycle does not restore)")
	}
}

func TestRejectExternalMetadata_OwnerCanPublishMemberList(t *testing.T) {
	store := newMockStore()
	ownerPubkey := "abcdef0123456789" + "0000111122223333"
	dtag := "my-space-abcdef0123456789-a1b2c3d4"

	store.addEvent(metadataEvent(39000, ownerPubkey, dtag, 1000))

	reject := RejectExternalMetadata("relay-pubkey", store)

	// Owner should be able to publish 39002 directly.
	event := &nostr.Event{
		Kind:   39002,
		PubKey: ownerPubkey,
		Tags:   nostr.Tags{nostr.Tag{"d", dtag}},
	}
	blocked, msg := reject(context.Background(), event)
	if blocked {
		t.Errorf("owner should be allowed to publish 39002, got blocked: %s", msg)
	}
}

func TestRejectExternalMetadata_GenesisOwnerNoStoredEvents(t *testing.T) {
	store := newMockStore() // empty store

	ownerPubkey := "abcdef0123456789" + "0000111122223333"
	dtag := "my-space-abcdef0123456789-a1b2c3d4"

	reject := RejectExternalMetadata("relay-pubkey", store)

	// Genesis owner (no events in store yet, but prefix matches).
	event := &nostr.Event{
		Kind:   39000,
		PubKey: ownerPubkey,
		Tags:   nostr.Tags{nostr.Tag{"d", dtag}},
	}
	blocked, msg := reject(context.Background(), event)
	if blocked {
		t.Errorf("genesis owner with matching prefix should be allowed even with empty store, got blocked: %s", msg)
	}

	// Non-owner should still be blocked.
	event2 := &nostr.Event{
		Kind:   39000,
		PubKey: "ffff000000000000" + "1111222233334444",
		Tags:   nostr.Tags{nostr.Tag{"d", dtag}},
	}
	blocked, _ = reject(context.Background(), event2)
	if !blocked {
		t.Error("non-owner should be blocked even with empty store")
	}
}

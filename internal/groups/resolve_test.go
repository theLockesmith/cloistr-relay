package groups

import (
	"context"
	"fmt"
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

// metadataEvt builds a test event for the resolver tests.
func metadataEvt(kind int, pubkey string, dTag string, createdAt nostr.Timestamp, extraTags ...nostr.Tag) *nostr.Event {
	tags := nostr.Tags{nostr.Tag{"d", dTag}}
	tags = append(tags, extraTags...)
	return &nostr.Event{
		Kind:      kind,
		PubKey:    pubkey,
		CreatedAt: createdAt,
		Tags:      tags,
		ID:        pubkey[:8] + fmt.Sprintf("-%d-%d", kind, createdAt),
	}
}

// --- ResolveOwner ---

func TestResolveOwner_GenesisOwner(t *testing.T) {
	store := newMockStore()
	owner := "abcdef0123456789" + "0000111122223333"
	dtag := "my-space-abcdef0123456789-a1b2c3d4"

	store.addEvent(metadataEvt(39000, owner, dtag, 1000))

	got, err := ResolveOwner(context.Background(), store, dtag)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != owner {
		t.Errorf("expected owner %s, got %s", owner, got)
	}
}

func TestResolveOwner_LegacyDtag(t *testing.T) {
	store := newMockStore()

	got, err := ResolveOwner(context.Background(), store, "test-project-t9mn5b1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("legacy d-tag should return empty owner, got %q", got)
	}
}

func TestResolveOwner_NoEventsInStore(t *testing.T) {
	store := newMockStore()
	dtag := "my-space-abcdef0123456789-a1b2c3d4"

	got, err := ResolveOwner(context.Background(), store, dtag)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "prefix:abcdef0123456789" {
		t.Errorf("expected prefix:abcdef0123456789, got %q", got)
	}
}

func TestResolveOwner_SingleTransfer(t *testing.T) {
	store := newMockStore()
	owner1 := "abcdef0123456789" + "0000111122223333"
	owner2 := "bbbb000000000000" + "cccc111122223333"
	dtag := "my-space-abcdef0123456789-a1b2c3d4"

	store.addEvent(metadataEvt(39000, owner1, dtag, 1000,
		nostr.Tag{"transfer-to", owner2},
	))

	got, err := ResolveOwner(context.Background(), store, dtag)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != owner2 {
		t.Errorf("expected transferred owner %s, got %s", owner2, got)
	}
}

func TestResolveOwner_MultiHopTransfer(t *testing.T) {
	store := newMockStore()
	o1 := "abcdef0123456789" + "0000111122223333"
	o2 := "bbbb000000000000" + "cccc111122223333"
	o3 := "dddd000000000000" + "eeee111122223333"
	dtag := "my-space-abcdef0123456789-a1b2c3d4"

	store.addEvent(metadataEvt(39000, o1, dtag, 1000, nostr.Tag{"transfer-to", o2}))
	store.addEvent(metadataEvt(39000, o2, dtag, 2000, nostr.Tag{"transfer-to", o3}))

	got, err := ResolveOwner(context.Background(), store, dtag)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != o3 {
		t.Errorf("expected o3 %s, got %s", o3, got)
	}
}

func TestResolveOwner_CycleDetection(t *testing.T) {
	store := newMockStore()
	a := "abcdef0123456789" + "0000111122223333"
	b := "bbbb000000000000" + "cccc111122223333"
	dtag := "my-space-abcdef0123456789-a1b2c3d4"

	store.addEvent(metadataEvt(39000, a, dtag, 1000, nostr.Tag{"transfer-to", b}))
	store.addEvent(metadataEvt(39000, b, dtag, 2000, nostr.Tag{"transfer-to", a}))

	got, err := ResolveOwner(context.Background(), store, dtag)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Chain stops at b (cycle detected before revisiting a).
	if got != b {
		t.Errorf("expected b %s (cycle stops here), got %s", b, got)
	}
}

func TestResolveOwner_NoTransferTag(t *testing.T) {
	store := newMockStore()
	owner := "abcdef0123456789" + "0000111122223333"
	dtag := "my-space-abcdef0123456789-a1b2c3d4"

	// Owner's 39000 with no transfer-to tag.
	store.addEvent(metadataEvt(39000, owner, dtag, 1000))

	got, err := ResolveOwner(context.Background(), store, dtag)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != owner {
		t.Errorf("expected owner %s (no transfer), got %s", owner, got)
	}
}

// --- IsAuthorizedMetadataWriter ---

func TestIsAuthorizedMetadataWriter_OwnerAllKinds(t *testing.T) {
	store := newMockStore()
	owner := "abcdef0123456789" + "0000111122223333"
	dtag := "my-space-abcdef0123456789-a1b2c3d4"

	store.addEvent(metadataEvt(39000, owner, dtag, 1000))

	for _, kind := range []int{39000, 39001, 39002, 39003, 39009} {
		ok, err := IsAuthorizedMetadataWriter(context.Background(), store, dtag, kind, owner)
		if err != nil {
			t.Fatalf("kind %d: unexpected error: %v", kind, err)
		}
		if !ok {
			t.Errorf("kind %d: owner should be authorized", kind)
		}
	}
}

func TestIsAuthorizedMetadataWriter_NonOwnerRefused(t *testing.T) {
	store := newMockStore()
	owner := "abcdef0123456789" + "0000111122223333"
	stranger := "ffff000000000000" + "1111222233334444"
	dtag := "my-space-abcdef0123456789-a1b2c3d4"

	store.addEvent(metadataEvt(39000, owner, dtag, 1000))

	for _, kind := range []int{39000, 39001, 39003} {
		ok, err := IsAuthorizedMetadataWriter(context.Background(), store, dtag, kind, stranger)
		if err != nil {
			t.Fatalf("kind %d: unexpected error: %v", kind, err)
		}
		if ok {
			t.Errorf("kind %d: stranger should be refused", kind)
		}
	}
}

func TestIsAuthorizedMetadataWriter_DelegatedAdmin39002(t *testing.T) {
	store := newMockStore()
	owner := "abcdef0123456789" + "0000111122223333"
	admin := "dddd000000000000" + "aaaa111122223333"
	dtag := "my-space-abcdef0123456789-a1b2c3d4"

	store.addEvent(metadataEvt(39000, owner, dtag, 1000))
	store.addEvent(metadataEvt(39001, owner, dtag, 1001,
		nostr.Tag{"p", admin, "admin", "add-user", "remove-user"},
	))

	ok, err := IsAuthorizedMetadataWriter(context.Background(), store, dtag, 39002, admin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("delegated admin with add-user should be authorized for 39002")
	}
}

func TestIsAuthorizedMetadataWriter_DelegatedAdminRefused39001(t *testing.T) {
	store := newMockStore()
	owner := "abcdef0123456789" + "0000111122223333"
	admin := "dddd000000000000" + "aaaa111122223333"
	dtag := "my-space-abcdef0123456789-a1b2c3d4"

	store.addEvent(metadataEvt(39000, owner, dtag, 1000))
	store.addEvent(metadataEvt(39001, owner, dtag, 1001,
		nostr.Tag{"p", admin, "admin", "add-user", "remove-user"},
	))

	// Delegated admin should NOT be allowed to publish 39001 (admin list).
	ok, err := IsAuthorizedMetadataWriter(context.Background(), store, dtag, 39001, admin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("delegated admin should NOT be authorized for 39001 (admin list is owner-only)")
	}
}

func TestIsAuthorizedMetadataWriter_NoDelegation(t *testing.T) {
	store := newMockStore()
	owner := "abcdef0123456789" + "0000111122223333"
	random := "eeee000000000000" + "ffff111122223333"
	dtag := "my-space-abcdef0123456789-a1b2c3d4"

	store.addEvent(metadataEvt(39000, owner, dtag, 1000))
	// No 39001 at all.

	ok, err := IsAuthorizedMetadataWriter(context.Background(), store, dtag, 39002, random)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("random pubkey should be refused for 39002 with no admin list")
	}
}

func TestIsAuthorizedMetadataWriter_LegacyAllowsAnyone(t *testing.T) {
	store := newMockStore()

	ok, err := IsAuthorizedMetadataWriter(context.Background(), store, "test-77qwa6p", 39000, "anyone")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("legacy d-tag should allow anyone through")
	}
}

func TestIsAuthorizedMetadataWriter_GenesisPrefix(t *testing.T) {
	store := newMockStore() // empty
	owner := "abcdef0123456789" + "0000111122223333"
	stranger := "ffff000000000000" + "1111222233334444"
	dtag := "my-space-abcdef0123456789-a1b2c3d4"

	ok, err := IsAuthorizedMetadataWriter(context.Background(), store, dtag, 39000, owner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("genesis owner (prefix match, no store events) should be authorized")
	}

	ok, err = IsAuthorizedMetadataWriter(context.Background(), store, dtag, 39000, stranger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("stranger should be refused even with empty store")
	}
}

// --- latestEventFrom ---

func TestLatestEventFrom_SortsByCreatedAtDesc(t *testing.T) {
	pk := "abcdef0123456789" + "0000111122223333"
	events := []*nostr.Event{
		{PubKey: pk, CreatedAt: 100, ID: "aaa"},
		{PubKey: pk, CreatedAt: 300, ID: "bbb"},
		{PubKey: pk, CreatedAt: 200, ID: "ccc"},
	}
	got := latestEventFrom(events, pk)
	if got.ID != "bbb" {
		t.Errorf("expected event bbb (highest created_at), got %s", got.ID)
	}
}

func TestLatestEventFrom_TiebreakByID(t *testing.T) {
	pk := "abcdef0123456789" + "0000111122223333"
	events := []*nostr.Event{
		{PubKey: pk, CreatedAt: 100, ID: "aaa"},
		{PubKey: pk, CreatedAt: 100, ID: "zzz"},
		{PubKey: pk, CreatedAt: 100, ID: "mmm"},
	}
	got := latestEventFrom(events, pk)
	if got.ID != "zzz" {
		t.Errorf("expected event zzz (highest ID tiebreak), got %s", got.ID)
	}
}

func TestLatestEventFrom_NoCandidates(t *testing.T) {
	events := []*nostr.Event{
		{PubKey: "other", CreatedAt: 100, ID: "aaa"},
	}
	got := latestEventFrom(events, "nobody")
	if got != nil {
		t.Errorf("expected nil for no candidates, got %v", got)
	}
}

// --- getTagValue ---

func TestGetTagValue_Found(t *testing.T) {
	evt := &nostr.Event{
		Tags: nostr.Tags{
			nostr.Tag{"d", "my-group"},
			nostr.Tag{"transfer-to", "successor-pk"},
		},
	}
	got := getTagValue(evt, "transfer-to")
	if got != "successor-pk" {
		t.Errorf("expected successor-pk, got %q", got)
	}
}

func TestGetTagValue_NotFound(t *testing.T) {
	evt := &nostr.Event{
		Tags: nostr.Tags{nostr.Tag{"d", "my-group"}},
	}
	got := getTagValue(evt, "transfer-to")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestGetTagValue_ShortTag(t *testing.T) {
	evt := &nostr.Event{
		Tags: nostr.Tags{nostr.Tag{"d"}}, // only key, no value
	}
	got := getTagValue(evt, "d")
	if got != "" {
		t.Errorf("expected empty for single-element tag, got %q", got)
	}
}

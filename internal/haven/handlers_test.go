package haven

import (
	"context"
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

// Note: Tests requiring authenticated context use khatru.GetAuthed() which
// relies on WebSocket connection state that cannot be easily mocked in unit tests.
// These tests verify behavior with unauthenticated context, or skip tests that
// require authentication. Full authentication flow should be tested in integration tests.

// TestRejectEvent_Unauthenticated_PrivateBox tests unauthenticated access to private box
func TestRejectEvent_Unauthenticated_PrivateBox(t *testing.T) {
	cfg := &Config{
		Enabled:               true,
		OwnerPubkey:           ownerPubkey,
		RequireAuthForPrivate: true,
	}
	handler := NewHandler(cfg)
	rejectFn := handler.RejectEvent()

	event := &nostr.Event{
		PubKey: ownerPubkey,
		Kind:   30024, // Private kind
		Tags:   nostr.Tags{},
	}

	ctx := context.Background() // Unauthenticated

	reject, reason := rejectFn(ctx, event)
	if !reject {
		t.Error("Unauthenticated access to private box should be rejected")
	}
	if reason != "auth-required: authentication required for private box" {
		t.Errorf("Wrong rejection reason: got %q", reason)
	}
}

// TestRejectEvent_NonOwner_PrivateKind tests non-owner attempting private kinds
func TestRejectEvent_NonOwner_PrivateKind(t *testing.T) {
	t.Skip("Requires khatru authenticated context - test in integration tests")
}

// TestRejectEvent_Owner_PrivateKind tests owner accessing private kinds
func TestRejectEvent_Owner_PrivateKind(t *testing.T) {
	t.Skip("Requires khatru authenticated context - test in integration tests")
}

// TestRejectEvent_Unauthenticated_ChatBox tests unauthenticated access to chat
func TestRejectEvent_Unauthenticated_ChatBox_AuthRequired(t *testing.T) {
	cfg := &Config{
		Enabled:            true,
		OwnerPubkey:        ownerPubkey,
		RequireAuthForChat: true,
	}
	handler := NewHandler(cfg)
	rejectFn := handler.RejectEvent()

	event := &nostr.Event{
		PubKey: alicePubkey,
		Kind:   4, // DM kind
		Tags:   nostr.Tags{},
	}

	ctx := context.Background()

	reject, reason := rejectFn(ctx, event)
	if !reject {
		t.Error("Unauthenticated access to chat (with auth required) should be rejected")
	}
	if reason != "auth-required: authentication required for chat" {
		t.Errorf("Wrong rejection reason: got %q", reason)
	}
}

// TestRejectEvent_Unauthenticated_ChatBox_NoAuthRequired tests chat without auth requirement
func TestRejectEvent_Unauthenticated_ChatBox_NoAuthRequired(t *testing.T) {
	cfg := &Config{
		Enabled:            true,
		OwnerPubkey:        ownerPubkey,
		RequireAuthForChat: false,
	}
	handler := NewHandler(cfg)
	rejectFn := handler.RejectEvent()

	event := &nostr.Event{
		PubKey: alicePubkey,
		Kind:   4,
		Tags:   nostr.Tags{},
	}

	ctx := context.Background()

	reject, _ := rejectFn(ctx, event)
	if reject {
		t.Error("Chat access without auth requirement should not reject unauthenticated users")
	}
}

// TestRejectEvent_Authenticated_ChatBox tests authenticated chat access
func TestRejectEvent_Authenticated_ChatBox(t *testing.T) {
	t.Skip("Requires khatru authenticated context - test in integration tests")
}

// TestRejectEvent_Unauthenticated_InboxWrite tests public inbox write
func TestRejectEvent_Unauthenticated_InboxWrite(t *testing.T) {
	cfg := &Config{
		Enabled:               true,
		OwnerPubkey:           ownerPubkey,
		AllowPublicInboxWrite: true,
	}
	handler := NewHandler(cfg)
	rejectFn := handler.RejectEvent()

	// Event addressed to owner
	event := &nostr.Event{
		PubKey: alicePubkey,
		Kind:   1,
		Tags: nostr.Tags{
			{"p", ownerPubkey},
		},
	}

	ctx := context.Background()

	reject, _ := rejectFn(ctx, event)
	if reject {
		t.Error("Public inbox write should not be rejected")
	}
}

// TestRejectEvent_UnknownBox_Unauthenticated tests events that don't belong to any box
func TestRejectEvent_UnknownBox_Unauthenticated(t *testing.T) {
	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	handler := NewHandler(cfg)
	rejectFn := handler.RejectEvent()

	// Event from non-owner without addressing owner
	event := &nostr.Event{
		PubKey: alicePubkey,
		Kind:   1,
		Tags:   nostr.Tags{}, // No p-tag to owner
	}

	ctx := context.Background()

	reject, reason := rejectFn(ctx, event)
	if !reject {
		t.Error("Event not belonging to any box should be rejected")
	}
	if reason != "restricted: event does not belong to any HAVEN box" {
		t.Errorf("Wrong rejection reason: got %q", reason)
	}
}

// TestRejectEvent_DisabledHaven tests that disabled HAVEN doesn't reject
func TestRejectEvent_DisabledHaven(t *testing.T) {
	cfg := &Config{
		Enabled:     false,
		OwnerPubkey: ownerPubkey,
	}
	handler := NewHandler(cfg)
	rejectFn := handler.RejectEvent()

	event := &nostr.Event{
		PubKey: alicePubkey,
		Kind:   1,
		Tags:   nostr.Tags{},
	}

	ctx := context.Background()

	reject, _ := rejectFn(ctx, event)
	if reject {
		t.Error("Disabled HAVEN should not reject events")
	}
}

// TestRejectFilter_Unauthenticated_PrivateBox tests unauthenticated filter for private kinds
func TestRejectFilter_Unauthenticated_PrivateBox(t *testing.T) {
	cfg := &Config{
		Enabled:               true,
		OwnerPubkey:           ownerPubkey,
		RequireAuthForPrivate: true,
	}
	handler := NewHandler(cfg)
	rejectFn := handler.RejectFilter()

	filter := nostr.Filter{
		Kinds: []int{30024, 7375}, // Private kinds
	}

	ctx := context.Background()

	reject, reason := rejectFn(ctx, filter)
	if !reject {
		t.Error("Unauthenticated filter for private box should be rejected")
	}
	if reason != "auth-required: authentication required for private box" {
		t.Errorf("Wrong rejection reason: got %q", reason)
	}
}

// TestRejectFilter_Authenticated_PrivateBox tests authenticated filter access
func TestRejectFilter_Authenticated_PrivateBox(t *testing.T) {
	t.Skip("Requires khatru authenticated context - test in integration tests")
}

// TestRejectFilter_Unauthenticated_ChatBox tests unauthenticated chat filter
func TestRejectFilter_Unauthenticated_ChatBox_AuthRequired(t *testing.T) {
	cfg := &Config{
		Enabled:            true,
		OwnerPubkey:        ownerPubkey,
		RequireAuthForChat: true,
	}
	handler := NewHandler(cfg)
	rejectFn := handler.RejectFilter()

	filter := nostr.Filter{
		Kinds: []int{4, 1059}, // Chat kinds
	}

	ctx := context.Background()

	reject, reason := rejectFn(ctx, filter)
	if !reject {
		t.Error("Unauthenticated chat filter (auth required) should be rejected")
	}
	if reason != "auth-required: authentication required for chat" {
		t.Errorf("Wrong rejection reason: got %q", reason)
	}
}

// TestRejectFilter_Unauthenticated_ChatBox_NoAuth tests chat without auth requirement
func TestRejectFilter_Unauthenticated_ChatBox_NoAuth(t *testing.T) {
	cfg := &Config{
		Enabled:            true,
		OwnerPubkey:        ownerPubkey,
		RequireAuthForChat: false,
	}
	handler := NewHandler(cfg)
	rejectFn := handler.RejectFilter()

	filter := nostr.Filter{
		Kinds: []int{4, 1059},
	}

	ctx := context.Background()

	reject, _ := rejectFn(ctx, filter)
	if reject {
		t.Error("Chat filter without auth requirement should not reject")
	}
}

// TestRejectFilter_Unauthenticated_InboxRead tests unauthenticated inbox read
func TestRejectFilter_Unauthenticated_InboxRead(t *testing.T) {
	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	handler := NewHandler(cfg)
	rejectFn := handler.RejectFilter()

	filter := nostr.Filter{
		Tags: nostr.TagMap{
			"p": []string{ownerPubkey},
		},
	}

	ctx := context.Background()

	reject, reason := rejectFn(ctx, filter)
	if !reject {
		t.Error("Unauthenticated inbox read should be rejected")
	}
	if reason != "restricted: only owner can read inbox" {
		t.Errorf("Wrong rejection reason: got %q", reason)
	}
}

// TestRejectFilter_Unauthenticated_OutboxRead tests public outbox read
func TestRejectFilter_Unauthenticated_OutboxRead(t *testing.T) {
	cfg := &Config{
		Enabled:               true,
		OwnerPubkey:           ownerPubkey,
		AllowPublicOutboxRead: true,
	}
	handler := NewHandler(cfg)
	rejectFn := handler.RejectFilter()

	filter := nostr.Filter{
		Authors: []string{ownerPubkey},
	}

	ctx := context.Background()

	reject, _ := rejectFn(ctx, filter)
	if reject {
		t.Error("Public outbox read should not be rejected")
	}
}

// TestRejectFilter_DisabledHaven tests disabled HAVEN filter behavior
func TestRejectFilter_DisabledHaven(t *testing.T) {
	cfg := &Config{
		Enabled:     false,
		OwnerPubkey: ownerPubkey,
	}
	handler := NewHandler(cfg)
	rejectFn := handler.RejectFilter()

	filter := nostr.Filter{
		Kinds: []int{30024}, // Private kind
	}

	ctx := context.Background()

	reject, _ := rejectFn(ctx, filter)
	if reject {
		t.Error("Disabled HAVEN should not reject filters")
	}
}

// TestOverwriteFilter_Unauthenticated_RemovePrivateKinds tests private kind removal
func TestOverwriteFilter_Unauthenticated_RemovePrivateKinds(t *testing.T) {
	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	handler := NewHandler(cfg)
	overwriteFn := handler.OverwriteFilter()

	tests := []struct {
		name          string
		inputKinds    []int
		expectedKinds []int
	}{
		{
			name:          "mixed kinds - private removed",
			inputKinds:    []int{1, 30024, 7, 7375},
			expectedKinds: []int{1, 7},
		},
		{
			name:          "all private kinds - all removed",
			inputKinds:    []int{30024, 7375, 7376},
			expectedKinds: []int{},
		},
		{
			name:          "no private kinds - unchanged",
			inputKinds:    []int{1, 6, 7},
			expectedKinds: []int{1, 6, 7},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := &nostr.Filter{
				Kinds: tt.inputKinds,
			}

			ctx := context.Background() // Unauthenticated

			overwriteFn(ctx, filter)

			if len(filter.Kinds) != len(tt.expectedKinds) {
				t.Errorf("OverwriteFilter() kinds = %v, want %v", filter.Kinds, tt.expectedKinds)
				return
			}

			for i, kind := range filter.Kinds {
				if kind != tt.expectedKinds[i] {
					t.Errorf("OverwriteFilter() kinds[%d] = %d, want %d", i, kind, tt.expectedKinds[i])
				}
			}
		})
	}
}

// TestOverwriteFilter_Owner tests owner filter behavior
func TestOverwriteFilter_Owner(t *testing.T) {
	t.Skip("Requires khatru authenticated context - test in integration tests")
}

// TestOverwriteFilter_EmptyKinds tests behavior with no kinds filter
func TestOverwriteFilter_EmptyKinds(t *testing.T) {
	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	handler := NewHandler(cfg)
	overwriteFn := handler.OverwriteFilter()

	filter := &nostr.Filter{
		Authors: []string{alicePubkey},
	}

	ctx := context.Background()
	overwriteFn(ctx, filter)

	if filter.Kinds != nil {
		t.Error("OverwriteFilter() should not add kinds when none specified")
	}
}

// TestOverwriteFilter_DisabledHaven tests disabled HAVEN filter behavior
func TestOverwriteFilter_DisabledHaven(t *testing.T) {
	cfg := &Config{
		Enabled:     false,
		OwnerPubkey: ownerPubkey,
	}
	handler := NewHandler(cfg)
	overwriteFn := handler.OverwriteFilter()

	filter := &nostr.Filter{
		Kinds: []int{1, 30024, 7375},
	}
	originalKinds := make([]int, len(filter.Kinds))
	copy(originalKinds, filter.Kinds)

	ctx := context.Background()
	overwriteFn(ctx, filter)

	if len(filter.Kinds) != len(originalKinds) {
		t.Error("OverwriteFilter() with disabled HAVEN should not modify filter")
	}
}

// TestOnEventSaved tests the OnEventSaved handler (logging only)
func TestOnEventSaved(t *testing.T) {
	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	handler := NewHandler(cfg)
	onSavedFn := handler.OnEventSaved()

	event := &nostr.Event{
		ID:     "test123456789",
		PubKey: ownerPubkey,
		Kind:   1,
		Tags:   nostr.Tags{},
	}

	ctx := context.Background()

	// Should not panic
	onSavedFn(ctx, event)
}

// TestOnEventSaved_DisabledHaven tests OnEventSaved with disabled HAVEN
func TestOnEventSaved_DisabledHaven(t *testing.T) {
	cfg := &Config{
		Enabled:     false,
		OwnerPubkey: ownerPubkey,
	}
	handler := NewHandler(cfg)
	onSavedFn := handler.OnEventSaved()

	event := &nostr.Event{
		ID:     "test123456789",
		PubKey: ownerPubkey,
		Kind:   1,
		Tags:   nostr.Tags{},
	}

	ctx := context.Background()

	// Should not panic
	onSavedFn(ctx, event)
}

// TestNewHandler_NilConfig tests handler creation with nil config
func TestNewHandler_NilConfig(t *testing.T) {
	handler := NewHandler(nil)
	if handler == nil {
		t.Error("NewHandler(nil) should return a valid handler")
	}
	if handler.config == nil {
		t.Error("NewHandler(nil) should use DefaultConfig")
	}
}

// TestBoxPolicies tests box policy generation
func TestBoxPolicies(t *testing.T) {
	cfg := &Config{
		Enabled:               true,
		OwnerPubkey:           ownerPubkey,
		AllowPublicOutboxRead: true,
		AllowPublicInboxWrite: true,
		RequireAuthForChat:    true,
		RequireAuthForPrivate: true,
	}

	policies := BoxPolicies(cfg)

	// Test private box policy
	privatePolicy := policies[BoxPrivate]
	if !privatePolicy.OwnerOnly {
		t.Error("Private box should be owner-only")
	}
	if !privatePolicy.ReadRequiresAuth {
		t.Error("Private box should require auth for read")
	}
	if !privatePolicy.WriteRequiresAuth {
		t.Error("Private box should require auth for write")
	}

	// Test chat box policy
	chatPolicy := policies[BoxChat]
	if chatPolicy.OwnerOnly {
		t.Error("Chat box should not be owner-only")
	}
	if !chatPolicy.WoTFiltered {
		t.Error("Chat box should be WoT filtered")
	}

	// Test inbox box policy
	inboxPolicy := policies[BoxInbox]
	if !inboxPolicy.ReadRequiresAuth {
		t.Error("Inbox box read should require auth (owner only)")
	}
	if inboxPolicy.WriteRequiresAuth {
		t.Error("Inbox box should allow public write when configured")
	}

	// Test outbox box policy
	outboxPolicy := policies[BoxOutbox]
	if outboxPolicy.ReadRequiresAuth {
		t.Error("Outbox box should allow public read when configured")
	}
	if !outboxPolicy.WriteRequiresAuth {
		t.Error("Outbox box should require auth for write")
	}
}

// TestBoxPolicies_RestrictedConfig tests restrictive configuration
func TestBoxPolicies_RestrictedConfig(t *testing.T) {
	cfg := &Config{
		Enabled:               true,
		OwnerPubkey:           ownerPubkey,
		AllowPublicOutboxRead: false,
		AllowPublicInboxWrite: false,
		RequireAuthForChat:    true,
		RequireAuthForPrivate: true,
	}

	policies := BoxPolicies(cfg)

	if !policies[BoxInbox].WriteRequiresAuth {
		t.Error("Inbox should require auth for write when public write disabled")
	}
	if !policies[BoxOutbox].ReadRequiresAuth {
		t.Error("Outbox should require auth for read when public read disabled")
	}
}

// TestBoxStats_String tests BoxStats string representation
func TestBoxStats_String(t *testing.T) {
	stats := BoxStats{
		Private: 10,
		Chat:    25,
		Inbox:   100,
		Outbox:  500,
	}

	expected := "private=10 chat=25 inbox=100 outbox=500"
	if stats.String() != expected {
		t.Errorf("BoxStats.String() = %q, want %q", stats.String(), expected)
	}
}

// TestBoxStats_ZeroValues tests BoxStats with zero values
func TestBoxStats_ZeroValues(t *testing.T) {
	stats := BoxStats{}

	expected := "private=0 chat=0 inbox=0 outbox=0"
	if stats.String() != expected {
		t.Errorf("BoxStats.String() = %q, want %q", stats.String(), expected)
	}
}

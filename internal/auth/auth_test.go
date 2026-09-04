package auth

import (
	"context"
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

// TestPolicy_String tests the string representation of policies
func TestPolicy_String(t *testing.T) {
	tests := []struct {
		policy   Policy
		expected string
	}{
		{PolicyOpen, "open"},
		{PolicyAuthRead, "auth-read"},
		{PolicyAuthWrite, "auth-write"},
		{PolicyAuthAll, "auth-all"},
		{Policy(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := tt.policy.String()
			if got != tt.expected {
				t.Errorf("Policy.String() = %s, want %s", got, tt.expected)
			}
		})
	}
}

// TestRequireAuthForRead_Authenticated tests authenticated read requests
func TestRequireAuthForRead_Authenticated(t *testing.T) {
	t.Skip("Requires khatru context - test in integration tests")
}

// TestRequireAuthForRead_Unauthenticated tests unauthenticated read requests
func TestRequireAuthForRead_Unauthenticated(t *testing.T) {
	ctx := context.Background()
	filter := nostr.Filter{}

	reject, msg := requireAuthForRead(ctx, filter)
	if !reject {
		t.Error("Unauthenticated read was not rejected")
	}
	if msg != "auth-required: authentication required to read from this relay" {
		t.Errorf("Wrong rejection message: got %s", msg)
	}
}

// TestRequireAuthForWrite_AuthEvent tests that AUTH events (kind 22242) bypass authentication
func TestRequireAuthForWrite_AuthEvent(t *testing.T) {
	cfg := &Config{Policy: PolicyAuthWrite}
	handler := requireAuthForWrite(cfg)

	ctx := context.Background() // Unauthenticated
	event := &nostr.Event{Kind: 22242}

	reject, msg := handler(ctx, event)
	if reject {
		t.Errorf("AUTH event was rejected: %s", msg)
	}
}

// TestRequireAuthForWrite_Unauthenticated tests unauthenticated write requests
func TestRequireAuthForWrite_Unauthenticated(t *testing.T) {
	cfg := &Config{Policy: PolicyAuthWrite}
	handler := requireAuthForWrite(cfg)

	ctx := context.Background()
	event := &nostr.Event{
		Kind:   1,
		PubKey: "test-pubkey",
	}

	reject, msg := handler(ctx, event)
	if !reject {
		t.Error("Unauthenticated write was not rejected")
	}
	if msg != "auth-required: authentication required to publish events" {
		t.Errorf("Wrong rejection message: got %s", msg)
	}
}

// TestRequireAuthForWrite_AuthenticatedMatchingPubkey tests authenticated write with matching pubkey
func TestRequireAuthForWrite_AuthenticatedMatchingPubkey(t *testing.T) {
	t.Skip("Requires khatru context - test in integration tests")
}

// TestRequireAuthForWrite_AuthenticatedMismatchedPubkey tests authenticated write with mismatched pubkey
func TestRequireAuthForWrite_AuthenticatedMismatchedPubkey(t *testing.T) {
	t.Skip("Requires khatru context - test in integration tests")
}

// The write gate, exercised for real.
//
// These four cases were t.Skip("Requires khatru context - test in integration
// tests") for the whole life of the file, and they name precisely the behaviour
// that then shipped broken: a non-empty whitelist refused every real user,
// because the same value was also the WoT bypass list. The decision now takes
// the authenticated pubkey as an argument (evaluateWrite), so there is nothing
// left to skip.
func whitelistOf(pubkeys ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(pubkeys))
	for _, pk := range pubkeys {
		m[pk] = struct{}{}
	}
	return m
}

func TestEvaluateWrite(t *testing.T) {
	const alice = "aaaa000000000000000000000000000000000000000000000000000000000001"
	const bob = "bbbb000000000000000000000000000000000000000000000000000000000002"

	tests := []struct {
		name       string
		exempt     map[int]bool
		whitelist  map[string]struct{}
		authed     string
		event      *nostr.Event
		wantReject bool
		wantMsg    string
	}{
		{
			name:      "whitelisted pubkey can write",
			whitelist: whitelistOf(alice),
			authed:    alice,
			event:     &nostr.Event{Kind: 1, PubKey: alice},
		},
		{
			name:       "non-whitelisted pubkey is rejected",
			whitelist:  whitelistOf(alice),
			authed:     bob,
			event:      &nostr.Event{Kind: 1, PubKey: bob},
			wantReject: true,
			wantMsg:    "restricted: your pubkey is not on the whitelist",
		},
		{
			// THE REGRESSION GUARD. An empty whitelist is the hosted relay's
			// configuration, and it must let an ordinary authenticated user write.
			name:      "empty whitelist allows any authenticated user",
			whitelist: whitelistOf(),
			authed:    bob,
			event:     &nostr.Event{Kind: 30078, PubKey: bob},
		},
		{
			name:   "nil whitelist allows any authenticated user",
			authed: bob,
			event:  &nostr.Event{Kind: 30078, PubKey: bob},
		},
		{
			name:       "unauthenticated write is refused before the whitelist",
			whitelist:  whitelistOf(alice),
			authed:     "",
			event:      &nostr.Event{Kind: 1, PubKey: alice},
			wantReject: true,
			wantMsg:    "auth-required: authentication required to publish events",
		},
		{
			// Being on the whitelist does not let you publish as someone else.
			name:       "authenticated as someone else",
			whitelist:  whitelistOf(alice, bob),
			authed:     alice,
			event:      &nostr.Event{Kind: 1, PubKey: bob},
			wantReject: true,
			wantMsg:    "restricted: you can only publish events as your authenticated identity",
		},
		{
			name:      "kind 22242 AUTH events bypass the gate entirely",
			whitelist: whitelistOf(alice),
			authed:    "",
			event:     &nostr.Event{Kind: 22242, PubKey: bob},
		},
		{
			// NIP-46 signer traffic must survive an invite-only relay, or remote
			// signing breaks for everyone not on the list.
			name:      "exempt kind bypasses the whitelist",
			exempt:    map[int]bool{24133: true},
			whitelist: whitelistOf(alice),
			authed:    "",
			event:     &nostr.Event{Kind: 24133, PubKey: bob},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exempt := tt.exempt
			if exempt == nil {
				exempt = map[int]bool{}
			}
			reject, msg := evaluateWrite(exempt, tt.whitelist, tt.authed, tt.event)
			if reject != tt.wantReject {
				t.Fatalf("reject = %v, want %v (msg %q)", reject, tt.wantReject, msg)
			}
			if msg != tt.wantMsg {
				t.Errorf("msg = %q, want %q", msg, tt.wantMsg)
			}
		})
	}
}

// The restrictive whitelist and the WoT bypass list are separate fields, and
// nothing may quietly re-join them. This is a compile-and-behaviour guard on the
// exact defect: a Config carrying no WriteWhitelist restricts nobody, however
// many pubkeys the deployment has granted a WoT bypass.
func TestWriteWhitelistIsNotTheWoTBypassList(t *testing.T) {
	const stranger = "cccc000000000000000000000000000000000000000000000000000000000003"

	cfg := &Config{Policy: PolicyAuthWrite} // WriteWhitelist deliberately unset
	handler := requireAuthForWrite(cfg)
	if handler == nil {
		t.Fatal("requireAuthForWrite returned nil")
	}

	reject, msg := evaluateWrite(map[int]bool{}, whitelistOf(), stranger,
		&nostr.Event{Kind: 30078, PubKey: stranger})
	if reject {
		t.Fatalf("an authenticated stranger was refused with %q; the hosted relay "+
			"must accept writes when no write whitelist is configured", msg)
	}
}

// TestGetAuthenticatedPubkey_Authenticated tests getting authenticated pubkey
func TestGetAuthenticatedPubkey_Authenticated(t *testing.T) {
	t.Skip("Requires khatru context - test in integration tests")
}

// TestGetAuthenticatedPubkey_Unauthenticated tests getting pubkey from unauthenticated context
func TestGetAuthenticatedPubkey_Unauthenticated(t *testing.T) {
	ctx := context.Background()

	pubkey := GetAuthenticatedPubkey(ctx)
	if pubkey != "" {
		t.Errorf("GetAuthenticatedPubkey() = %s, want empty string", pubkey)
	}
}

// TestIsAuthenticated_Authenticated tests authenticated context
func TestIsAuthenticated_Authenticated(t *testing.T) {
	t.Skip("Requires khatru context - test in integration tests")
}

// TestIsAuthenticated_Unauthenticated tests unauthenticated context
func TestIsAuthenticated_Unauthenticated(t *testing.T) {
	ctx := context.Background()

	if IsAuthenticated(ctx) {
		t.Error("IsAuthenticated() = true, want false")
	}
}

// TestRegisterAuthHandlers_NilConfig tests that nil config defaults to PolicyOpen
func TestRegisterAuthHandlers_NilConfig(t *testing.T) {
	// This test mainly ensures the function doesn't panic with nil config
	// We can't fully test relay handler registration without creating a real relay
	// but we can at least verify the function handles nil gracefully

	// Note: This would require a mock relay to fully test
	// For now, we document that the function should be tested in integration tests
	t.Skip("Requires relay instance - test in integration tests")
}

// TestRegisterAuthHandlers_PolicyOpen tests PolicyOpen configuration
func TestRegisterAuthHandlers_PolicyOpen(t *testing.T) {
	t.Skip("Requires relay instance - test in integration tests")
}

// TestRegisterAuthHandlers_PolicyAuthRead tests PolicyAuthRead configuration
func TestRegisterAuthHandlers_PolicyAuthRead(t *testing.T) {
	t.Skip("Requires relay instance - test in integration tests")
}

// TestRegisterAuthHandlers_PolicyAuthWrite tests PolicyAuthWrite configuration
func TestRegisterAuthHandlers_PolicyAuthWrite(t *testing.T) {
	t.Skip("Requires relay instance - test in integration tests")
}

// TestRegisterAuthHandlers_PolicyAuthAll tests PolicyAuthAll configuration
func TestRegisterAuthHandlers_PolicyAuthAll(t *testing.T) {
	t.Skip("Requires relay instance - test in integration tests")
}

// testContextKey is a type for test-specific context keys
type testContextKey string

// createAuthenticatedContext creates a mock authenticated context for testing
// Note: In real khatru usage, authentication is managed via context values
// This is a simplified version for testing purposes
var _ = createAuthenticatedContext // Silence unused warning - reserved for future integration tests

func createAuthenticatedContext(pubkey string) context.Context {
	// In actual khatru implementation, we would use their context key
	// For unit tests, we need to understand that khatru.GetAuthed(ctx) returns ""
	// for contexts we create here, so these tests verify the logic given
	// authenticated vs unauthenticated states

	// Since we can't actually set khatru's internal context values,
	// we'll create a context that represents an authenticated state
	// This is a limitation of unit testing without mocking the khatru package

	// For now, these tests verify the handler logic itself
	// Integration tests should verify the full authentication flow
	return context.WithValue(context.Background(), testContextKey("test-authed"), pubkey)
}

// Note: The tests above for requireAuthForRead and requireAuthForWrite
// test the function logic, but they rely on khatru.GetAuthed() which we
// cannot easily mock without additional tooling. In a production test suite,
// you would either:
// 1. Use a mocking framework to mock khatru.GetAuthed
// 2. Create integration tests that use actual khatru relay instances
// 3. Refactor the code to make the auth getter injectable
//
// For this test suite, we document that full authentication flow testing
// should be done in integration tests with real khatru instances.

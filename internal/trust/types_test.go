package trust

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

func TestKindConstants(t *testing.T) {
	if KindTrustedProviders != 10040 {
		t.Errorf("KindTrustedProviders = %d, want 10040", KindTrustedProviders)
	}
	if KindTrustAssertion != 30040 {
		t.Errorf("KindTrustAssertion = %d, want 30040", KindTrustAssertion)
	}
}

func TestIsTrustKind(t *testing.T) {
	if !IsTrustKind(KindTrustedProviders) {
		t.Error("IsTrustKind(KindTrustedProviders) = false, want true")
	}
	if !IsTrustKind(KindTrustAssertion) {
		t.Error("IsTrustKind(KindTrustAssertion) = false, want true")
	}
	if IsTrustKind(1) {
		t.Error("IsTrustKind(1) = true, want false")
	}
}

func TestParseTrustedProviders(t *testing.T) {
	event := &nostr.Event{
		Kind: KindTrustedProviders,
		Tags: nostr.Tags{
			{"p", "provider1", "wss://relay.example.com"},
			{"p", "provider2"},
			{"e", "someevent"}, // Not a p-tag
		},
	}

	providers := ParseTrustedProviders(event)

	if len(providers) != 2 {
		t.Errorf("Providers count = %d, want 2", len(providers))
	}

	if providers[0].Pubkey != "provider1" {
		t.Errorf("Provider[0].Pubkey = %s, want provider1", providers[0].Pubkey)
	}

	if providers[0].RelayHint != "wss://relay.example.com" {
		t.Errorf("Provider[0].RelayHint = %s, want wss://relay.example.com", providers[0].RelayHint)
	}

	if providers[1].RelayHint != "" {
		t.Errorf("Provider[1].RelayHint = %s, want empty", providers[1].RelayHint)
	}
}

func TestParseTrustedProvidersWrongKind(t *testing.T) {
	event := &nostr.Event{Kind: 1}
	if ParseTrustedProviders(event) != nil {
		t.Error("Should return nil for wrong kind")
	}
}

func TestCreateTrustedProvidersEvent(t *testing.T) {
	providers := []TrustProvider{
		{Pubkey: "provider1", RelayHint: "wss://relay.example.com"},
		{Pubkey: "provider2"},
	}

	event := CreateTrustedProvidersEvent("userpubkey", providers)

	if event.Kind != KindTrustedProviders {
		t.Errorf("Kind = %d, want %d", event.Kind, KindTrustedProviders)
	}

	if event.PubKey != "userpubkey" {
		t.Errorf("PubKey = %s, want userpubkey", event.PubKey)
	}

	if len(event.Tags) != 2 {
		t.Errorf("Tags count = %d, want 2", len(event.Tags))
	}

	// First tag should have relay hint
	if len(event.Tags[0]) != 3 || event.Tags[0][2] != "wss://relay.example.com" {
		t.Errorf("Tag[0] = %v, expected relay hint", event.Tags[0])
	}

	// Second tag should not have relay hint
	if len(event.Tags[1]) != 2 {
		t.Errorf("Tag[1] = %v, expected no relay hint", event.Tags[1])
	}
}

func TestParseTrustAssertion(t *testing.T) {
	event := &nostr.Event{
		Kind:   KindTrustAssertion,
		PubKey: "providerpubkey",
		Tags: nostr.Tags{
			{"d", "spam"},
			{"p", "targetpubkey"},
			{"e", "targetevent"},
			{"assertion", "spam"},
			{"reason", "Known spammer"},
		},
	}

	assertion := ParseTrustAssertion(event)

	if assertion == nil {
		t.Fatal("ParseTrustAssertion returned nil")
	}

	if assertion.ProviderPubkey != "providerpubkey" {
		t.Errorf("ProviderPubkey = %s, want providerpubkey", assertion.ProviderPubkey)
	}

	if assertion.TargetPubkey != "targetpubkey" {
		t.Errorf("TargetPubkey = %s, want targetpubkey", assertion.TargetPubkey)
	}

	if assertion.TargetEventID != "targetevent" {
		t.Errorf("TargetEventID = %s, want targetevent", assertion.TargetEventID)
	}

	if assertion.AssertionType != "spam" {
		t.Errorf("AssertionType = %s, want spam", assertion.AssertionType)
	}

	if assertion.Reason != "Known spammer" {
		t.Errorf("Reason = %s, want 'Known spammer'", assertion.Reason)
	}
}

func TestParseTrustAssertionWrongKind(t *testing.T) {
	event := &nostr.Event{Kind: 1}
	if ParseTrustAssertion(event) != nil {
		t.Error("Should return nil for wrong kind")
	}
}

func TestCreateTrustAssertionEvent(t *testing.T) {
	assertion := &TrustAssertion{
		TargetPubkey:  "targetpubkey",
		TargetEventID: "targetevent",
		AssertionType: "trusted",
		Reason:        "Good reputation",
	}

	event := CreateTrustAssertionEvent("providerpubkey", assertion)

	if event.Kind != KindTrustAssertion {
		t.Errorf("Kind = %d, want %d", event.Kind, KindTrustAssertion)
	}

	if event.PubKey != "providerpubkey" {
		t.Errorf("PubKey = %s, want providerpubkey", event.PubKey)
	}

	// Check for required tags
	foundD := false
	foundP := false
	foundAssertion := false

	for _, tag := range event.Tags {
		if len(tag) >= 2 {
			switch tag[0] {
			case "d":
				foundD = true
			case "p":
				foundP = true
			case "assertion":
				foundAssertion = true
			}
		}
	}

	if !foundD {
		t.Error("Missing d-tag")
	}
	if !foundP {
		t.Error("Missing p-tag")
	}
	if !foundAssertion {
		t.Error("Missing assertion tag")
	}
}

func TestIsTrustedProvider(t *testing.T) {
	providers := []TrustProvider{
		{Pubkey: "provider1"},
		{Pubkey: "provider2"},
	}

	if !IsTrustedProvider(providers, "provider1") {
		t.Error("IsTrustedProvider(providers, provider1) = false, want true")
	}

	if !IsTrustedProvider(providers, "provider2") {
		t.Error("IsTrustedProvider(providers, provider2) = false, want true")
	}

	if IsTrustedProvider(providers, "unknown") {
		t.Error("IsTrustedProvider(providers, unknown) = true, want false")
	}
}

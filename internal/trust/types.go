// Package trust implements NIP-85 trusted assertions
//
// NIP-85 allows users to delegate trust calculations to trusted service providers.
// Users publish kind 10040 events listing their trusted assertion providers,
// and clients can use these to augment Web of Trust calculations.
//
// Reference: https://github.com/nostr-protocol/nips/blob/master/85.md
package trust

import (
	"github.com/nbd-wtf/go-nostr"
)

// Event kinds for NIP-85
const (
	// KindTrustedProviders lists trusted assertion providers
	KindTrustedProviders = 10040

	// KindTrustAssertion is an assertion from a provider
	KindTrustAssertion = 30040
)

// TrustProvider represents a trusted assertion provider
type TrustProvider struct {
	// Pubkey is the provider's pubkey
	Pubkey string
	// RelayHint is an optional relay URL
	RelayHint string
	// Kinds are the event kinds this provider is trusted for (empty = all)
	Kinds []int
	// Tags are the tag types this provider is trusted for (empty = all)
	Tags []string
}

// TrustAssertion represents an assertion from a provider
type TrustAssertion struct {
	// ProviderPubkey is who made the assertion
	ProviderPubkey string
	// TargetPubkey is the pubkey being asserted about
	TargetPubkey string
	// TargetEventID is the event being asserted about (optional)
	TargetEventID string
	// AssertionType is the type of assertion (e.g., "trusted", "spam")
	AssertionType string
	// Confidence is an optional confidence level (0-1)
	Confidence float64
	// Reason is an optional reason for the assertion
	Reason string
}

// ParseTrustedProviders parses a kind 10040 event
func ParseTrustedProviders(event *nostr.Event) []TrustProvider {
	if event.Kind != KindTrustedProviders {
		return nil
	}

	var providers []TrustProvider

	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}

		if tag[0] == "p" {
			provider := TrustProvider{
				Pubkey: tag[1],
			}

			// Parse optional relay hint
			if len(tag) >= 3 {
				provider.RelayHint = tag[2]
			}

			providers = append(providers, provider)
		}
	}

	return providers
}

// CreateTrustedProvidersEvent creates a kind 10040 event
func CreateTrustedProvidersEvent(pubkey string, providers []TrustProvider) *nostr.Event {
	event := &nostr.Event{
		Kind:      KindTrustedProviders,
		PubKey:    pubkey,
		CreatedAt: nostr.Now(),
		Content:   "",
		Tags:      nostr.Tags{},
	}

	for _, provider := range providers {
		tag := nostr.Tag{"p", provider.Pubkey}
		if provider.RelayHint != "" {
			tag = append(tag, provider.RelayHint)
		}
		event.Tags = append(event.Tags, tag)
	}

	return event
}

// ParseTrustAssertion parses a kind 30040 assertion event
func ParseTrustAssertion(event *nostr.Event) *TrustAssertion {
	if event.Kind != KindTrustAssertion {
		return nil
	}

	assertion := &TrustAssertion{
		ProviderPubkey: event.PubKey,
	}

	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}

		switch tag[0] {
		case "p":
			assertion.TargetPubkey = tag[1]
		case "e":
			assertion.TargetEventID = tag[1]
		case "d":
			// d-tag often contains assertion type
			assertion.AssertionType = tag[1]
		case "assertion":
			assertion.AssertionType = tag[1]
		case "reason":
			assertion.Reason = tag[1]
		}
	}

	return assertion
}

// CreateTrustAssertionEvent creates a kind 30040 assertion event
func CreateTrustAssertionEvent(providerPubkey string, assertion *TrustAssertion) *nostr.Event {
	event := &nostr.Event{
		Kind:      KindTrustAssertion,
		PubKey:    providerPubkey,
		CreatedAt: nostr.Now(),
		Content:   "",
		Tags:      nostr.Tags{},
	}

	// d-tag for deduplication
	if assertion.AssertionType != "" {
		event.Tags = append(event.Tags, nostr.Tag{"d", assertion.AssertionType})
	}

	if assertion.TargetPubkey != "" {
		event.Tags = append(event.Tags, nostr.Tag{"p", assertion.TargetPubkey})
	}

	if assertion.TargetEventID != "" {
		event.Tags = append(event.Tags, nostr.Tag{"e", assertion.TargetEventID})
	}

	if assertion.AssertionType != "" {
		event.Tags = append(event.Tags, nostr.Tag{"assertion", assertion.AssertionType})
	}

	if assertion.Reason != "" {
		event.Tags = append(event.Tags, nostr.Tag{"reason", assertion.Reason})
	}

	return event
}

// IsTrustKind returns true if the kind is NIP-85 related
func IsTrustKind(kind int) bool {
	return kind == KindTrustedProviders || kind == KindTrustAssertion
}

// IsTrustedProvider checks if a pubkey is in the user's trusted providers
func IsTrustedProvider(providers []TrustProvider, pubkey string) bool {
	for _, p := range providers {
		if p.Pubkey == pubkey {
			return true
		}
	}
	return false
}

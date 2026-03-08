// Package labels implements NIP-32 content labeling
//
// NIP-32 defines kind 1985 for labeling events, pubkeys, relays, and other
// content. Labels are useful for:
// - Content moderation (spam, adult, nsfw)
// - Content discovery (topics, categories)
// - Licensing (CC, MIT, etc.)
// - Quality indicators (trusted, verified)
//
// Label events use L tags for namespaces and l tags for values:
//   - L tag: namespace identifier (e.g., "ugc", "com.example", "ISO-639-1")
//   - l tag: label value with namespace reference (e.g., ["l", "spam", "ugc"])
//
// Targets are identified by standard Nostr tags:
//   - e: event ID (with optional relay hint)
//   - p: pubkey
//   - a: addressable event reference
//   - r: relay URL
//   - t: topic/hashtag
//
// Reference: https://github.com/nostr-protocol/nips/blob/master/32.md
package labels

import (
	"github.com/nbd-wtf/go-nostr"
)

// KindLabel is the event kind for NIP-32 labels
const KindLabel = 1985

// Common label namespaces
const (
	// NamespaceUGC is for user-generated content labels
	NamespaceUGC = "ugc"

	// NamespaceModeration is for relay/admin moderation labels
	NamespaceModeration = "relay/moderation"

	// NamespaceContentWarning is for content warnings
	NamespaceContentWarning = "content-warning"

	// NamespaceQuality is for quality/trust indicators
	NamespaceQuality = "quality"

	// NamespaceLanguage is for ISO-639-1 language codes
	NamespaceLanguage = "ISO-639-1"

	// NamespaceLicense is for content licensing
	NamespaceLicense = "license"
)

// Common moderation labels
const (
	// LabelSpam indicates spam content
	LabelSpam = "spam"

	// LabelNSFW indicates not-safe-for-work content
	LabelNSFW = "nsfw"

	// LabelAdult indicates adult content
	LabelAdult = "adult"

	// LabelGore indicates violent/gore content
	LabelGore = "gore"

	// LabelAbuse indicates abusive content
	LabelAbuse = "abuse"

	// LabelIllegal indicates potentially illegal content
	LabelIllegal = "illegal"

	// LabelImpersonation indicates impersonation
	LabelImpersonation = "impersonation"

	// LabelBot indicates automated/bot account
	LabelBot = "bot"
)

// Common quality labels
const (
	// LabelVerified indicates verified content/user
	LabelVerified = "verified"

	// LabelTrusted indicates trusted by WoT
	LabelTrusted = "trusted"

	// LabelHighQuality indicates high-quality content
	LabelHighQuality = "high-quality"

	// LabelLowQuality indicates low-quality content
	LabelLowQuality = "low-quality"
)

// Label represents a parsed label from a label event
type Label struct {
	// Value is the label string (e.g., "spam", "nsfw")
	Value string
	// Namespace is the label namespace (e.g., "ugc", "relay/moderation")
	Namespace string
}

// Target represents what is being labeled
type Target struct {
	// Type is the target type: "e", "p", "a", "r", "t"
	Type string
	// ID is the target identifier (event ID, pubkey, relay URL, etc.)
	ID string
	// RelayHint is an optional relay hint for events/pubkeys
	RelayHint string
}

// LabelEvent wraps a nostr.Event and provides label-specific methods
type LabelEvent struct {
	Event *nostr.Event
}

// NewLabelEvent creates a LabelEvent wrapper
func NewLabelEvent(event *nostr.Event) *LabelEvent {
	return &LabelEvent{Event: event}
}

// IsLabelEvent returns true if the event is a NIP-32 label
func IsLabelEvent(event *nostr.Event) bool {
	return event.Kind == KindLabel
}

// GetNamespaces returns all L tags (namespaces) from the event
func (le *LabelEvent) GetNamespaces() []string {
	var namespaces []string
	for _, tag := range le.Event.Tags {
		if len(tag) >= 2 && tag[0] == "L" {
			namespaces = append(namespaces, tag[1])
		}
	}
	return namespaces
}

// GetLabels returns all labels from the event
func (le *LabelEvent) GetLabels() []Label {
	var labels []Label
	for _, tag := range le.Event.Tags {
		if len(tag) >= 2 && tag[0] == "l" {
			label := Label{Value: tag[1]}
			if len(tag) >= 3 {
				label.Namespace = tag[2]
			}
			labels = append(labels, label)
		}
	}
	return labels
}

// GetTargets returns all targets being labeled
func (le *LabelEvent) GetTargets() []Target {
	var targets []Target
	for _, tag := range le.Event.Tags {
		if len(tag) >= 2 {
			switch tag[0] {
			case "e", "p", "a", "r", "t":
				target := Target{
					Type: tag[0],
					ID:   tag[1],
				}
				// Check for relay hint
				if len(tag) >= 3 && tag[0] != "t" {
					target.RelayHint = tag[2]
				}
				targets = append(targets, target)
			}
		}
	}
	return targets
}

// HasLabel checks if the event has a specific label in a specific namespace
func (le *LabelEvent) HasLabel(value, namespace string) bool {
	for _, tag := range le.Event.Tags {
		if len(tag) >= 3 && tag[0] == "l" && tag[1] == value && tag[2] == namespace {
			return true
		}
	}
	return false
}

// HasNamespace checks if the event uses a specific namespace
func (le *LabelEvent) HasNamespace(namespace string) bool {
	for _, tag := range le.Event.Tags {
		if len(tag) >= 2 && tag[0] == "L" && tag[1] == namespace {
			return true
		}
	}
	return false
}

// IsModerationLabel returns true if this is a moderation-related label
func (le *LabelEvent) IsModerationLabel() bool {
	return le.HasNamespace(NamespaceModeration) || le.HasNamespace(NamespaceUGC)
}

// GetModerationLabels returns any moderation-related labels
func (le *LabelEvent) GetModerationLabels() []string {
	var modLabels []string
	moderationValues := map[string]bool{
		LabelSpam:          true,
		LabelNSFW:          true,
		LabelAdult:         true,
		LabelGore:          true,
		LabelAbuse:         true,
		LabelIllegal:       true,
		LabelImpersonation: true,
		LabelBot:           true,
	}

	for _, label := range le.GetLabels() {
		if moderationValues[label.Value] {
			modLabels = append(modLabels, label.Value)
		}
	}
	return modLabels
}

// TargetsEvent returns true if this label targets a specific event
func (le *LabelEvent) TargetsEvent(eventID string) bool {
	for _, target := range le.GetTargets() {
		if target.Type == "e" && target.ID == eventID {
			return true
		}
	}
	return false
}

// TargetsPubkey returns true if this label targets a specific pubkey
func (le *LabelEvent) TargetsPubkey(pubkey string) bool {
	for _, target := range le.GetTargets() {
		if target.Type == "p" && target.ID == pubkey {
			return true
		}
	}
	return false
}

// CreateLabelEvent creates a new label event (unsigned)
func CreateLabelEvent(pubkey string, labels []Label, targets []Target) *nostr.Event {
	event := &nostr.Event{
		Kind:      KindLabel,
		PubKey:    pubkey,
		CreatedAt: nostr.Now(),
		Content:   "",
		Tags:      make(nostr.Tags, 0),
	}

	// Add namespace tags
	namespaces := make(map[string]bool)
	for _, label := range labels {
		if label.Namespace != "" && !namespaces[label.Namespace] {
			event.Tags = append(event.Tags, nostr.Tag{"L", label.Namespace})
			namespaces[label.Namespace] = true
		}
	}

	// Add label tags
	for _, label := range labels {
		if label.Namespace != "" {
			event.Tags = append(event.Tags, nostr.Tag{"l", label.Value, label.Namespace})
		} else {
			event.Tags = append(event.Tags, nostr.Tag{"l", label.Value})
		}
	}

	// Add target tags
	for _, target := range targets {
		if target.RelayHint != "" {
			event.Tags = append(event.Tags, nostr.Tag{target.Type, target.ID, target.RelayHint})
		} else {
			event.Tags = append(event.Tags, nostr.Tag{target.Type, target.ID})
		}
	}

	return event
}

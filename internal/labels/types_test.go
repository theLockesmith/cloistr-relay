package labels

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

func TestIsLabelEvent(t *testing.T) {
	tests := []struct {
		kind     int
		expected bool
	}{
		{KindLabel, true},
		{1985, true},
		{0, false},
		{1, false},
		{7, false},
	}

	for _, tt := range tests {
		event := &nostr.Event{Kind: tt.kind}
		if got := IsLabelEvent(event); got != tt.expected {
			t.Errorf("IsLabelEvent(kind %d) = %v, want %v", tt.kind, got, tt.expected)
		}
	}
}

func TestLabelEventGetNamespaces(t *testing.T) {
	event := &nostr.Event{
		Kind: KindLabel,
		Tags: nostr.Tags{
			{"L", "ugc"},
			{"L", "relay/moderation"},
			{"l", "spam", "ugc"},
			{"e", "abc123"},
		},
	}

	le := NewLabelEvent(event)
	namespaces := le.GetNamespaces()

	if len(namespaces) != 2 {
		t.Errorf("Expected 2 namespaces, got %d", len(namespaces))
	}

	expected := map[string]bool{"ugc": true, "relay/moderation": true}
	for _, ns := range namespaces {
		if !expected[ns] {
			t.Errorf("Unexpected namespace: %s", ns)
		}
	}
}

func TestLabelEventGetLabels(t *testing.T) {
	event := &nostr.Event{
		Kind: KindLabel,
		Tags: nostr.Tags{
			{"L", "ugc"},
			{"l", "spam", "ugc"},
			{"l", "nsfw", "ugc"},
			{"l", "custom"}, // no namespace
			{"e", "abc123"},
		},
	}

	le := NewLabelEvent(event)
	labels := le.GetLabels()

	if len(labels) != 3 {
		t.Errorf("Expected 3 labels, got %d", len(labels))
	}

	// Check first label
	if labels[0].Value != "spam" || labels[0].Namespace != "ugc" {
		t.Errorf("First label mismatch: got %+v", labels[0])
	}

	// Check label without namespace
	if labels[2].Value != "custom" || labels[2].Namespace != "" {
		t.Errorf("Third label mismatch: got %+v", labels[2])
	}
}

func TestLabelEventGetTargets(t *testing.T) {
	event := &nostr.Event{
		Kind: KindLabel,
		Tags: nostr.Tags{
			{"L", "ugc"},
			{"l", "spam", "ugc"},
			{"e", "event123", "wss://relay.example.com"},
			{"p", "pubkey456"},
			{"r", "wss://badrelay.com"},
			{"t", "bitcoin"},
		},
	}

	le := NewLabelEvent(event)
	targets := le.GetTargets()

	if len(targets) != 4 {
		t.Errorf("Expected 4 targets, got %d", len(targets))
	}

	// Check event target with relay hint
	if targets[0].Type != "e" || targets[0].ID != "event123" || targets[0].RelayHint != "wss://relay.example.com" {
		t.Errorf("Event target mismatch: got %+v", targets[0])
	}

	// Check pubkey target
	if targets[1].Type != "p" || targets[1].ID != "pubkey456" {
		t.Errorf("Pubkey target mismatch: got %+v", targets[1])
	}

	// Check relay target
	if targets[2].Type != "r" || targets[2].ID != "wss://badrelay.com" {
		t.Errorf("Relay target mismatch: got %+v", targets[2])
	}

	// Check topic target (should not have relay hint)
	if targets[3].Type != "t" || targets[3].ID != "bitcoin" || targets[3].RelayHint != "" {
		t.Errorf("Topic target mismatch: got %+v", targets[3])
	}
}

func TestLabelEventHasLabel(t *testing.T) {
	event := &nostr.Event{
		Kind: KindLabel,
		Tags: nostr.Tags{
			{"L", "ugc"},
			{"l", "spam", "ugc"},
			{"l", "nsfw", "content-warning"},
		},
	}

	le := NewLabelEvent(event)

	if !le.HasLabel("spam", "ugc") {
		t.Error("Expected to have spam/ugc label")
	}

	if !le.HasLabel("nsfw", "content-warning") {
		t.Error("Expected to have nsfw/content-warning label")
	}

	if le.HasLabel("spam", "content-warning") {
		t.Error("Should not have spam/content-warning label")
	}

	if le.HasLabel("nonexistent", "ugc") {
		t.Error("Should not have nonexistent label")
	}
}

func TestLabelEventHasNamespace(t *testing.T) {
	event := &nostr.Event{
		Kind: KindLabel,
		Tags: nostr.Tags{
			{"L", "ugc"},
			{"L", "relay/moderation"},
		},
	}

	le := NewLabelEvent(event)

	if !le.HasNamespace("ugc") {
		t.Error("Expected to have ugc namespace")
	}

	if !le.HasNamespace("relay/moderation") {
		t.Error("Expected to have relay/moderation namespace")
	}

	if le.HasNamespace("nonexistent") {
		t.Error("Should not have nonexistent namespace")
	}
}

func TestLabelEventIsModerationLabel(t *testing.T) {
	tests := []struct {
		name     string
		tags     nostr.Tags
		expected bool
	}{
		{
			name: "moderation namespace",
			tags: nostr.Tags{
				{"L", NamespaceModeration},
				{"l", "spam", NamespaceModeration},
			},
			expected: true,
		},
		{
			name: "ugc namespace",
			tags: nostr.Tags{
				{"L", NamespaceUGC},
				{"l", "spam", NamespaceUGC},
			},
			expected: true,
		},
		{
			name: "other namespace",
			tags: nostr.Tags{
				{"L", "license"},
				{"l", "CC-BY", "license"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &nostr.Event{Kind: KindLabel, Tags: tt.tags}
			le := NewLabelEvent(event)
			if got := le.IsModerationLabel(); got != tt.expected {
				t.Errorf("IsModerationLabel() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestLabelEventGetModerationLabels(t *testing.T) {
	event := &nostr.Event{
		Kind: KindLabel,
		Tags: nostr.Tags{
			{"L", "ugc"},
			{"l", "spam", "ugc"},
			{"l", "nsfw", "ugc"},
			{"l", "custom", "ugc"}, // not a standard moderation label
		},
	}

	le := NewLabelEvent(event)
	modLabels := le.GetModerationLabels()

	if len(modLabels) != 2 {
		t.Errorf("Expected 2 moderation labels, got %d: %v", len(modLabels), modLabels)
	}

	expected := map[string]bool{"spam": true, "nsfw": true}
	for _, label := range modLabels {
		if !expected[label] {
			t.Errorf("Unexpected moderation label: %s", label)
		}
	}
}

func TestLabelEventTargetsEvent(t *testing.T) {
	event := &nostr.Event{
		Kind: KindLabel,
		Tags: nostr.Tags{
			{"L", "ugc"},
			{"l", "spam", "ugc"},
			{"e", "target123"},
			{"e", "target456"},
		},
	}

	le := NewLabelEvent(event)

	if !le.TargetsEvent("target123") {
		t.Error("Should target event target123")
	}

	if !le.TargetsEvent("target456") {
		t.Error("Should target event target456")
	}

	if le.TargetsEvent("other") {
		t.Error("Should not target event other")
	}
}

func TestLabelEventTargetsPubkey(t *testing.T) {
	event := &nostr.Event{
		Kind: KindLabel,
		Tags: nostr.Tags{
			{"L", "ugc"},
			{"l", "spam", "ugc"},
			{"p", "pubkey123"},
		},
	}

	le := NewLabelEvent(event)

	if !le.TargetsPubkey("pubkey123") {
		t.Error("Should target pubkey pubkey123")
	}

	if le.TargetsPubkey("other") {
		t.Error("Should not target pubkey other")
	}
}

func TestCreateLabelEvent(t *testing.T) {
	labels := []Label{
		{Value: "spam", Namespace: "ugc"},
		{Value: "nsfw", Namespace: "content-warning"},
	}

	targets := []Target{
		{Type: "e", ID: "event123", RelayHint: "wss://relay.example.com"},
		{Type: "p", ID: "pubkey456"},
	}

	event := CreateLabelEvent("testpubkey", labels, targets)

	if event.Kind != KindLabel {
		t.Errorf("Expected kind %d, got %d", KindLabel, event.Kind)
	}

	if event.PubKey != "testpubkey" {
		t.Errorf("Expected pubkey testpubkey, got %s", event.PubKey)
	}

	// Verify tags
	le := NewLabelEvent(event)

	namespaces := le.GetNamespaces()
	if len(namespaces) != 2 {
		t.Errorf("Expected 2 namespaces, got %d", len(namespaces))
	}

	parsedLabels := le.GetLabels()
	if len(parsedLabels) != 2 {
		t.Errorf("Expected 2 labels, got %d", len(parsedLabels))
	}

	parsedTargets := le.GetTargets()
	if len(parsedTargets) != 2 {
		t.Errorf("Expected 2 targets, got %d", len(parsedTargets))
	}

	// Verify relay hint preserved
	if parsedTargets[0].RelayHint != "wss://relay.example.com" {
		t.Errorf("Relay hint not preserved: got %s", parsedTargets[0].RelayHint)
	}
}

func TestKindConstant(t *testing.T) {
	if KindLabel != 1985 {
		t.Errorf("KindLabel = %d, want 1985", KindLabel)
	}
}

func TestNamespaceConstants(t *testing.T) {
	// Just verify constants are defined and non-empty
	namespaces := []string{
		NamespaceUGC,
		NamespaceModeration,
		NamespaceContentWarning,
		NamespaceQuality,
		NamespaceLanguage,
		NamespaceLicense,
	}

	for _, ns := range namespaces {
		if ns == "" {
			t.Error("Found empty namespace constant")
		}
	}
}

func TestLabelConstants(t *testing.T) {
	// Just verify constants are defined and non-empty
	labels := []string{
		LabelSpam,
		LabelNSFW,
		LabelAdult,
		LabelGore,
		LabelAbuse,
		LabelIllegal,
		LabelImpersonation,
		LabelBot,
		LabelVerified,
		LabelTrusted,
		LabelHighQuality,
		LabelLowQuality,
	}

	for _, label := range labels {
		if label == "" {
			t.Error("Found empty label constant")
		}
	}
}

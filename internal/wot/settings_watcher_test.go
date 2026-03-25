package wot

import (
	"encoding/json"
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

func TestIsWoTSettingsEvent(t *testing.T) {
	tests := []struct {
		name     string
		event    *nostr.Event
		expected bool
	}{
		{
			name: "valid WoT settings event",
			event: &nostr.Event{
				Kind: KindAppSpecificData,
				Tags: nostr.Tags{{"d", WoTSettingsDTag}},
			},
			expected: true,
		},
		{
			name: "wrong kind",
			event: &nostr.Event{
				Kind: 1,
				Tags: nostr.Tags{{"d", WoTSettingsDTag}},
			},
			expected: false,
		},
		{
			name: "wrong d-tag",
			event: &nostr.Event{
				Kind: KindAppSpecificData,
				Tags: nostr.Tags{{"d", "other-app"}},
			},
			expected: false,
		},
		{
			name: "no d-tag",
			event: &nostr.Event{
				Kind: KindAppSpecificData,
				Tags: nostr.Tags{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsWoTSettingsEvent(tt.event); got != tt.expected {
				t.Errorf("IsWoTSettingsEvent() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetTagValue(t *testing.T) {
	event := &nostr.Event{
		Tags: nostr.Tags{
			{"d", "my-d-value"},
			{"p", "pubkey1"},
			{"e", "event1", "relay"},
		},
	}

	tests := []struct {
		key      string
		expected string
	}{
		{"d", "my-d-value"},
		{"p", "pubkey1"},
		{"e", "event1"},
		{"missing", ""},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := getTagValue(event, tt.key); got != tt.expected {
				t.Errorf("getTagValue(%s) = %v, want %v", tt.key, got, tt.expected)
			}
		})
	}
}

func TestTruncatePubkey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"abcdefgh12345678", "abcdefgh"},
		{"abcdefgh", "abcdefgh"},
		{"abc", "abc"},
		{"", ""},
	}

	for _, tt := range tests {
		if got := truncatePubkey(tt.input); got != tt.expected {
			t.Errorf("truncatePubkey(%s) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestSettingsEventContent_MarshalUnmarshal(t *testing.T) {
	depth := 3
	bits := 12

	original := SettingsEventContent{
		BlockedPubkeys: []string{"blocked1", "blocked2"},
		TrustedPubkeys: []string{"trusted1"},
		MaxTrustDepth:  &depth,
		MinPowBits:     &bits,
		TrustAnchor:    "myanchor",
	}

	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	// Unmarshal
	var result SettingsEventContent
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	// Verify
	if len(result.BlockedPubkeys) != 2 {
		t.Errorf("Expected 2 blocked pubkeys, got %d", len(result.BlockedPubkeys))
	}
	if len(result.TrustedPubkeys) != 1 {
		t.Errorf("Expected 1 trusted pubkey, got %d", len(result.TrustedPubkeys))
	}
	if result.MaxTrustDepth == nil || *result.MaxTrustDepth != 3 {
		t.Errorf("MaxTrustDepth mismatch")
	}
	if result.MinPowBits == nil || *result.MinPowBits != 12 {
		t.Errorf("MinPowBits mismatch")
	}
	if result.TrustAnchor != "myanchor" {
		t.Errorf("TrustAnchor mismatch")
	}
}

func TestSettingsEventContent_OmitEmpty(t *testing.T) {
	// Empty content should produce minimal JSON
	content := SettingsEventContent{}
	data, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	// Should be just {}
	if string(data) != "{}" {
		t.Errorf("Expected {}, got %s", string(data))
	}
}

func TestCreateSettingsEvent(t *testing.T) {
	depth := 2
	settings := &UserSettings{
		Pubkey:         "user123",
		TrustAnchor:    "anchor456",
		MaxTrustDepth:  &depth,
		BlockedPubkeys: []string{"blocked1"},
		TrustedPubkeys: []string{"trusted1", "trusted2"},
	}

	event := CreateSettingsEvent(settings)

	// Check kind
	if event.Kind != KindAppSpecificData {
		t.Errorf("Expected kind %d, got %d", KindAppSpecificData, event.Kind)
	}

	// Check pubkey
	if event.PubKey != "user123" {
		t.Errorf("Expected pubkey user123, got %s", event.PubKey)
	}

	// Check d-tag
	dTag := getTagValue(event, "d")
	if dTag != WoTSettingsDTag {
		t.Errorf("Expected d-tag %s, got %s", WoTSettingsDTag, dTag)
	}

	// Check content
	var content SettingsEventContent
	if err := json.Unmarshal([]byte(event.Content), &content); err != nil {
		t.Fatalf("Failed to parse content: %v", err)
	}

	if len(content.BlockedPubkeys) != 1 {
		t.Errorf("Expected 1 blocked, got %d", len(content.BlockedPubkeys))
	}
	if len(content.TrustedPubkeys) != 2 {
		t.Errorf("Expected 2 trusted, got %d", len(content.TrustedPubkeys))
	}
	if content.TrustAnchor != "anchor456" {
		t.Errorf("Expected anchor anchor456, got %s", content.TrustAnchor)
	}
	if content.MaxTrustDepth == nil || *content.MaxTrustDepth != 2 {
		t.Errorf("Expected max depth 2")
	}
}

func TestSettingsWatcher_ParseSettingsEvent(t *testing.T) {
	watcher := NewSettingsWatcher(nil)

	depth := 3
	content := SettingsEventContent{
		BlockedPubkeys: []string{"blocked1"},
		TrustedPubkeys: []string{"trusted1"},
		MaxTrustDepth:  &depth,
		TrustAnchor:    "customanchor",
	}
	contentBytes, _ := json.Marshal(content)

	event := &nostr.Event{
		PubKey:  "testuser",
		Kind:    KindAppSpecificData,
		Content: string(contentBytes),
		Tags:    nostr.Tags{{"d", WoTSettingsDTag}},
	}

	settings, err := watcher.parseSettingsEvent(event)
	if err != nil {
		t.Fatalf("parseSettingsEvent error: %v", err)
	}

	if settings.Pubkey != "testuser" {
		t.Errorf("Pubkey mismatch: got %s", settings.Pubkey)
	}
	if settings.TrustAnchor != "customanchor" {
		t.Errorf("TrustAnchor mismatch: got %s", settings.TrustAnchor)
	}
	if len(settings.BlockedPubkeys) != 1 {
		t.Errorf("BlockedPubkeys count mismatch")
	}
}

func TestSettingsWatcher_ParseSettingsEvent_DefaultAnchor(t *testing.T) {
	watcher := NewSettingsWatcher(nil)

	// Content without trust_anchor
	content := SettingsEventContent{
		BlockedPubkeys: []string{"blocked1"},
	}
	contentBytes, _ := json.Marshal(content)

	event := &nostr.Event{
		PubKey:  "testuser",
		Kind:    KindAppSpecificData,
		Content: string(contentBytes),
		Tags:    nostr.Tags{{"d", WoTSettingsDTag}},
	}

	settings, err := watcher.parseSettingsEvent(event)
	if err != nil {
		t.Fatalf("parseSettingsEvent error: %v", err)
	}

	// Should default to event pubkey
	if settings.TrustAnchor != "testuser" {
		t.Errorf("TrustAnchor should default to pubkey, got %s", settings.TrustAnchor)
	}
}

func TestSettingsWatcher_ParseSettingsEvent_InvalidJSON(t *testing.T) {
	watcher := NewSettingsWatcher(nil)

	event := &nostr.Event{
		PubKey:  "testuser",
		Kind:    KindAppSpecificData,
		Content: "not valid json",
		Tags:    nostr.Tags{{"d", WoTSettingsDTag}},
	}

	_, err := watcher.parseSettingsEvent(event)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

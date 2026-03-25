package haven

import (
	"encoding/json"
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

func TestIsHavenSettingsEvent(t *testing.T) {
	tests := []struct {
		name     string
		event    *nostr.Event
		expected bool
	}{
		{
			name: "valid HAVEN settings event",
			event: &nostr.Event{
				Kind: 30078,
				Tags: nostr.Tags{{"d", HavenSettingsDTag}},
			},
			expected: true,
		},
		{
			name: "wrong kind",
			event: &nostr.Event{
				Kind: 1,
				Tags: nostr.Tags{{"d", HavenSettingsDTag}},
			},
			expected: false,
		},
		{
			name: "wrong d-tag",
			event: &nostr.Event{
				Kind: 30078,
				Tags: nostr.Tags{{"d", "other-app"}},
			},
			expected: false,
		},
		{
			name: "no d-tag",
			event: &nostr.Event{
				Kind: 30078,
				Tags: nostr.Tags{},
			},
			expected: false,
		},
		{
			name: "WoT settings (different d-tag)",
			event: &nostr.Event{
				Kind: 30078,
				Tags: nostr.Tags{{"d", "cloistr-wot-settings"}},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsHavenSettingsEvent(tt.event); got != tt.expected {
				t.Errorf("IsHavenSettingsEvent() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetEventTagValue(t *testing.T) {
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
			if got := getEventTagValue(event, tt.key); got != tt.expected {
				t.Errorf("getEventTagValue(%s) = %v, want %v", tt.key, got, tt.expected)
			}
		})
	}
}

func TestHavenSettingsEventContent_MarshalUnmarshal(t *testing.T) {
	outboxRead := true
	inboxWrite := false

	original := HavenSettingsEventContent{
		BlastrEnabled:    true,
		BlastrRelays:     []string{"wss://relay1.com", "wss://relay2.com"},
		ImporterEnabled:  true,
		ImporterRelays:   []string{"wss://relay3.com"},
		ImporterRealtime: true,
		PublicOutboxRead: &outboxRead,
		PublicInboxWrite: &inboxWrite,
	}

	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	// Unmarshal
	var result HavenSettingsEventContent
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	// Verify
	if !result.BlastrEnabled {
		t.Error("BlastrEnabled should be true")
	}
	if len(result.BlastrRelays) != 2 {
		t.Errorf("Expected 2 blastr relays, got %d", len(result.BlastrRelays))
	}
	if !result.ImporterEnabled {
		t.Error("ImporterEnabled should be true")
	}
	if len(result.ImporterRelays) != 1 {
		t.Errorf("Expected 1 importer relay, got %d", len(result.ImporterRelays))
	}
	if !result.ImporterRealtime {
		t.Error("ImporterRealtime should be true")
	}
	if result.PublicOutboxRead == nil || !*result.PublicOutboxRead {
		t.Error("PublicOutboxRead mismatch")
	}
	if result.PublicInboxWrite == nil || *result.PublicInboxWrite {
		t.Error("PublicInboxWrite mismatch")
	}
}

func TestHavenSettingsEventContent_OmitEmpty(t *testing.T) {
	// Empty content should produce minimal JSON
	content := HavenSettingsEventContent{}
	data, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	// Should be just {}
	if string(data) != "{}" {
		t.Errorf("Expected {}, got %s", string(data))
	}
}

func TestHavenSettingsEventContent_WithWoT(t *testing.T) {
	depth := 2
	content := HavenSettingsEventContent{
		BlastrEnabled: true,
		WoT: &WoTSettingsContent{
			BlockedPubkeys: []string{"blocked1"},
			TrustedPubkeys: []string{"trusted1"},
			MaxTrustDepth:  &depth,
			TrustAnchor:    "myanchor",
		},
	}

	data, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var result HavenSettingsEventContent
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if result.WoT == nil {
		t.Fatal("WoT should be present")
	}
	if len(result.WoT.BlockedPubkeys) != 1 {
		t.Error("BlockedPubkeys mismatch")
	}
	if result.WoT.TrustAnchor != "myanchor" {
		t.Error("TrustAnchor mismatch")
	}
}

func TestCreateHavenSettingsEvent(t *testing.T) {
	settings := &UserSettings{
		Pubkey:           "user123",
		BlastrEnabled:    true,
		BlastrRelays:     []string{"wss://relay1.com"},
		ImporterEnabled:  true,
		ImporterRelays:   []string{"wss://relay2.com"},
		ImporterRealtime: true,
		PublicOutboxRead: true, // Default, should not appear in JSON
		PublicInboxWrite: false, // Non-default, should appear
	}

	event := CreateHavenSettingsEvent(settings)

	// Check kind
	if event.Kind != 30078 {
		t.Errorf("Expected kind 30078, got %d", event.Kind)
	}

	// Check pubkey
	if event.PubKey != "user123" {
		t.Errorf("Expected pubkey user123, got %s", event.PubKey)
	}

	// Check d-tag
	dTag := getEventTagValue(event, "d")
	if dTag != HavenSettingsDTag {
		t.Errorf("Expected d-tag %s, got %s", HavenSettingsDTag, dTag)
	}

	// Check content
	var content HavenSettingsEventContent
	if err := json.Unmarshal([]byte(event.Content), &content); err != nil {
		t.Fatalf("Failed to parse content: %v", err)
	}

	if !content.BlastrEnabled {
		t.Error("BlastrEnabled should be true")
	}
	if len(content.BlastrRelays) != 1 {
		t.Errorf("Expected 1 blastr relay, got %d", len(content.BlastrRelays))
	}
	if !content.ImporterEnabled {
		t.Error("ImporterEnabled should be true")
	}
	// PublicOutboxRead is default (true), should be nil
	if content.PublicOutboxRead != nil {
		t.Error("PublicOutboxRead should be nil (default)")
	}
	// PublicInboxWrite is non-default (false), should be present
	if content.PublicInboxWrite == nil || *content.PublicInboxWrite {
		t.Error("PublicInboxWrite should be false")
	}
}

func TestHavenSettingsWatcher_ParseSettingsEvent(t *testing.T) {
	watcher := NewHavenSettingsWatcher(nil)

	content := HavenSettingsEventContent{
		BlastrEnabled:   true,
		BlastrRelays:    []string{"wss://relay1.com"},
		ImporterEnabled: true,
		ImporterRelays:  []string{"wss://relay2.com"},
	}
	contentBytes, _ := json.Marshal(content)

	event := &nostr.Event{
		PubKey:  "testuser",
		Kind:    30078,
		Content: string(contentBytes),
		Tags:    nostr.Tags{{"d", HavenSettingsDTag}},
	}

	settings, err := watcher.parseSettingsEvent(event)
	if err != nil {
		t.Fatalf("parseSettingsEvent error: %v", err)
	}

	if settings.Pubkey != "testuser" {
		t.Errorf("Pubkey mismatch: got %s", settings.Pubkey)
	}
	if !settings.BlastrEnabled {
		t.Error("BlastrEnabled should be true")
	}
	if len(settings.BlastrRelays) != 1 {
		t.Error("BlastrRelays mismatch")
	}
	if !settings.ImporterEnabled {
		t.Error("ImporterEnabled should be true")
	}
	// Defaults should be applied
	if !settings.PublicOutboxRead {
		t.Error("PublicOutboxRead should default to true")
	}
	if !settings.PublicInboxWrite {
		t.Error("PublicInboxWrite should default to true")
	}
}

func TestHavenSettingsWatcher_ParseSettingsEvent_WithPrivacy(t *testing.T) {
	watcher := NewHavenSettingsWatcher(nil)

	outbox := false
	inbox := false
	content := HavenSettingsEventContent{
		BlastrEnabled:    true,
		PublicOutboxRead: &outbox,
		PublicInboxWrite: &inbox,
	}
	contentBytes, _ := json.Marshal(content)

	event := &nostr.Event{
		PubKey:  "testuser",
		Kind:    30078,
		Content: string(contentBytes),
		Tags:    nostr.Tags{{"d", HavenSettingsDTag}},
	}

	settings, err := watcher.parseSettingsEvent(event)
	if err != nil {
		t.Fatalf("parseSettingsEvent error: %v", err)
	}

	if settings.PublicOutboxRead {
		t.Error("PublicOutboxRead should be false")
	}
	if settings.PublicInboxWrite {
		t.Error("PublicInboxWrite should be false")
	}
}

func TestHavenSettingsWatcher_ParseSettingsEvent_InvalidJSON(t *testing.T) {
	watcher := NewHavenSettingsWatcher(nil)

	event := &nostr.Event{
		PubKey:  "testuser",
		Kind:    30078,
		Content: "not valid json",
		Tags:    nostr.Tags{{"d", HavenSettingsDTag}},
	}

	_, err := watcher.parseSettingsEvent(event)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestUserSettingsExport_Fields(t *testing.T) {
	export := UserSettingsExport{
		Pubkey:     alicePubkey,
		Tier:       "premium",
		ExportedAt: "2026-03-25T12:00:00Z",
		Haven: &HavenSettingsEventContent{
			BlastrEnabled: true,
			BlastrRelays:  []string{"wss://relay.test"},
		},
	}

	if export.Pubkey != alicePubkey {
		t.Error("Pubkey mismatch")
	}
	if export.Tier != "premium" {
		t.Error("Tier mismatch")
	}
	if export.Haven == nil {
		t.Error("Haven should not be nil")
	}
	if !export.Haven.BlastrEnabled {
		t.Error("BlastrEnabled should be true")
	}
}

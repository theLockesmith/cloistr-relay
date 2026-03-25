package wot

import (
	"context"
	"encoding/json"
	"log"

	"github.com/nbd-wtf/go-nostr"
)

// NIP-78 Application-Specific Data
// Users publish their WoT settings as kind 30078 events
// This allows settings to be portable and signed by the user

const (
	// KindAppSpecificData is NIP-78 application-specific data
	KindAppSpecificData = 30078
	// WoTSettingsDTag is the "d" tag value for WoT settings
	WoTSettingsDTag = "cloistr-wot-settings"
)

// SettingsEventContent is the JSON structure for WoT settings in NIP-78 events
type SettingsEventContent struct {
	// BlockedPubkeys are pubkeys the user wants to block
	BlockedPubkeys []string `json:"blocked_pubkeys,omitempty"`
	// TrustedPubkeys are pubkeys the user explicitly trusts
	TrustedPubkeys []string `json:"trusted_pubkeys,omitempty"`
	// MaxTrustDepth overrides relay default (omitted = use relay default)
	MaxTrustDepth *int `json:"max_trust_depth,omitempty"`
	// MinPowBits overrides relay default for unknown senders (omitted = use relay default)
	MinPowBits *int `json:"min_pow_bits,omitempty"`
	// TrustAnchor is the pubkey to use as trust root (typically themselves)
	TrustAnchor string `json:"trust_anchor,omitempty"`
}

// SettingsWatcher watches for NIP-78 WoT settings events and syncs to database
type SettingsWatcher struct {
	store *UserSettingsStore
}

// NewSettingsWatcher creates a new settings watcher
func NewSettingsWatcher(store *UserSettingsStore) *SettingsWatcher {
	return &SettingsWatcher{store: store}
}

// OnEventSaved returns a handler that watches for WoT settings events
// This should be registered with relay.OnEventSaved
func (w *SettingsWatcher) OnEventSaved() func(context.Context, *nostr.Event) {
	return func(ctx context.Context, event *nostr.Event) {
		// Only process NIP-78 application-specific data
		if event.Kind != KindAppSpecificData {
			return
		}

		// Check for our specific "d" tag
		dTag := getTagValue(event, "d")
		if dTag != WoTSettingsDTag {
			return
		}

		// Parse settings from content
		settings, err := w.parseSettingsEvent(event)
		if err != nil {
			log.Printf("WoT SettingsWatcher: failed to parse settings for %s: %v",
				truncatePubkey(event.PubKey), err)
			return
		}

		// Save to database
		if err := w.store.SaveSettings(ctx, settings); err != nil {
			log.Printf("WoT SettingsWatcher: failed to save settings for %s: %v",
				truncatePubkey(event.PubKey), err)
			return
		}

		log.Printf("WoT SettingsWatcher: synced settings for %s (blocked=%d, trusted=%d)",
			truncatePubkey(event.PubKey),
			len(settings.BlockedPubkeys),
			len(settings.TrustedPubkeys))
	}
}

// parseSettingsEvent parses a NIP-78 event into UserSettings
func (w *SettingsWatcher) parseSettingsEvent(event *nostr.Event) (*UserSettings, error) {
	var content SettingsEventContent
	if err := json.Unmarshal([]byte(event.Content), &content); err != nil {
		return nil, err
	}

	settings := &UserSettings{
		Pubkey:         event.PubKey,
		TrustAnchor:    content.TrustAnchor,
		MaxTrustDepth:  content.MaxTrustDepth,
		MinPowBits:     content.MinPowBits,
		BlockedPubkeys: content.BlockedPubkeys,
		TrustedPubkeys: content.TrustedPubkeys,
	}

	// Default trust anchor to self if not specified
	if settings.TrustAnchor == "" {
		settings.TrustAnchor = event.PubKey
	}

	return settings, nil
}

// CreateSettingsEvent creates a NIP-78 event from UserSettings
// This is useful for exporting settings or for clients to publish updates
func CreateSettingsEvent(settings *UserSettings) *nostr.Event {
	content := SettingsEventContent{
		BlockedPubkeys: settings.BlockedPubkeys,
		TrustedPubkeys: settings.TrustedPubkeys,
		MaxTrustDepth:  settings.MaxTrustDepth,
		MinPowBits:     settings.MinPowBits,
		TrustAnchor:    settings.TrustAnchor,
	}

	contentBytes, _ := json.Marshal(content)

	return &nostr.Event{
		Kind:      KindAppSpecificData,
		PubKey:    settings.Pubkey,
		CreatedAt: nostr.Now(),
		Content:   string(contentBytes),
		Tags: nostr.Tags{
			{"d", WoTSettingsDTag},
		},
	}
}

// getTagValue returns the first value for a tag key
func getTagValue(event *nostr.Event, key string) string {
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == key {
			return tag[1]
		}
	}
	return ""
}

// truncatePubkey returns first 8 chars of a pubkey for logging
func truncatePubkey(pubkey string) string {
	if len(pubkey) >= 8 {
		return pubkey[:8]
	}
	return pubkey
}

// IsWoTSettingsEvent checks if an event is a WoT settings event
func IsWoTSettingsEvent(event *nostr.Event) bool {
	if event.Kind != KindAppSpecificData {
		return false
	}
	return getTagValue(event, "d") == WoTSettingsDTag
}

package haven

import (
	"context"
	"encoding/json"
	"log"

	"github.com/nbd-wtf/go-nostr"
)

// NIP-78 Application-Specific Data for HAVEN settings
// Users publish their HAVEN settings as kind 30078 events
// This allows settings to be portable and signed by the user

const (
	// HavenSettingsDTag is the "d" tag value for HAVEN settings
	HavenSettingsDTag = "cloistr-haven-settings"
)

// HavenSettingsEventContent is the JSON structure for HAVEN settings in NIP-78 events
type HavenSettingsEventContent struct {
	// Blastr settings
	BlastrEnabled bool     `json:"blastr_enabled,omitempty"`
	BlastrRelays  []string `json:"blastr_relays,omitempty"`

	// Importer settings
	ImporterEnabled  bool     `json:"importer_enabled,omitempty"`
	ImporterRelays   []string `json:"importer_relays,omitempty"`
	ImporterRealtime bool     `json:"importer_realtime,omitempty"`

	// Privacy settings
	PublicOutboxRead   *bool `json:"public_outbox_read,omitempty"`
	PublicInboxWrite   *bool `json:"public_inbox_write,omitempty"`
	RequireAuthChat    *bool `json:"require_auth_chat,omitempty"`
	RequireAuthPrivate *bool `json:"require_auth_private,omitempty"`

	// WoT settings (embedded for combined settings event)
	WoT *WoTSettingsContent `json:"wot,omitempty"`
}

// WoTSettingsContent is the nested WoT settings in HAVEN settings
type WoTSettingsContent struct {
	BlockedPubkeys []string `json:"blocked_pubkeys,omitempty"`
	TrustedPubkeys []string `json:"trusted_pubkeys,omitempty"`
	MaxTrustDepth  *int     `json:"max_trust_depth,omitempty"`
	MinPowBits     *int     `json:"min_pow_bits,omitempty"`
	TrustAnchor    string   `json:"trust_anchor,omitempty"`
}

// HavenSettingsWatcher watches for NIP-78 HAVEN settings events and syncs to database
type HavenSettingsWatcher struct {
	store *UserSettingsStore
}

// NewHavenSettingsWatcher creates a new HAVEN settings watcher
func NewHavenSettingsWatcher(store *UserSettingsStore) *HavenSettingsWatcher {
	return &HavenSettingsWatcher{store: store}
}

// OnEventSaved returns a handler that watches for HAVEN settings events
// This should be registered with relay.OnEventSaved
func (w *HavenSettingsWatcher) OnEventSaved() func(context.Context, *nostr.Event) {
	return func(ctx context.Context, event *nostr.Event) {
		// Only process NIP-78 application-specific data
		if event.Kind != 30078 {
			return
		}

		// Check for our specific "d" tag
		dTag := getEventTagValue(event, "d")
		if dTag != HavenSettingsDTag {
			return
		}

		// Parse settings from content
		settings, err := w.parseSettingsEvent(event)
		if err != nil {
			log.Printf("HAVEN SettingsWatcher: failed to parse settings for %s: %v",
				truncateID(event.PubKey), err)
			return
		}

		// Save to database
		if err := w.store.SaveSettings(ctx, settings); err != nil {
			log.Printf("HAVEN SettingsWatcher: failed to save settings for %s: %v",
				truncateID(event.PubKey), err)
			return
		}

		log.Printf("HAVEN SettingsWatcher: synced settings for %s (blastr=%v, importer=%v)",
			truncateID(event.PubKey), settings.BlastrEnabled, settings.ImporterEnabled)
	}
}

// parseSettingsEvent parses a NIP-78 event into UserSettings
func (w *HavenSettingsWatcher) parseSettingsEvent(event *nostr.Event) (*UserSettings, error) {
	var content HavenSettingsEventContent
	if err := json.Unmarshal([]byte(event.Content), &content); err != nil {
		return nil, err
	}

	settings := &UserSettings{
		Pubkey:           event.PubKey,
		BlastrEnabled:    content.BlastrEnabled,
		BlastrRelays:     content.BlastrRelays,
		ImporterEnabled:  content.ImporterEnabled,
		ImporterRelays:   content.ImporterRelays,
		ImporterRealtime: content.ImporterRealtime,
	}

	// Apply privacy settings with defaults
	if content.PublicOutboxRead != nil {
		settings.PublicOutboxRead = *content.PublicOutboxRead
	} else {
		settings.PublicOutboxRead = true // Default
	}

	if content.PublicInboxWrite != nil {
		settings.PublicInboxWrite = *content.PublicInboxWrite
	} else {
		settings.PublicInboxWrite = true // Default
	}

	if content.RequireAuthChat != nil {
		settings.RequireAuthChat = *content.RequireAuthChat
	} else {
		settings.RequireAuthChat = true // Default
	}

	if content.RequireAuthPrivate != nil {
		settings.RequireAuthPrivate = *content.RequireAuthPrivate
	} else {
		settings.RequireAuthPrivate = true // Default
	}

	return settings, nil
}

// CreateHavenSettingsEvent creates a NIP-78 event from UserSettings
// This is useful for exporting settings or for clients to publish updates
func CreateHavenSettingsEvent(settings *UserSettings) *nostr.Event {
	content := HavenSettingsEventContent{
		BlastrEnabled:    settings.BlastrEnabled,
		BlastrRelays:     settings.BlastrRelays,
		ImporterEnabled:  settings.ImporterEnabled,
		ImporterRelays:   settings.ImporterRelays,
		ImporterRealtime: settings.ImporterRealtime,
	}

	// Only include privacy settings if they differ from defaults
	if !settings.PublicOutboxRead {
		f := false
		content.PublicOutboxRead = &f
	}
	if !settings.PublicInboxWrite {
		f := false
		content.PublicInboxWrite = &f
	}
	if !settings.RequireAuthChat {
		f := false
		content.RequireAuthChat = &f
	}
	if !settings.RequireAuthPrivate {
		f := false
		content.RequireAuthPrivate = &f
	}

	contentBytes, _ := json.Marshal(content)

	return &nostr.Event{
		Kind:      30078,
		PubKey:    settings.Pubkey,
		CreatedAt: nostr.Now(),
		Content:   string(contentBytes),
		Tags: nostr.Tags{
			{"d", HavenSettingsDTag},
		},
	}
}

// IsHavenSettingsEvent checks if an event is a HAVEN settings event
func IsHavenSettingsEvent(event *nostr.Event) bool {
	if event.Kind != 30078 {
		return false
	}
	return getEventTagValue(event, "d") == HavenSettingsDTag
}

// getEventTagValue returns the first value for a tag key
func getEventTagValue(event *nostr.Event, key string) string {
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == key {
			return tag[1]
		}
	}
	return ""
}

// UserSettingsExport combines all user settings for export
type UserSettingsExport struct {
	Pubkey      string                     `json:"pubkey"`
	Haven       *HavenSettingsEventContent `json:"haven,omitempty"`
	Tier        string                     `json:"tier,omitempty"`
	TierExpires string                     `json:"tier_expires,omitempty"`
	ExportedAt  string                     `json:"exported_at"`
}

// ExportUserSettings creates an exportable settings bundle for a user
func ExportUserSettings(ctx context.Context, userSettings *UserSettingsStore, pubkey string) (*UserSettingsExport, error) {
	settings, err := userSettings.GetSettings(ctx, pubkey)
	if err != nil {
		return nil, err
	}

	export := &UserSettingsExport{
		Pubkey:     pubkey,
		ExportedAt: nostr.Now().Time().Format("2006-01-02T15:04:05Z"),
	}

	if settings != nil {
		export.Haven = &HavenSettingsEventContent{
			BlastrEnabled:    settings.BlastrEnabled,
			BlastrRelays:     settings.BlastrRelays,
			ImporterEnabled:  settings.ImporterEnabled,
			ImporterRelays:   settings.ImporterRelays,
			ImporterRealtime: settings.ImporterRealtime,
		}

		// Include privacy settings if non-default
		if !settings.PublicOutboxRead {
			f := false
			export.Haven.PublicOutboxRead = &f
		}
		if !settings.PublicInboxWrite {
			f := false
			export.Haven.PublicInboxWrite = &f
		}
	}

	return export, nil
}

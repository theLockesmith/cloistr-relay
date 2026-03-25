package haven

import (
	"testing"
	"time"
)

func TestUserSettings_Defaults(t *testing.T) {
	// Default settings should have sensible values
	settings := &UserSettings{
		Pubkey:             "test123",
		BlastrEnabled:      false,
		ImporterEnabled:    false,
		PublicOutboxRead:   true,
		PublicInboxWrite:   true,
		RequireAuthChat:    true,
		RequireAuthPrivate: true,
	}

	if settings.BlastrEnabled {
		t.Error("BlastrEnabled should default to false")
	}
	if settings.ImporterEnabled {
		t.Error("ImporterEnabled should default to false")
	}
	if !settings.PublicOutboxRead {
		t.Error("PublicOutboxRead should default to true")
	}
	if !settings.PublicInboxWrite {
		t.Error("PublicInboxWrite should default to true")
	}
}

func TestUserSettings_RelayLists(t *testing.T) {
	settings := &UserSettings{
		Pubkey:         "test123",
		BlastrEnabled:  true,
		BlastrRelays:   []string{"wss://relay1.com", "wss://relay2.com"},
		ImporterEnabled: true,
		ImporterRelays: []string{"wss://relay3.com"},
	}

	if len(settings.BlastrRelays) != 2 {
		t.Errorf("Expected 2 blastr relays, got %d", len(settings.BlastrRelays))
	}
	if len(settings.ImporterRelays) != 1 {
		t.Errorf("Expected 1 importer relay, got %d", len(settings.ImporterRelays))
	}
}

func TestUserSettings_LastImportTime(t *testing.T) {
	now := time.Now()
	settings := &UserSettings{
		Pubkey:          "test123",
		ImporterEnabled: true,
		LastImportTime:  &now,
	}

	if settings.LastImportTime == nil {
		t.Error("LastImportTime should be set")
	}
	if !settings.LastImportTime.Equal(now) {
		t.Errorf("LastImportTime mismatch: got %v, want %v", *settings.LastImportTime, now)
	}
}

func TestUserSettings_PrivacySettings(t *testing.T) {
	tests := []struct {
		name             string
		publicOutbox     bool
		publicInbox      bool
		authChat         bool
		authPrivate      bool
	}{
		{"all public", true, true, false, false},
		{"all private", false, false, true, true},
		{"mixed", true, false, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := &UserSettings{
				Pubkey:             "test123",
				PublicOutboxRead:   tt.publicOutbox,
				PublicInboxWrite:   tt.publicInbox,
				RequireAuthChat:    tt.authChat,
				RequireAuthPrivate: tt.authPrivate,
			}

			if settings.PublicOutboxRead != tt.publicOutbox {
				t.Errorf("PublicOutboxRead: got %v, want %v", settings.PublicOutboxRead, tt.publicOutbox)
			}
			if settings.PublicInboxWrite != tt.publicInbox {
				t.Errorf("PublicInboxWrite: got %v, want %v", settings.PublicInboxWrite, tt.publicInbox)
			}
		})
	}
}

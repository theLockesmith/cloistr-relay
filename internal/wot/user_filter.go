package wot

import (
	"context"
	"log"

	"github.com/nbd-wtf/go-nostr"
)

// UserFilter applies per-user WoT filtering to events
// It checks user blocklists/trusted lists before allowing events to reach a user's inbox
type UserFilter struct {
	settingsStore *UserSettingsStore
	relayHandler  *Handler // For relay-level WoT checks
}

// NewUserFilter creates a new per-user WoT filter
func NewUserFilter(settingsStore *UserSettingsStore, relayHandler *Handler) *UserFilter {
	return &UserFilter{
		settingsStore: settingsStore,
		relayHandler:  relayHandler,
	}
}

// FilterResult represents the outcome of a filter check
type FilterResult struct {
	// Allowed is true if the event should reach the user
	Allowed bool
	// Reason explains why the event was blocked (empty if allowed)
	Reason string
	// Source indicates what caused the decision
	Source FilterSource
}

// FilterSource indicates what made the filtering decision
type FilterSource string

const (
	// FilterSourceRelayBlock means the relay's global blocklist blocked it
	FilterSourceRelayBlock FilterSource = "relay_block"
	// FilterSourceUserBlock means the user's personal blocklist blocked it
	FilterSourceUserBlock FilterSource = "user_block"
	// FilterSourceUserTrust means the user's personal trusted list allowed it
	FilterSourceUserTrust FilterSource = "user_trust"
	// FilterSourceRelayWoT means the relay's WoT allowed it
	FilterSourceRelayWoT FilterSource = "relay_wot"
	// FilterSourceDefault means no specific rule applied, allowed by default
	FilterSourceDefault FilterSource = "default"
)

// ShouldAllowToInbox checks if an event from senderPubkey should reach recipientPubkey's inbox
// This implements the filter stack:
// 1. Relay WoT floor (global blocks apply)
// 2. User blocklist (user can block anyone)
// 3. User trusted list (user can explicitly trust)
// 4. Relay WoT (standard WoT checks)
func (f *UserFilter) ShouldAllowToInbox(ctx context.Context, event *nostr.Event, recipientPubkey string) FilterResult {
	if event == nil {
		return FilterResult{
			Allowed: true,
			Reason:  "",
			Source:  FilterSourceDefault,
		}
	}

	senderPubkey := event.PubKey

	// Step 1: Check user's blocklist first (user always wins for blocks)
	if f.settingsStore != nil {
		blocked, err := f.settingsStore.IsBlockedBy(ctx, recipientPubkey, senderPubkey)
		if err != nil {
			log.Printf("WoT UserFilter: error checking blocklist: %v", err)
		} else if blocked {
			return FilterResult{
				Allowed: false,
				Reason:  "sender is on recipient's blocklist",
				Source:  FilterSourceUserBlock,
			}
		}

		// Step 2: Check user's trusted list (bypasses relay WoT but not relay blocks)
		trusted, err := f.settingsStore.IsTrustedBy(ctx, recipientPubkey, senderPubkey)
		if err != nil {
			log.Printf("WoT UserFilter: error checking trusted list: %v", err)
		} else if trusted {
			return FilterResult{
				Allowed: true,
				Reason:  "",
				Source:  FilterSourceUserTrust,
			}
		}
	}

	// Step 3: Apply relay-level WoT (if configured)
	// The relay handler already ran in RejectEvent, so we don't re-check here
	// Just return allowed by default
	return FilterResult{
		Allowed: true,
		Reason:  "",
		Source:  FilterSourceDefault,
	}
}

// GetUserSettings retrieves the WoT settings for a user
func (f *UserFilter) GetUserSettings(ctx context.Context, pubkey string) (*UserSettings, error) {
	if f.settingsStore == nil {
		return nil, nil
	}
	return f.settingsStore.GetSettings(ctx, pubkey)
}

// UserFilterStats holds statistics about user WoT filtering
type UserFilterStats struct {
	UsersWithSettings    int
	UsersWithBlocklist   int
	UsersWithTrustedList int
}

// GetStats returns statistics about user WoT usage
func (f *UserFilter) GetStats(ctx context.Context) (*UserFilterStats, error) {
	if f.settingsStore == nil {
		return &UserFilterStats{}, nil
	}

	users, err := f.settingsStore.ListUsersWithSettings(ctx)
	if err != nil {
		return nil, err
	}

	blocklist, err := f.settingsStore.CountUsersWithBlocklist(ctx)
	if err != nil {
		return nil, err
	}

	trusted, err := f.settingsStore.CountUsersWithTrustedlist(ctx)
	if err != nil {
		return nil, err
	}

	return &UserFilterStats{
		UsersWithSettings:    len(users),
		UsersWithBlocklist:   blocklist,
		UsersWithTrustedList: trusted,
	}, nil
}

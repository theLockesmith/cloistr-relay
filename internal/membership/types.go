// Package membership implements NIP-43 relay access and membership
//
// NIP-43 enables relays to manage membership through:
// - Membership lists (kind 13534)
// - Join/leave requests (kinds 28934, 28936)
// - Invite codes (kind 28935)
// - Add/remove notifications (kinds 8000, 8001)
//
// All membership events require a "-" protected tag (NIP-70) to prevent
// relays from propagating them to other relays.
//
// Reference: https://github.com/nostr-protocol/nips/blob/master/43.md
package membership

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

// Event kinds for NIP-43
const (
	// KindMembershipList is published by relays to advertise members
	KindMembershipList = 13534

	// KindAddMember notification when a user joins
	KindAddMember = 8000

	// KindRemoveMember notification when a user leaves
	KindRemoveMember = 8001

	// KindJoinRequest from users wanting to join
	KindJoinRequest = 28934

	// KindInviteResponse ephemeral invite code response
	KindInviteResponse = 28935

	// KindLeaveRequest from users wanting to leave
	KindLeaveRequest = 28936
)

// MemberTier represents a membership tier with different feature access
type MemberTier string

const (
	// TierFree is the default tier with basic access
	TierFree MemberTier = "free"
	// TierHybrid unlocks HAVEN features with limited relays
	TierHybrid MemberTier = "hybrid"
	// TierPremium unlocks full HAVEN features
	TierPremium MemberTier = "premium"
	// TierEnterprise unlocks unlimited features
	TierEnterprise MemberTier = "enterprise"
)

// TierLimits defines feature access for each tier
type TierLimits struct {
	// HasHavenBoxes enables per-user inbox/outbox/private/chat
	HasHavenBoxes bool
	// HasBlastr enables outbox broadcasting
	HasBlastr bool
	// HasImporter enables inbox event fetching
	HasImporter bool
	// HasWoTControl enables user WoT customization
	HasWoTControl bool
	// MaxBlastrRelays limits broadcast relays (0 = unlimited)
	MaxBlastrRelays int
	// MaxImporterRelays limits import relays (0 = unlimited)
	MaxImporterRelays int
}

// TierConfig maps tiers to their feature limits
var TierConfig = map[MemberTier]TierLimits{
	TierFree:       {false, false, false, false, 0, 0},
	TierHybrid:     {true, true, true, true, 3, 3},
	TierPremium:    {true, true, true, true, 10, 10},
	TierEnterprise: {true, true, true, true, 0, 0},
}

// GetLimits returns the feature limits for a tier
func (t MemberTier) GetLimits() TierLimits {
	if limits, ok := TierConfig[t]; ok {
		return limits
	}
	return TierConfig[TierFree]
}

// IsValid returns true if the tier is a known tier
func (t MemberTier) IsValid() bool {
	_, ok := TierConfig[t]
	return ok
}

// Member represents a relay member
type Member struct {
	// Pubkey is the member's hex pubkey
	Pubkey string
	// JoinedAt is when they joined
	JoinedAt time.Time
	// InviteCode is the code they used (if any)
	InviteCode string
	// AddedBy is the pubkey that added them (admin or via invite)
	AddedBy string
	// Tier is the member's subscription tier
	Tier MemberTier
	// TierExpiresAt is when the paid tier expires (zero = never)
	TierExpiresAt time.Time
	// LightningAddress for payments/refunds
	LightningAddress string
}

// GetEffectiveTier returns the member's tier, accounting for expiration
func (m *Member) GetEffectiveTier() MemberTier {
	if m.Tier == "" || m.Tier == TierFree {
		return TierFree
	}
	// Check if tier has expired
	if !m.TierExpiresAt.IsZero() && time.Now().After(m.TierExpiresAt) {
		return TierFree
	}
	return m.Tier
}

// GetLimits returns the feature limits for this member's effective tier
func (m *Member) GetLimits() TierLimits {
	return m.GetEffectiveTier().GetLimits()
}

// Invite represents an invite code
type Invite struct {
	// Code is the invite string
	Code string
	// CreatedAt is when the invite was created
	CreatedAt time.Time
	// ExpiresAt is when the invite expires (zero = never)
	ExpiresAt time.Time
	// MaxUses is maximum uses (0 = unlimited)
	MaxUses int
	// Uses is current use count
	Uses int
	// CreatedBy is the pubkey that created the invite
	CreatedBy string
}

// Config holds membership configuration
type Config struct {
	// Enabled activates NIP-43 membership management
	Enabled bool
	// RelayPrivateKey is the relay's signing key (hex)
	// Required for signing membership events
	RelayPrivateKey string
	// RelayPubkey is derived from the private key
	RelayPubkey string
	// RequireMembership if true, only members can access the relay
	RequireMembership bool
	// AllowJoinRequests if true, users can request to join
	AllowJoinRequests bool
	// PublishMembershipList if true, publish kind 13534 events
	PublishMembershipList bool
	// DefaultInviteExpiry is default invite expiration
	DefaultInviteExpiry time.Duration
	// DefaultInviteMaxUses is default max uses for invites
	DefaultInviteMaxUses int
	// AdminPubkeys can manage membership
	AdminPubkeys []string
}

// DefaultConfig returns sensible defaults for membership
func DefaultConfig() *Config {
	return &Config{
		Enabled:               false,
		RequireMembership:     false,
		AllowJoinRequests:     true,
		PublishMembershipList: false,
		DefaultInviteExpiry:   7 * 24 * time.Hour, // 1 week
		DefaultInviteMaxUses:  1,
	}
}

// GenerateInviteCode creates a random invite code
func GenerateInviteCode() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// IsValidInvite checks if an invite is valid
func (i *Invite) IsValidInvite() bool {
	// Check if expired
	if !i.ExpiresAt.IsZero() && time.Now().After(i.ExpiresAt) {
		return false
	}
	// Check if max uses reached
	if i.MaxUses > 0 && i.Uses >= i.MaxUses {
		return false
	}
	return true
}

// CreateMembershipListEvent creates a kind 13534 event with current members
func CreateMembershipListEvent(relayPubkey string, members []Member) *nostr.Event {
	event := &nostr.Event{
		Kind:      KindMembershipList,
		PubKey:    relayPubkey,
		CreatedAt: nostr.Now(),
		Content:   "",
		Tags:      nostr.Tags{{"-"}}, // NIP-70 protected
	}

	for _, member := range members {
		event.Tags = append(event.Tags, nostr.Tag{"member", member.Pubkey})
	}

	return event
}

// CreateAddMemberEvent creates a kind 8000 notification
func CreateAddMemberEvent(relayPubkey, memberPubkey string) *nostr.Event {
	return &nostr.Event{
		Kind:      KindAddMember,
		PubKey:    relayPubkey,
		CreatedAt: nostr.Now(),
		Content:   "",
		Tags: nostr.Tags{
			{"-"},                    // NIP-70 protected
			{"p", memberPubkey},
		},
	}
}

// CreateRemoveMemberEvent creates a kind 8001 notification
func CreateRemoveMemberEvent(relayPubkey, memberPubkey string) *nostr.Event {
	return &nostr.Event{
		Kind:      KindRemoveMember,
		PubKey:    relayPubkey,
		CreatedAt: nostr.Now(),
		Content:   "",
		Tags: nostr.Tags{
			{"-"},                    // NIP-70 protected
			{"p", memberPubkey},
		},
	}
}

// CreateJoinRequestEvent creates a kind 28934 join request
func CreateJoinRequestEvent(userPubkey, inviteCode string) *nostr.Event {
	tags := nostr.Tags{
		{"-"}, // NIP-70 protected
		{"p", userPubkey},
	}

	if inviteCode != "" {
		tags = append(tags, nostr.Tag{"claim", inviteCode})
	}

	return &nostr.Event{
		Kind:      KindJoinRequest,
		PubKey:    userPubkey,
		CreatedAt: nostr.Now(),
		Content:   "",
		Tags:      tags,
	}
}

// CreateInviteResponseEvent creates a kind 28935 ephemeral invite response
func CreateInviteResponseEvent(relayPubkey, inviteCode, requestPubkey string) *nostr.Event {
	return &nostr.Event{
		Kind:      KindInviteResponse,
		PubKey:    relayPubkey,
		CreatedAt: nostr.Now(),
		Content:   "",
		Tags: nostr.Tags{
			{"-"},                    // NIP-70 protected
			{"p", requestPubkey},
			{"claim", inviteCode},
		},
	}
}

// CreateLeaveRequestEvent creates a kind 28936 leave request
func CreateLeaveRequestEvent(userPubkey string) *nostr.Event {
	return &nostr.Event{
		Kind:      KindLeaveRequest,
		PubKey:    userPubkey,
		CreatedAt: nostr.Now(),
		Content:   "",
		Tags: nostr.Tags{
			{"-"}, // NIP-70 protected
			{"p", userPubkey},
		},
	}
}

// ParseJoinRequest extracts info from a join request event
func ParseJoinRequest(event *nostr.Event) (pubkey, inviteCode string) {
	pubkey = event.PubKey

	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "claim" {
			inviteCode = tag[1]
			break
		}
	}

	return pubkey, inviteCode
}

// IsMembershipKind returns true if the kind is NIP-43 related
func IsMembershipKind(kind int) bool {
	switch kind {
	case KindMembershipList, KindAddMember, KindRemoveMember,
		KindJoinRequest, KindInviteResponse, KindLeaveRequest:
		return true
	default:
		return false
	}
}

// HasProtectedTag checks if event has the NIP-70 "-" tag
func HasProtectedTag(event *nostr.Event) bool {
	for _, tag := range event.Tags {
		if len(tag) >= 1 && tag[0] == "-" {
			return true
		}
	}
	return false
}

// Package groups implements NIP-29 relay-based groups
//
// NIP-29 provides a framework for closed-membership group communication
// where groups are hosted on specific relays and identified by random strings.
//
// Reference: https://github.com/nostr-protocol/nips/blob/master/29.md
package groups

import (
	"time"
)

// Event kinds for NIP-29

// User management event kinds
const (
	KindJoinRequest  = 9021 // Request to join a group
	KindLeaveRequest = 9022 // Request to leave a group
)

// Moderation event kinds (9000-9020)
const (
	KindAddUser       = 9000 // Add user to group with role
	KindRemoveUser    = 9001 // Remove user from group
	KindEditMetadata  = 9002 // Edit group metadata
	KindDeleteEvent   = 9005 // Delete an event from group
	KindCreateGroup   = 9007 // Create a new group
	KindDeleteGroup   = 9008 // Delete a group
	KindCreateInvite  = 9009 // Create invite code
)

// Relay-generated metadata event kinds
const (
	KindGroupMetadata = 39000 // Group metadata (name, picture, about)
	KindGroupAdmins   = 39001 // Admin list with roles
	KindGroupMembers  = 39002 // Member list
	KindGroupRoles    = 39003 // Supported roles
)

// Privacy settings for groups
type Privacy string

const (
	// PrivacyOpen - anyone can read and write (default)
	PrivacyOpen Privacy = "open"
	// PrivacyRestricted - anyone can read, only members can write
	PrivacyRestricted Privacy = "restricted"
	// PrivacyPrivate - only members can read and write
	PrivacyPrivate Privacy = "private"
	// PrivacyHidden - private + metadata hidden from non-members
	PrivacyHidden Privacy = "hidden"
	// PrivacyClosed - like private but no join requests allowed
	PrivacyClosed Privacy = "closed"
)

// Role represents a role that can be assigned to group members
type Role struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
}

// DefaultRoles are the standard roles for groups
var DefaultRoles = []Role{
	{Name: "admin", Description: "Full control over group", Permissions: []string{"*"}},
	{Name: "moderator", Description: "Can moderate content and users", Permissions: []string{"delete-event", "remove-user"}},
	{Name: "member", Description: "Can post to the group", Permissions: []string{"post"}},
}

// Group represents a NIP-29 group
type Group struct {
	ID          string            `json:"id"`           // Random group identifier
	Name        string            `json:"name"`         // Display name
	Picture     string            `json:"picture,omitempty"` // Group picture URL
	About       string            `json:"about,omitempty"`   // Description
	Privacy     Privacy           `json:"privacy"`      // Privacy level
	CreatedAt   time.Time         `json:"created_at"`
	CreatedBy   string            `json:"created_by"`   // Pubkey of creator
	RelayURL    string            `json:"relay_url"`    // Hosting relay URL
	Admins      map[string]string `json:"admins"`       // pubkey -> role
	Members     map[string]bool   `json:"members"`      // pubkey -> true
	InviteCodes map[string]InviteCode `json:"invite_codes,omitempty"`
}

// InviteCode represents an invite to join a group
type InviteCode struct {
	Code      string    `json:"code"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	MaxUses   int       `json:"max_uses,omitempty"`
	Uses      int       `json:"uses"`
}

// Member represents a group member
type Member struct {
	Pubkey   string    `json:"pubkey"`
	Role     string    `json:"role,omitempty"` // Empty = regular member
	JoinedAt time.Time `json:"joined_at"`
	AddedBy  string    `json:"added_by,omitempty"`
}

// GroupMetadata represents the metadata stored in kind 39000
type GroupMetadata struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Picture  string  `json:"picture,omitempty"`
	About    string  `json:"about,omitempty"`
	Privacy  Privacy `json:"privacy"`
	Open     bool    `json:"open,omitempty"`     // Deprecated: use privacy
	Closed   bool    `json:"closed,omitempty"`   // Deprecated: use privacy
	Private  bool    `json:"private,omitempty"`  // Deprecated: use privacy
}

// Config holds NIP-29 groups configuration
type Config struct {
	// Enabled activates NIP-29 groups support
	Enabled bool

	// RelayURL is the URL of this relay (used in group metadata)
	RelayURL string

	// AdminPubkeys are pubkeys that can create groups on this relay
	AdminPubkeys []string

	// AllowPublicGroupCreation allows anyone to create groups
	AllowPublicGroupCreation bool

	// MaxGroupsPerUser limits how many groups a user can create
	MaxGroupsPerUser int

	// DefaultPrivacy is the default privacy level for new groups
	DefaultPrivacy Privacy

	// InviteCodeExpiry is the default expiry time for invite codes
	InviteCodeExpiry time.Duration
}

// DefaultConfig returns sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Enabled:                  false,
		AllowPublicGroupCreation: false,
		MaxGroupsPerUser:         10,
		DefaultPrivacy:           PrivacyRestricted,
		InviteCodeExpiry:         7 * 24 * time.Hour, // 1 week
	}
}

// IsModeratorKind returns true if the kind is a moderation event
func IsModeratorKind(kind int) bool {
	return kind >= 9000 && kind <= 9020
}

// IsGroupMetadataKind returns true if the kind is a group metadata event
func IsGroupMetadataKind(kind int) bool {
	return kind >= 39000 && kind <= 39009
}

// IsGroupManagementKind returns true if the kind is for user management
func IsGroupManagementKind(kind int) bool {
	return kind == KindJoinRequest || kind == KindLeaveRequest
}

// CanRead returns true if the privacy level allows reading
func (p Privacy) CanRead(isMember bool) bool {
	switch p {
	case PrivacyOpen, PrivacyRestricted:
		return true
	case PrivacyPrivate, PrivacyHidden, PrivacyClosed:
		return isMember
	default:
		return true
	}
}

// CanWrite returns true if the privacy level allows writing
func (p Privacy) CanWrite(isMember bool) bool {
	switch p {
	case PrivacyOpen:
		return true
	case PrivacyRestricted, PrivacyPrivate, PrivacyHidden, PrivacyClosed:
		return isMember
	default:
		return isMember
	}
}

// CanJoin returns true if the privacy level allows join requests
func (p Privacy) CanJoin() bool {
	return p != PrivacyClosed
}

// ShowMetadata returns true if metadata should be visible
func (p Privacy) ShowMetadata(isMember bool) bool {
	if p == PrivacyHidden {
		return isMember
	}
	return true
}

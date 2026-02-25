package groups

import (
	"testing"
)

func TestPrivacy_CanRead(t *testing.T) {
	tests := []struct {
		privacy  Privacy
		isMember bool
		expected bool
	}{
		{PrivacyOpen, false, true},
		{PrivacyOpen, true, true},
		{PrivacyRestricted, false, true},
		{PrivacyRestricted, true, true},
		{PrivacyPrivate, false, false},
		{PrivacyPrivate, true, true},
		{PrivacyHidden, false, false},
		{PrivacyHidden, true, true},
		{PrivacyClosed, false, false},
		{PrivacyClosed, true, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.privacy), func(t *testing.T) {
			result := tt.privacy.CanRead(tt.isMember)
			if result != tt.expected {
				t.Errorf("Privacy(%s).CanRead(%v) = %v, want %v",
					tt.privacy, tt.isMember, result, tt.expected)
			}
		})
	}
}

func TestPrivacy_CanWrite(t *testing.T) {
	tests := []struct {
		privacy  Privacy
		isMember bool
		expected bool
	}{
		{PrivacyOpen, false, true},
		{PrivacyOpen, true, true},
		{PrivacyRestricted, false, false},
		{PrivacyRestricted, true, true},
		{PrivacyPrivate, false, false},
		{PrivacyPrivate, true, true},
		{PrivacyHidden, false, false},
		{PrivacyHidden, true, true},
		{PrivacyClosed, false, false},
		{PrivacyClosed, true, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.privacy), func(t *testing.T) {
			result := tt.privacy.CanWrite(tt.isMember)
			if result != tt.expected {
				t.Errorf("Privacy(%s).CanWrite(%v) = %v, want %v",
					tt.privacy, tt.isMember, result, tt.expected)
			}
		})
	}
}

func TestPrivacy_CanJoin(t *testing.T) {
	tests := []struct {
		privacy  Privacy
		expected bool
	}{
		{PrivacyOpen, true},
		{PrivacyRestricted, true},
		{PrivacyPrivate, true},
		{PrivacyHidden, true},
		{PrivacyClosed, false}, // Closed groups don't allow join requests
	}

	for _, tt := range tests {
		t.Run(string(tt.privacy), func(t *testing.T) {
			result := tt.privacy.CanJoin()
			if result != tt.expected {
				t.Errorf("Privacy(%s).CanJoin() = %v, want %v",
					tt.privacy, result, tt.expected)
			}
		})
	}
}

func TestPrivacy_ShowMetadata(t *testing.T) {
	tests := []struct {
		privacy  Privacy
		isMember bool
		expected bool
	}{
		{PrivacyOpen, false, true},
		{PrivacyOpen, true, true},
		{PrivacyRestricted, false, true},
		{PrivacyRestricted, true, true},
		{PrivacyPrivate, false, true},
		{PrivacyPrivate, true, true},
		{PrivacyHidden, false, false}, // Hidden groups hide metadata from non-members
		{PrivacyHidden, true, true},
		{PrivacyClosed, false, true},
		{PrivacyClosed, true, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.privacy), func(t *testing.T) {
			result := tt.privacy.ShowMetadata(tt.isMember)
			if result != tt.expected {
				t.Errorf("Privacy(%s).ShowMetadata(%v) = %v, want %v",
					tt.privacy, tt.isMember, result, tt.expected)
			}
		})
	}
}

func TestIsModeratorKind(t *testing.T) {
	tests := []struct {
		kind     int
		expected bool
	}{
		{8999, false},
		{9000, true},  // KindAddUser
		{9001, true},  // KindRemoveUser
		{9002, true},  // KindEditMetadata
		{9005, true},  // KindDeleteEvent
		{9007, true},  // KindCreateGroup
		{9008, true},  // KindDeleteGroup
		{9009, true},  // KindCreateInvite
		{9020, true},  // Upper bound
		{9021, false}, // KindJoinRequest (user management, not moderation)
		{9022, false}, // KindLeaveRequest
		{1, false},
		{39000, false},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := IsModeratorKind(tt.kind)
			if result != tt.expected {
				t.Errorf("IsModeratorKind(%d) = %v, want %v", tt.kind, result, tt.expected)
			}
		})
	}
}

func TestIsGroupMetadataKind(t *testing.T) {
	tests := []struct {
		kind     int
		expected bool
	}{
		{38999, false},
		{39000, true}, // KindGroupMetadata
		{39001, true}, // KindGroupAdmins
		{39002, true}, // KindGroupMembers
		{39003, true}, // KindGroupRoles
		{39009, true}, // Upper bound
		{39010, false},
		{1, false},
		{9000, false},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := IsGroupMetadataKind(tt.kind)
			if result != tt.expected {
				t.Errorf("IsGroupMetadataKind(%d) = %v, want %v", tt.kind, result, tt.expected)
			}
		})
	}
}

func TestIsGroupManagementKind(t *testing.T) {
	tests := []struct {
		kind     int
		expected bool
	}{
		{KindJoinRequest, true},
		{KindLeaveRequest, true},
		{9000, false},
		{9001, false},
		{1, false},
		{39000, false},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := IsGroupManagementKind(tt.kind)
			if result != tt.expected {
				t.Errorf("IsGroupManagementKind(%d) = %v, want %v", tt.kind, result, tt.expected)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}

	if cfg.Enabled {
		t.Error("DefaultConfig should have Enabled = false")
	}

	if cfg.AllowPublicGroupCreation {
		t.Error("DefaultConfig should not allow public group creation")
	}

	if cfg.MaxGroupsPerUser != 10 {
		t.Errorf("DefaultConfig MaxGroupsPerUser = %d, want 10", cfg.MaxGroupsPerUser)
	}

	if cfg.DefaultPrivacy != PrivacyRestricted {
		t.Errorf("DefaultConfig DefaultPrivacy = %s, want %s", cfg.DefaultPrivacy, PrivacyRestricted)
	}

	// Should be 1 week
	if cfg.InviteCodeExpiry.Hours() < 167 || cfg.InviteCodeExpiry.Hours() > 169 {
		t.Errorf("DefaultConfig InviteCodeExpiry = %v, want ~168h", cfg.InviteCodeExpiry)
	}
}

func TestDefaultRoles(t *testing.T) {
	if len(DefaultRoles) != 3 {
		t.Errorf("DefaultRoles has %d roles, want 3", len(DefaultRoles))
	}

	// Check admin role
	found := false
	for _, role := range DefaultRoles {
		if role.Name == "admin" {
			found = true
			if len(role.Permissions) != 1 || role.Permissions[0] != "*" {
				t.Error("Admin role should have '*' permission")
			}
		}
	}
	if !found {
		t.Error("DefaultRoles should include admin role")
	}
}

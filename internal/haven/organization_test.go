package haven

import (
	"testing"
	"time"
)

func TestOrganization_Fields(t *testing.T) {
	org := Organization{
		ID:               "org-123",
		Name:             "Acme Corp",
		OwnerPubkey:      alicePubkey,
		Tier:             "enterprise",
		MemberLimit:      100,
		LightningAddress: "acme@getalby.com",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	if org.ID != "org-123" {
		t.Error("ID mismatch")
	}
	if org.Name != "Acme Corp" {
		t.Error("Name mismatch")
	}
	if org.OwnerPubkey != alicePubkey {
		t.Error("OwnerPubkey mismatch")
	}
	if org.Tier != "enterprise" {
		t.Error("Tier mismatch")
	}
	if org.MemberLimit != 100 {
		t.Error("MemberLimit mismatch")
	}
	if org.LightningAddress != "acme@getalby.com" {
		t.Error("LightningAddress mismatch")
	}
}

func TestOrgMember_Fields(t *testing.T) {
	member := OrgMember{
		OrgID:        "org-123",
		Pubkey:       bobPubkey,
		Role:         OrgRoleMember,
		InheritsTier: true,
		JoinedAt:     time.Now(),
	}

	if member.OrgID != "org-123" {
		t.Error("OrgID mismatch")
	}
	if member.Pubkey != bobPubkey {
		t.Error("Pubkey mismatch")
	}
	if member.Role != OrgRoleMember {
		t.Error("Role mismatch")
	}
	if !member.InheritsTier {
		t.Error("InheritsTier should be true")
	}
}

func TestOrgRole_Constants(t *testing.T) {
	if OrgRoleAdmin != "admin" {
		t.Error("OrgRoleAdmin should be 'admin'")
	}
	if OrgRoleMember != "member" {
		t.Error("OrgRoleMember should be 'member'")
	}
}

func TestOrgSettings_Fields(t *testing.T) {
	depth := 2
	settings := OrgSettings{
		OrgID:             "org-123",
		InternalRelayOnly: true,
		SharedOutbox:      true,
		WoTBaseline: &WoTSettingsContent{
			BlockedPubkeys: []string{"blocked1"},
			TrustedPubkeys: []string{"trusted1"},
			MaxTrustDepth:  &depth,
		},
	}

	if settings.OrgID != "org-123" {
		t.Error("OrgID mismatch")
	}
	if !settings.InternalRelayOnly {
		t.Error("InternalRelayOnly should be true")
	}
	if !settings.SharedOutbox {
		t.Error("SharedOutbox should be true")
	}
	if settings.WoTBaseline == nil {
		t.Fatal("WoTBaseline should not be nil")
	}
	if len(settings.WoTBaseline.BlockedPubkeys) != 1 {
		t.Error("BlockedPubkeys length mismatch")
	}
	if len(settings.WoTBaseline.TrustedPubkeys) != 1 {
		t.Error("TrustedPubkeys length mismatch")
	}
	if settings.WoTBaseline.MaxTrustDepth == nil || *settings.WoTBaseline.MaxTrustDepth != 2 {
		t.Error("MaxTrustDepth mismatch")
	}
}

func TestNewOrgStore(t *testing.T) {
	store := NewOrgStore(nil)
	if store == nil {
		t.Fatal("NewOrgStore returned nil")
	}
}

func TestOrgMember_AdminRole(t *testing.T) {
	admin := OrgMember{
		OrgID:        "org-123",
		Pubkey:       alicePubkey,
		Role:         OrgRoleAdmin,
		InheritsTier: true,
	}

	if admin.Role != OrgRoleAdmin {
		t.Error("Admin role not set correctly")
	}
}

func TestOrgMember_NoTierInheritance(t *testing.T) {
	member := OrgMember{
		OrgID:        "org-123",
		Pubkey:       bobPubkey,
		Role:         OrgRoleMember,
		InheritsTier: false, // Uses personal tier
	}

	if member.InheritsTier {
		t.Error("InheritsTier should be false")
	}
}

func TestOrganization_UnlimitedMembers(t *testing.T) {
	org := Organization{
		ID:          "org-123",
		Name:        "Unlimited Org",
		OwnerPubkey: alicePubkey,
		Tier:        "enterprise",
		MemberLimit: 0, // 0 = unlimited
	}

	if org.MemberLimit != 0 {
		t.Error("MemberLimit should be 0 for unlimited")
	}
}

func TestOrgSettings_EmptyWoT(t *testing.T) {
	settings := OrgSettings{
		OrgID:             "org-123",
		InternalRelayOnly: false,
		SharedOutbox:      false,
		WoTBaseline:       nil, // No WoT baseline
	}

	if settings.WoTBaseline != nil {
		t.Error("WoTBaseline should be nil")
	}
}

func TestOrganization_Tiers(t *testing.T) {
	tests := []struct {
		tier     string
		expected string
	}{
		{"free", "free"},
		{"hybrid", "hybrid"},
		{"premium", "premium"},
		{"enterprise", "enterprise"},
	}

	for _, tt := range tests {
		t.Run(tt.tier, func(t *testing.T) {
			org := Organization{
				ID:          "org-" + tt.tier,
				Name:        "Test Org",
				OwnerPubkey: alicePubkey,
				Tier:        tt.tier,
			}
			if org.Tier != tt.expected {
				t.Errorf("Tier = %s, want %s", org.Tier, tt.expected)
			}
		})
	}
}

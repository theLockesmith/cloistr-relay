package haven

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
)

// OrgRole defines the role of a member within an organization
type OrgRole string

const (
	OrgRoleAdmin  OrgRole = "admin"  // Can manage members, settings
	OrgRoleMember OrgRole = "member" // Uses org features
)

// Organization represents a B2B organization that can manage multiple members
type Organization struct {
	ID               string
	Name             string
	OwnerPubkey      string    // Admin who manages the org
	Tier             string    // Org-wide tier (typically enterprise)
	MemberLimit      int       // Max members (0 = unlimited)
	LightningAddress string    // Billing address
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// OrgMember represents a member's association with an organization
type OrgMember struct {
	OrgID        string
	Pubkey       string
	Role         OrgRole
	InheritsTier bool // Uses org tier vs personal tier
	JoinedAt     time.Time
}

// OrgSettings holds organization-wide settings
type OrgSettings struct {
	OrgID             string
	InternalRelayOnly bool             // Events stay on this relay
	SharedOutbox      bool             // Org members see each other's outbox
	WoTBaseline       *WoTSettingsContent // Org-wide trust defaults
}

// OrgStore handles persistence of organizations and their members
type OrgStore struct {
	db *sql.DB
}

// NewOrgStore creates a new organization store
func NewOrgStore(db *sql.DB) *OrgStore {
	return &OrgStore{db: db}
}

// Init creates the organization tables
func (s *OrgStore) Init(ctx context.Context) error {
	schema := `
		CREATE TABLE IF NOT EXISTS organizations (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			owner_pubkey TEXT NOT NULL,
			tier TEXT DEFAULT 'enterprise',
			member_limit INT DEFAULT 0,
			lightning_address TEXT,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_organizations_owner ON organizations(owner_pubkey);

		CREATE TABLE IF NOT EXISTS org_members (
			org_id TEXT REFERENCES organizations(id) ON DELETE CASCADE,
			pubkey TEXT NOT NULL,
			role TEXT DEFAULT 'member',
			inherits_tier BOOLEAN DEFAULT true,
			joined_at TIMESTAMPTZ DEFAULT NOW(),
			PRIMARY KEY (org_id, pubkey)
		);

		CREATE INDEX IF NOT EXISTS idx_org_members_pubkey ON org_members(pubkey);

		CREATE TABLE IF NOT EXISTS org_settings (
			org_id TEXT PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
			internal_relay_only BOOLEAN DEFAULT false,
			shared_outbox BOOLEAN DEFAULT false,
			wot_blocked_pubkeys TEXT[] DEFAULT '{}',
			wot_trusted_pubkeys TEXT[] DEFAULT '{}',
			wot_max_trust_depth INT,
			updated_at TIMESTAMPTZ DEFAULT NOW()
		);
	`
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// CreateOrganization creates a new organization
func (s *OrgStore) CreateOrganization(ctx context.Context, org *Organization) error {
	if org.ID == "" {
		return errors.New("organization ID is required")
	}
	if org.Name == "" {
		return errors.New("organization name is required")
	}
	if org.OwnerPubkey == "" {
		return errors.New("owner pubkey is required")
	}

	query := `
		INSERT INTO organizations (id, name, owner_pubkey, tier, member_limit, lightning_address)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := s.db.ExecContext(ctx, query,
		org.ID, org.Name, org.OwnerPubkey, org.Tier, org.MemberLimit, org.LightningAddress)
	return err
}

// GetOrganization retrieves an organization by ID
func (s *OrgStore) GetOrganization(ctx context.Context, id string) (*Organization, error) {
	query := `
		SELECT id, name, owner_pubkey, tier, member_limit, lightning_address, created_at, updated_at
		FROM organizations WHERE id = $1
	`
	org := &Organization{}
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&org.ID, &org.Name, &org.OwnerPubkey, &org.Tier,
		&org.MemberLimit, &org.LightningAddress, &org.CreatedAt, &org.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return org, nil
}

// GetOrganizationByOwner retrieves organizations owned by a pubkey
func (s *OrgStore) GetOrganizationByOwner(ctx context.Context, ownerPubkey string) ([]*Organization, error) {
	query := `
		SELECT id, name, owner_pubkey, tier, member_limit, lightning_address, created_at, updated_at
		FROM organizations WHERE owner_pubkey = $1
		ORDER BY created_at DESC
	`
	rows, err := s.db.QueryContext(ctx, query, ownerPubkey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orgs []*Organization
	for rows.Next() {
		org := &Organization{}
		if err := rows.Scan(&org.ID, &org.Name, &org.OwnerPubkey, &org.Tier,
			&org.MemberLimit, &org.LightningAddress, &org.CreatedAt, &org.UpdatedAt); err != nil {
			return nil, err
		}
		orgs = append(orgs, org)
	}
	return orgs, rows.Err()
}

// UpdateOrganization updates an organization's details
func (s *OrgStore) UpdateOrganization(ctx context.Context, org *Organization) error {
	query := `
		UPDATE organizations
		SET name = $2, tier = $3, member_limit = $4, lightning_address = $5, updated_at = NOW()
		WHERE id = $1
	`
	result, err := s.db.ExecContext(ctx, query,
		org.ID, org.Name, org.Tier, org.MemberLimit, org.LightningAddress)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errors.New("organization not found")
	}
	return nil
}

// DeleteOrganization removes an organization and all its members
func (s *OrgStore) DeleteOrganization(ctx context.Context, id string) error {
	query := `DELETE FROM organizations WHERE id = $1`
	_, err := s.db.ExecContext(ctx, query, id)
	return err
}

// AddMember adds a member to an organization
func (s *OrgStore) AddMember(ctx context.Context, member *OrgMember) error {
	// Check member limit
	org, err := s.GetOrganization(ctx, member.OrgID)
	if err != nil {
		return err
	}
	if org == nil {
		return errors.New("organization not found")
	}

	if org.MemberLimit > 0 {
		count, err := s.GetMemberCount(ctx, member.OrgID)
		if err != nil {
			return err
		}
		if count >= org.MemberLimit {
			return errors.New("organization member limit reached")
		}
	}

	query := `
		INSERT INTO org_members (org_id, pubkey, role, inherits_tier)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (org_id, pubkey) DO UPDATE
		SET role = $3, inherits_tier = $4
	`
	_, err = s.db.ExecContext(ctx, query,
		member.OrgID, member.Pubkey, member.Role, member.InheritsTier)
	return err
}

// RemoveMember removes a member from an organization
func (s *OrgStore) RemoveMember(ctx context.Context, orgID, pubkey string) error {
	query := `DELETE FROM org_members WHERE org_id = $1 AND pubkey = $2`
	_, err := s.db.ExecContext(ctx, query, orgID, pubkey)
	return err
}

// GetMember retrieves a specific org membership
func (s *OrgStore) GetMember(ctx context.Context, orgID, pubkey string) (*OrgMember, error) {
	query := `
		SELECT org_id, pubkey, role, inherits_tier, joined_at
		FROM org_members WHERE org_id = $1 AND pubkey = $2
	`
	member := &OrgMember{}
	err := s.db.QueryRowContext(ctx, query, orgID, pubkey).Scan(
		&member.OrgID, &member.Pubkey, &member.Role, &member.InheritsTier, &member.JoinedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return member, nil
}

// GetMembership retrieves all org memberships for a pubkey
func (s *OrgStore) GetMembership(ctx context.Context, pubkey string) ([]*OrgMember, error) {
	query := `
		SELECT org_id, pubkey, role, inherits_tier, joined_at
		FROM org_members WHERE pubkey = $1
	`
	rows, err := s.db.QueryContext(ctx, query, pubkey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []*OrgMember
	for rows.Next() {
		member := &OrgMember{}
		if err := rows.Scan(&member.OrgID, &member.Pubkey, &member.Role,
			&member.InheritsTier, &member.JoinedAt); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

// GetOrgMembers retrieves all members of an organization
func (s *OrgStore) GetOrgMembers(ctx context.Context, orgID string) ([]*OrgMember, error) {
	query := `
		SELECT org_id, pubkey, role, inherits_tier, joined_at
		FROM org_members WHERE org_id = $1
		ORDER BY joined_at ASC
	`
	rows, err := s.db.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []*OrgMember
	for rows.Next() {
		member := &OrgMember{}
		if err := rows.Scan(&member.OrgID, &member.Pubkey, &member.Role,
			&member.InheritsTier, &member.JoinedAt); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

// GetMemberCount returns the number of members in an organization
func (s *OrgStore) GetMemberCount(ctx context.Context, orgID string) (int, error) {
	query := `SELECT COUNT(*) FROM org_members WHERE org_id = $1`
	var count int
	err := s.db.QueryRowContext(ctx, query, orgID).Scan(&count)
	return count, err
}

// GetOrgSettings retrieves organization settings
func (s *OrgStore) GetOrgSettings(ctx context.Context, orgID string) (*OrgSettings, error) {
	query := `
		SELECT org_id, internal_relay_only, shared_outbox,
		       wot_blocked_pubkeys, wot_trusted_pubkeys, wot_max_trust_depth
		FROM org_settings WHERE org_id = $1
	`
	settings := &OrgSettings{WoTBaseline: &WoTSettingsContent{}}
	var blockedPubkeys, trustedPubkeys pq.StringArray
	var maxTrustDepth sql.NullInt32

	err := s.db.QueryRowContext(ctx, query, orgID).Scan(
		&settings.OrgID, &settings.InternalRelayOnly, &settings.SharedOutbox,
		&blockedPubkeys, &trustedPubkeys, &maxTrustDepth)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	settings.WoTBaseline.BlockedPubkeys = blockedPubkeys
	settings.WoTBaseline.TrustedPubkeys = trustedPubkeys
	if maxTrustDepth.Valid {
		depth := int(maxTrustDepth.Int32)
		settings.WoTBaseline.MaxTrustDepth = &depth
	}

	return settings, nil
}

// SaveOrgSettings saves or updates organization settings
func (s *OrgStore) SaveOrgSettings(ctx context.Context, settings *OrgSettings) error {
	var blockedPubkeys, trustedPubkeys pq.StringArray
	var maxTrustDepth sql.NullInt32

	if settings.WoTBaseline != nil {
		blockedPubkeys = settings.WoTBaseline.BlockedPubkeys
		trustedPubkeys = settings.WoTBaseline.TrustedPubkeys
		if settings.WoTBaseline.MaxTrustDepth != nil {
			maxTrustDepth = sql.NullInt32{Int32: int32(*settings.WoTBaseline.MaxTrustDepth), Valid: true}
		}
	}

	query := `
		INSERT INTO org_settings (org_id, internal_relay_only, shared_outbox,
		                          wot_blocked_pubkeys, wot_trusted_pubkeys, wot_max_trust_depth)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (org_id) DO UPDATE
		SET internal_relay_only = $2, shared_outbox = $3,
		    wot_blocked_pubkeys = $4, wot_trusted_pubkeys = $5, wot_max_trust_depth = $6,
		    updated_at = NOW()
	`
	_, err := s.db.ExecContext(ctx, query,
		settings.OrgID, settings.InternalRelayOnly, settings.SharedOutbox,
		blockedPubkeys, trustedPubkeys, maxTrustDepth)
	return err
}

// GetEffectiveTierForPubkey returns the effective tier for a pubkey,
// checking org membership first
func (s *OrgStore) GetEffectiveTierForPubkey(ctx context.Context, pubkey string, personalTier string) (string, string) {
	// Check if user belongs to an org with tier inheritance
	memberships, err := s.GetMembership(ctx, pubkey)
	if err != nil || len(memberships) == 0 {
		return personalTier, ""
	}

	// Check each membership for tier inheritance
	// Use the highest tier org if multiple memberships exist
	for _, membership := range memberships {
		if !membership.InheritsTier {
			continue
		}

		org, err := s.GetOrganization(ctx, membership.OrgID)
		if err != nil || org == nil {
			continue
		}

		// Return org tier if it's enterprise (highest)
		if org.Tier == "enterprise" {
			return org.Tier, org.ID
		}

		// For other tiers, could implement tier comparison logic
		// For now, enterprise is the only B2B tier
	}

	return personalTier, ""
}

// IsOrgAdmin checks if a pubkey is an admin of an organization
func (s *OrgStore) IsOrgAdmin(ctx context.Context, orgID, pubkey string) (bool, error) {
	// Check if owner
	org, err := s.GetOrganization(ctx, orgID)
	if err != nil {
		return false, err
	}
	if org == nil {
		return false, nil
	}
	if org.OwnerPubkey == pubkey {
		return true, nil
	}

	// Check if admin member
	member, err := s.GetMember(ctx, orgID, pubkey)
	if err != nil {
		return false, err
	}
	if member == nil {
		return false, nil
	}
	return member.Role == OrgRoleAdmin, nil
}

// IsSameOrg checks if two pubkeys belong to the same organization
func (s *OrgStore) IsSameOrg(ctx context.Context, pubkey1, pubkey2 string) (bool, string, error) {
	memberships1, err := s.GetMembership(ctx, pubkey1)
	if err != nil {
		return false, "", err
	}

	memberships2, err := s.GetMembership(ctx, pubkey2)
	if err != nil {
		return false, "", err
	}

	// Check for common org
	for _, m1 := range memberships1 {
		for _, m2 := range memberships2 {
			if m1.OrgID == m2.OrgID {
				return true, m1.OrgID, nil
			}
		}
	}

	return false, "", nil
}

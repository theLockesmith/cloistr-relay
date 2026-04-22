package membership

import (
	"context"
	"database/sql"
	"time"

	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/haven"
)

// Store handles membership persistence
type Store struct {
	db *sql.DB
}

// NewStore creates a new membership store
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// InitSchema creates the membership tables
func (s *Store) InitSchema(ctx context.Context) error {
	schema := `
		CREATE TABLE IF NOT EXISTS nip43_members (
			pubkey TEXT PRIMARY KEY,
			joined_at TIMESTAMP NOT NULL DEFAULT NOW(),
			invite_code TEXT,
			added_by TEXT,
			tier TEXT NOT NULL DEFAULT 'free',
			tier_expires_at TIMESTAMPTZ,
			lightning_address TEXT
		);

		CREATE TABLE IF NOT EXISTS nip43_invites (
			code TEXT PRIMARY KEY,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			expires_at TIMESTAMP,
			max_uses INTEGER NOT NULL DEFAULT 1,
			uses INTEGER NOT NULL DEFAULT 0,
			created_by TEXT NOT NULL
		);

		CREATE INDEX IF NOT EXISTS nip43_invites_expires_idx
			ON nip43_invites(expires_at) WHERE expires_at IS NOT NULL;
		CREATE INDEX IF NOT EXISTS nip43_members_tier_idx
			ON nip43_members(tier);
	`

	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// MigrateAddTierColumns adds tier columns to existing tables
// Safe to run multiple times (uses IF NOT EXISTS pattern via ADD COLUMN IF NOT EXISTS)
func (s *Store) MigrateAddTierColumns(ctx context.Context) error {
	// PostgreSQL 9.6+ supports ADD COLUMN IF NOT EXISTS
	migrations := []string{
		`ALTER TABLE nip43_members ADD COLUMN IF NOT EXISTS tier TEXT NOT NULL DEFAULT 'free'`,
		`ALTER TABLE nip43_members ADD COLUMN IF NOT EXISTS tier_expires_at TIMESTAMPTZ`,
		`ALTER TABLE nip43_members ADD COLUMN IF NOT EXISTS lightning_address TEXT`,
		`CREATE INDEX IF NOT EXISTS nip43_members_tier_idx ON nip43_members(tier)`,
	}

	for _, migration := range migrations {
		if _, err := s.db.ExecContext(ctx, migration); err != nil {
			return err
		}
	}
	return nil
}

// AddMember adds a member to the relay
func (s *Store) AddMember(ctx context.Context, member Member) error {
	tier := member.Tier
	if tier == "" {
		tier = TierFree
	}

	var tierExpiresAt interface{} = nil
	if !member.TierExpiresAt.IsZero() {
		tierExpiresAt = member.TierExpiresAt
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO nip43_members (pubkey, joined_at, invite_code, added_by, tier, tier_expires_at, lightning_address)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (pubkey) DO UPDATE SET
			joined_at = EXCLUDED.joined_at,
			invite_code = EXCLUDED.invite_code,
			added_by = EXCLUDED.added_by,
			tier = EXCLUDED.tier,
			tier_expires_at = EXCLUDED.tier_expires_at,
			lightning_address = EXCLUDED.lightning_address
	`, member.Pubkey, member.JoinedAt, member.InviteCode, member.AddedBy, tier, tierExpiresAt, member.LightningAddress)
	return err
}

// RemoveMember removes a member from the relay
func (s *Store) RemoveMember(ctx context.Context, pubkey string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM nip43_members WHERE pubkey = $1
	`, pubkey)
	return err
}

// IsMember checks if a pubkey is a member
func (s *Store) IsMember(ctx context.Context, pubkey string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM nip43_members WHERE pubkey = $1
	`, pubkey).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetMember retrieves a member by pubkey
func (s *Store) GetMember(ctx context.Context, pubkey string) (*Member, error) {
	var member Member
	var inviteCode, addedBy, tier, lightningAddress sql.NullString
	var tierExpiresAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT pubkey, joined_at, invite_code, added_by, tier, tier_expires_at, lightning_address
		FROM nip43_members WHERE pubkey = $1
	`, pubkey).Scan(&member.Pubkey, &member.JoinedAt, &inviteCode, &addedBy, &tier, &tierExpiresAt, &lightningAddress)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	member.InviteCode = inviteCode.String
	member.AddedBy = addedBy.String
	member.Tier = MemberTier(tier.String)
	if member.Tier == "" {
		member.Tier = TierFree
	}
	if tierExpiresAt.Valid {
		member.TierExpiresAt = tierExpiresAt.Time
	}
	member.LightningAddress = lightningAddress.String

	return &member, nil
}

// ListMembers returns all members
func (s *Store) ListMembers(ctx context.Context) ([]Member, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT pubkey, joined_at, invite_code, added_by, tier, tier_expires_at, lightning_address
		FROM nip43_members ORDER BY joined_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanMembers(rows)
}

// ListMembersByTier returns members with a specific tier
func (s *Store) ListMembersByTier(ctx context.Context, tier MemberTier) ([]Member, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT pubkey, joined_at, invite_code, added_by, tier, tier_expires_at, lightning_address
		FROM nip43_members WHERE tier = $1 ORDER BY joined_at DESC
	`, tier)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanMembers(rows)
}

// CountMembersByTier returns member counts grouped by tier
func (s *Store) CountMembersByTier(ctx context.Context) (map[MemberTier]int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT tier, COUNT(*) FROM nip43_members GROUP BY tier
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[MemberTier]int)
	for rows.Next() {
		var tier string
		var count int
		if err := rows.Scan(&tier, &count); err != nil {
			return nil, err
		}
		counts[MemberTier(tier)] = count
	}
	return counts, rows.Err()
}

// scanMembers is a helper to scan member rows
func (s *Store) scanMembers(rows *sql.Rows) ([]Member, error) {
	var members []Member
	for rows.Next() {
		var member Member
		var inviteCode, addedBy, tier, lightningAddress sql.NullString
		var tierExpiresAt sql.NullTime

		if err := rows.Scan(&member.Pubkey, &member.JoinedAt, &inviteCode, &addedBy, &tier, &tierExpiresAt, &lightningAddress); err != nil {
			return nil, err
		}

		member.InviteCode = inviteCode.String
		member.AddedBy = addedBy.String
		member.Tier = MemberTier(tier.String)
		if member.Tier == "" {
			member.Tier = TierFree
		}
		if tierExpiresAt.Valid {
			member.TierExpiresAt = tierExpiresAt.Time
		}
		member.LightningAddress = lightningAddress.String
		members = append(members, member)
	}

	return members, rows.Err()
}

// CountMembers returns the number of members
func (s *Store) CountMembers(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM nip43_members
	`).Scan(&count)
	return count, err
}

// CreateInvite creates a new invite code
func (s *Store) CreateInvite(ctx context.Context, invite Invite) error {
	var expiresAt interface{} = nil
	if !invite.ExpiresAt.IsZero() {
		expiresAt = invite.ExpiresAt
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO nip43_invites (code, created_at, expires_at, max_uses, uses, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, invite.Code, invite.CreatedAt, expiresAt, invite.MaxUses, invite.Uses, invite.CreatedBy)
	return err
}

// GetInvite retrieves an invite by code
func (s *Store) GetInvite(ctx context.Context, code string) (*Invite, error) {
	var invite Invite
	var expiresAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT code, created_at, expires_at, max_uses, uses, created_by
		FROM nip43_invites WHERE code = $1
	`, code).Scan(&invite.Code, &invite.CreatedAt, &expiresAt, &invite.MaxUses, &invite.Uses, &invite.CreatedBy)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if expiresAt.Valid {
		invite.ExpiresAt = expiresAt.Time
	}

	return &invite, nil
}

// UseInvite increments the use count of an invite
func (s *Store) UseInvite(ctx context.Context, code string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE nip43_invites SET uses = uses + 1 WHERE code = $1
	`, code)
	return err
}

// DeleteInvite removes an invite
func (s *Store) DeleteInvite(ctx context.Context, code string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM nip43_invites WHERE code = $1
	`, code)
	return err
}

// ListInvites returns all invites
func (s *Store) ListInvites(ctx context.Context) ([]Invite, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT code, created_at, expires_at, max_uses, uses, created_by
		FROM nip43_invites ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invites []Invite
	for rows.Next() {
		var invite Invite
		var expiresAt sql.NullTime

		if err := rows.Scan(&invite.Code, &invite.CreatedAt, &expiresAt, &invite.MaxUses, &invite.Uses, &invite.CreatedBy); err != nil {
			return nil, err
		}

		if expiresAt.Valid {
			invite.ExpiresAt = expiresAt.Time
		}
		invites = append(invites, invite)
	}

	return invites, rows.Err()
}

// CleanupExpiredInvites removes expired invites
func (s *Store) CleanupExpiredInvites(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM nip43_invites
		WHERE expires_at IS NOT NULL AND expires_at < $1
	`, time.Now())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// CleanupUsedInvites removes fully-used invites
func (s *Store) CleanupUsedInvites(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM nip43_invites
		WHERE max_uses > 0 AND uses >= max_uses
	`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// UpdateTier updates a member's tier and expiration
func (s *Store) UpdateTier(ctx context.Context, pubkey string, tier MemberTier, expiresAt time.Time) error {
	var tierExpiresAt interface{} = nil
	if !expiresAt.IsZero() {
		tierExpiresAt = expiresAt
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE nip43_members SET tier = $1, tier_expires_at = $2 WHERE pubkey = $3
	`, tier, tierExpiresAt, pubkey)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SetLightningAddress updates a member's Lightning address
func (s *Store) SetLightningAddress(ctx context.Context, pubkey, address string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE nip43_members SET lightning_address = $1 WHERE pubkey = $2
	`, address, pubkey)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ListExpiredTiers returns members whose paid tier has expired
func (s *Store) ListExpiredTiers(ctx context.Context) ([]Member, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT pubkey, joined_at, invite_code, added_by, tier, tier_expires_at, lightning_address
		FROM nip43_members
		WHERE tier != 'free' AND tier_expires_at IS NOT NULL AND tier_expires_at < $1
		ORDER BY tier_expires_at ASC
	`, time.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanMembers(rows)
}

// ResetExpiredTiers downgrades expired tiers to free
func (s *Store) ResetExpiredTiers(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE nip43_members SET tier = 'free'
		WHERE tier != 'free' AND tier_expires_at IS NOT NULL AND tier_expires_at < $1
	`, time.Now())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// GetMemberInfo implements haven.MemberStore interface
// Returns tier information for HAVEN routing decisions
func (s *Store) GetMemberInfo(ctx context.Context, pubkey string) (*haven.MemberInfo, error) {
	member, err := s.GetMember(ctx, pubkey)
	if err != nil {
		return nil, err
	}
	if member == nil {
		return nil, nil
	}

	tier := member.GetEffectiveTier()
	limits := tier.GetLimits()

	return &haven.MemberInfo{
		Pubkey:            member.Pubkey,
		Tier:              string(tier),
		HasHavenBoxes:     limits.HasHavenBoxes,
		HasBlastr:         limits.HasBlastr,
		HasImporter:       limits.HasImporter,
		HasWoTControl:     limits.HasWoTControl,
		MaxBlastrRelays:   limits.MaxBlastrRelays,
		MaxImporterRelays: limits.MaxImporterRelays,
	}, nil
}

// Ensure Store implements haven.MemberStore
var _ haven.MemberStore = (*Store)(nil)

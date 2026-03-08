package membership

import (
	"context"
	"database/sql"
	"time"
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
			added_by TEXT
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
	`

	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// AddMember adds a member to the relay
func (s *Store) AddMember(ctx context.Context, member Member) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO nip43_members (pubkey, joined_at, invite_code, added_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (pubkey) DO UPDATE SET
			joined_at = EXCLUDED.joined_at,
			invite_code = EXCLUDED.invite_code,
			added_by = EXCLUDED.added_by
	`, member.Pubkey, member.JoinedAt, member.InviteCode, member.AddedBy)
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
	var inviteCode, addedBy sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT pubkey, joined_at, invite_code, added_by
		FROM nip43_members WHERE pubkey = $1
	`, pubkey).Scan(&member.Pubkey, &member.JoinedAt, &inviteCode, &addedBy)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	member.InviteCode = inviteCode.String
	member.AddedBy = addedBy.String

	return &member, nil
}

// ListMembers returns all members
func (s *Store) ListMembers(ctx context.Context) ([]Member, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT pubkey, joined_at, invite_code, added_by
		FROM nip43_members ORDER BY joined_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []Member
	for rows.Next() {
		var member Member
		var inviteCode, addedBy sql.NullString

		if err := rows.Scan(&member.Pubkey, &member.JoinedAt, &inviteCode, &addedBy); err != nil {
			return nil, err
		}

		member.InviteCode = inviteCode.String
		member.AddedBy = addedBy.String
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

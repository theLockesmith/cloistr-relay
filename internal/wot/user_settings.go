package wot

import (
	"context"
	"database/sql"
	"time"

	"github.com/lib/pq"
)

// UserSettings represents per-user WoT preferences
// Users can customize their trust settings within the relay's floor
type UserSettings struct {
	// Pubkey is the user's hex pubkey
	Pubkey string

	// TrustAnchor is the pubkey to use as trust root (typically themselves)
	TrustAnchor string

	// MaxTrustDepth overrides relay default (nil = use relay default)
	MaxTrustDepth *int

	// MinPowBits overrides relay default for unknown senders (nil = use relay default)
	MinPowBits *int

	// BlockedPubkeys are pubkeys that never reach this user's inbox
	BlockedPubkeys []string

	// TrustedPubkeys always reach this user's inbox (within relay floor)
	TrustedPubkeys []string

	// Metadata
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsBlocked returns true if the given pubkey is on this user's blocklist
func (s *UserSettings) IsBlocked(pubkey string) bool {
	for _, blocked := range s.BlockedPubkeys {
		if blocked == pubkey {
			return true
		}
	}
	return false
}

// IsTrusted returns true if the given pubkey is on this user's trusted list
func (s *UserSettings) IsTrusted(pubkey string) bool {
	for _, trusted := range s.TrustedPubkeys {
		if trusted == pubkey {
			return true
		}
	}
	return false
}

// GetEffectiveMaxTrustDepth returns the user's max trust depth or the default
func (s *UserSettings) GetEffectiveMaxTrustDepth(relayDefault int) int {
	if s.MaxTrustDepth != nil {
		return *s.MaxTrustDepth
	}
	return relayDefault
}

// GetEffectiveMinPowBits returns the user's min PoW bits or the default
func (s *UserSettings) GetEffectiveMinPowBits(relayDefault int) int {
	if s.MinPowBits != nil {
		return *s.MinPowBits
	}
	return relayDefault
}

// UserSettingsStore handles persistence of user WoT settings
type UserSettingsStore struct {
	db *sql.DB
}

// NewUserSettingsStore creates a new user settings store
func NewUserSettingsStore(db *sql.DB) *UserSettingsStore {
	return &UserSettingsStore{db: db}
}

// Init creates the user settings table
func (s *UserSettingsStore) Init(ctx context.Context) error {
	schema := `
		CREATE TABLE IF NOT EXISTS wot_user_settings (
			pubkey TEXT PRIMARY KEY,
			trust_anchor TEXT,
			max_trust_depth INT,
			min_pow_bits INT,
			blocked_pubkeys TEXT[] DEFAULT '{}',
			trusted_pubkeys TEXT[] DEFAULT '{}',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_wot_user_blocked ON wot_user_settings USING GIN(blocked_pubkeys);
		CREATE INDEX IF NOT EXISTS idx_wot_user_trusted ON wot_user_settings USING GIN(trusted_pubkeys);
	`
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// GetSettings retrieves user settings by pubkey
func (s *UserSettingsStore) GetSettings(ctx context.Context, pubkey string) (*UserSettings, error) {
	var settings UserSettings
	var trustAnchor sql.NullString
	var maxTrustDepth, minPowBits sql.NullInt32

	err := s.db.QueryRowContext(ctx, `
		SELECT pubkey, trust_anchor, max_trust_depth, min_pow_bits,
		       blocked_pubkeys, trusted_pubkeys, created_at, updated_at
		FROM wot_user_settings WHERE pubkey = $1
	`, pubkey).Scan(
		&settings.Pubkey,
		&trustAnchor,
		&maxTrustDepth,
		&minPowBits,
		pq.Array(&settings.BlockedPubkeys),
		pq.Array(&settings.TrustedPubkeys),
		&settings.CreatedAt,
		&settings.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	settings.TrustAnchor = trustAnchor.String
	if maxTrustDepth.Valid {
		depth := int(maxTrustDepth.Int32)
		settings.MaxTrustDepth = &depth
	}
	if minPowBits.Valid {
		bits := int(minPowBits.Int32)
		settings.MinPowBits = &bits
	}

	return &settings, nil
}

// SaveSettings creates or updates user settings
func (s *UserSettingsStore) SaveSettings(ctx context.Context, settings *UserSettings) error {
	var maxTrustDepth, minPowBits interface{}
	if settings.MaxTrustDepth != nil {
		maxTrustDepth = *settings.MaxTrustDepth
	}
	if settings.MinPowBits != nil {
		minPowBits = *settings.MinPowBits
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO wot_user_settings
			(pubkey, trust_anchor, max_trust_depth, min_pow_bits, blocked_pubkeys, trusted_pubkeys, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (pubkey) DO UPDATE SET
			trust_anchor = EXCLUDED.trust_anchor,
			max_trust_depth = EXCLUDED.max_trust_depth,
			min_pow_bits = EXCLUDED.min_pow_bits,
			blocked_pubkeys = EXCLUDED.blocked_pubkeys,
			trusted_pubkeys = EXCLUDED.trusted_pubkeys,
			updated_at = NOW()
	`,
		settings.Pubkey,
		settings.TrustAnchor,
		maxTrustDepth,
		minPowBits,
		pq.Array(settings.BlockedPubkeys),
		pq.Array(settings.TrustedPubkeys),
	)
	return err
}

// DeleteSettings removes user settings
func (s *UserSettingsStore) DeleteSettings(ctx context.Context, pubkey string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM wot_user_settings WHERE pubkey = $1`, pubkey)
	return err
}

// AddBlocked adds a pubkey to the user's blocklist
func (s *UserSettingsStore) AddBlocked(ctx context.Context, userPubkey, blockedPubkey string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO wot_user_settings (pubkey, blocked_pubkeys, updated_at)
		VALUES ($1, ARRAY[$2], NOW())
		ON CONFLICT (pubkey) DO UPDATE SET
			blocked_pubkeys = array_append(
				array_remove(wot_user_settings.blocked_pubkeys, $2), $2
			),
			updated_at = NOW()
	`, userPubkey, blockedPubkey)
	return err
}

// RemoveBlocked removes a pubkey from the user's blocklist
func (s *UserSettingsStore) RemoveBlocked(ctx context.Context, userPubkey, blockedPubkey string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE wot_user_settings
		SET blocked_pubkeys = array_remove(blocked_pubkeys, $2), updated_at = NOW()
		WHERE pubkey = $1
	`, userPubkey, blockedPubkey)
	return err
}

// AddTrusted adds a pubkey to the user's trusted list
func (s *UserSettingsStore) AddTrusted(ctx context.Context, userPubkey, trustedPubkey string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO wot_user_settings (pubkey, trusted_pubkeys, updated_at)
		VALUES ($1, ARRAY[$2], NOW())
		ON CONFLICT (pubkey) DO UPDATE SET
			trusted_pubkeys = array_append(
				array_remove(wot_user_settings.trusted_pubkeys, $2), $2
			),
			updated_at = NOW()
	`, userPubkey, trustedPubkey)
	return err
}

// RemoveTrusted removes a pubkey from the user's trusted list
func (s *UserSettingsStore) RemoveTrusted(ctx context.Context, userPubkey, trustedPubkey string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE wot_user_settings
		SET trusted_pubkeys = array_remove(trusted_pubkeys, $2), updated_at = NOW()
		WHERE pubkey = $1
	`, userPubkey, trustedPubkey)
	return err
}

// IsBlockedBy checks if senderPubkey is blocked by userPubkey
func (s *UserSettingsStore) IsBlockedBy(ctx context.Context, userPubkey, senderPubkey string) (bool, error) {
	var blocked bool
	err := s.db.QueryRowContext(ctx, `
		SELECT $2 = ANY(blocked_pubkeys) FROM wot_user_settings WHERE pubkey = $1
	`, userPubkey, senderPubkey).Scan(&blocked)

	if err == sql.ErrNoRows {
		return false, nil
	}
	return blocked, err
}

// IsTrustedBy checks if senderPubkey is trusted by userPubkey
func (s *UserSettingsStore) IsTrustedBy(ctx context.Context, userPubkey, senderPubkey string) (bool, error) {
	var trusted bool
	err := s.db.QueryRowContext(ctx, `
		SELECT $2 = ANY(trusted_pubkeys) FROM wot_user_settings WHERE pubkey = $1
	`, userPubkey, senderPubkey).Scan(&trusted)

	if err == sql.ErrNoRows {
		return false, nil
	}
	return trusted, err
}

// ListUsersWithSettings returns all users who have custom WoT settings
func (s *UserSettingsStore) ListUsersWithSettings(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT pubkey FROM wot_user_settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pubkeys []string
	for rows.Next() {
		var pk string
		if err := rows.Scan(&pk); err != nil {
			return nil, err
		}
		pubkeys = append(pubkeys, pk)
	}
	return pubkeys, rows.Err()
}

// CountUsersWithBlocklist returns count of users with non-empty blocklists
func (s *UserSettingsStore) CountUsersWithBlocklist(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM wot_user_settings WHERE array_length(blocked_pubkeys, 1) > 0
	`).Scan(&count)
	return count, err
}

// CountUsersWithTrustedlist returns count of users with non-empty trusted lists
func (s *UserSettingsStore) CountUsersWithTrustedlist(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM wot_user_settings WHERE array_length(trusted_pubkeys, 1) > 0
	`).Scan(&count)
	return count, err
}

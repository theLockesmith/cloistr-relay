package haven

import (
	"context"
	"database/sql"
	"time"

	"github.com/lib/pq"
)

// UserSettings represents per-user HAVEN settings
type UserSettings struct {
	// Pubkey is the user's hex pubkey
	Pubkey string

	// Blastr settings (tier-gated)
	BlastrEnabled bool
	BlastrRelays  []string

	// Importer settings (tier-gated)
	ImporterEnabled  bool
	ImporterRelays   []string
	ImporterRealtime bool
	LastImportTime   *time.Time

	// Privacy settings
	PublicOutboxRead     bool // Allow others to read outbox (default true)
	PublicInboxWrite     bool // Allow others to write to inbox (default true)
	RequireAuthChat      bool // Require auth for chat box (default true)
	RequireAuthPrivate   bool // Require auth for private box (default true)

	// Metadata
	CreatedAt time.Time
	UpdatedAt time.Time
}

// UserSettingsStore handles persistence of per-user HAVEN settings
type UserSettingsStore struct {
	db *sql.DB
}

// NewUserSettingsStore creates a new haven user settings store
func NewUserSettingsStore(db *sql.DB) *UserSettingsStore {
	return &UserSettingsStore{db: db}
}

// Init creates the haven user settings table
func (s *UserSettingsStore) Init(ctx context.Context) error {
	schema := `
		CREATE TABLE IF NOT EXISTS haven_user_settings (
			pubkey TEXT PRIMARY KEY,

			-- Blastr settings
			blastr_enabled BOOLEAN DEFAULT false,
			blastr_relays TEXT[] DEFAULT '{}',

			-- Importer settings
			importer_enabled BOOLEAN DEFAULT false,
			importer_relays TEXT[] DEFAULT '{}',
			importer_realtime BOOLEAN DEFAULT false,
			last_import_time TIMESTAMPTZ,

			-- Privacy settings
			public_outbox_read BOOLEAN DEFAULT true,
			public_inbox_write BOOLEAN DEFAULT true,
			require_auth_chat BOOLEAN DEFAULT true,
			require_auth_private BOOLEAN DEFAULT true,

			-- Metadata
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_haven_blastr_enabled ON haven_user_settings(blastr_enabled) WHERE blastr_enabled = true;
		CREATE INDEX IF NOT EXISTS idx_haven_importer_enabled ON haven_user_settings(importer_enabled) WHERE importer_enabled = true;
	`
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// GetSettings retrieves haven settings for a user
func (s *UserSettingsStore) GetSettings(ctx context.Context, pubkey string) (*UserSettings, error) {
	var settings UserSettings
	var lastImportTime sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT pubkey, blastr_enabled, blastr_relays, importer_enabled, importer_relays,
		       importer_realtime, last_import_time, public_outbox_read, public_inbox_write,
		       require_auth_chat, require_auth_private, created_at, updated_at
		FROM haven_user_settings WHERE pubkey = $1
	`, pubkey).Scan(
		&settings.Pubkey,
		&settings.BlastrEnabled,
		pq.Array(&settings.BlastrRelays),
		&settings.ImporterEnabled,
		pq.Array(&settings.ImporterRelays),
		&settings.ImporterRealtime,
		&lastImportTime,
		&settings.PublicOutboxRead,
		&settings.PublicInboxWrite,
		&settings.RequireAuthChat,
		&settings.RequireAuthPrivate,
		&settings.CreatedAt,
		&settings.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if lastImportTime.Valid {
		settings.LastImportTime = &lastImportTime.Time
	}

	return &settings, nil
}

// SaveSettings creates or updates haven settings
func (s *UserSettingsStore) SaveSettings(ctx context.Context, settings *UserSettings) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO haven_user_settings
			(pubkey, blastr_enabled, blastr_relays, importer_enabled, importer_relays,
			 importer_realtime, last_import_time, public_outbox_read, public_inbox_write,
			 require_auth_chat, require_auth_private, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
		ON CONFLICT (pubkey) DO UPDATE SET
			blastr_enabled = EXCLUDED.blastr_enabled,
			blastr_relays = EXCLUDED.blastr_relays,
			importer_enabled = EXCLUDED.importer_enabled,
			importer_relays = EXCLUDED.importer_relays,
			importer_realtime = EXCLUDED.importer_realtime,
			last_import_time = EXCLUDED.last_import_time,
			public_outbox_read = EXCLUDED.public_outbox_read,
			public_inbox_write = EXCLUDED.public_inbox_write,
			require_auth_chat = EXCLUDED.require_auth_chat,
			require_auth_private = EXCLUDED.require_auth_private,
			updated_at = NOW()
	`,
		settings.Pubkey,
		settings.BlastrEnabled,
		pq.Array(settings.BlastrRelays),
		settings.ImporterEnabled,
		pq.Array(settings.ImporterRelays),
		settings.ImporterRealtime,
		settings.LastImportTime,
		settings.PublicOutboxRead,
		settings.PublicInboxWrite,
		settings.RequireAuthChat,
		settings.RequireAuthPrivate,
	)
	return err
}

// DeleteSettings removes haven settings for a user
func (s *UserSettingsStore) DeleteSettings(ctx context.Context, pubkey string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM haven_user_settings WHERE pubkey = $1`, pubkey)
	return err
}

// SetBlastrEnabled enables/disables blastr for a user
func (s *UserSettingsStore) SetBlastrEnabled(ctx context.Context, pubkey string, enabled bool) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO haven_user_settings (pubkey, blastr_enabled, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (pubkey) DO UPDATE SET
			blastr_enabled = EXCLUDED.blastr_enabled,
			updated_at = NOW()
	`, pubkey, enabled)
	return err
}

// SetBlastrRelays sets the blastr relay list for a user
func (s *UserSettingsStore) SetBlastrRelays(ctx context.Context, pubkey string, relays []string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO haven_user_settings (pubkey, blastr_relays, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (pubkey) DO UPDATE SET
			blastr_relays = EXCLUDED.blastr_relays,
			updated_at = NOW()
	`, pubkey, pq.Array(relays))
	return err
}

// SetImporterEnabled enables/disables importer for a user
func (s *UserSettingsStore) SetImporterEnabled(ctx context.Context, pubkey string, enabled bool) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO haven_user_settings (pubkey, importer_enabled, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (pubkey) DO UPDATE SET
			importer_enabled = EXCLUDED.importer_enabled,
			updated_at = NOW()
	`, pubkey, enabled)
	return err
}

// SetImporterRelays sets the importer relay list for a user
func (s *UserSettingsStore) SetImporterRelays(ctx context.Context, pubkey string, relays []string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO haven_user_settings (pubkey, importer_relays, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (pubkey) DO UPDATE SET
			importer_relays = EXCLUDED.importer_relays,
			updated_at = NOW()
	`, pubkey, pq.Array(relays))
	return err
}

// UpdateLastImportTime updates the last import time for a user
func (s *UserSettingsStore) UpdateLastImportTime(ctx context.Context, pubkey string, t time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE haven_user_settings
		SET last_import_time = $2, updated_at = NOW()
		WHERE pubkey = $1
	`, pubkey, t)
	return err
}

// ListUsersWithBlastr returns pubkeys of users with blastr enabled
func (s *UserSettingsStore) ListUsersWithBlastr(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT pubkey FROM haven_user_settings WHERE blastr_enabled = true
	`)
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

// ListUsersWithImporter returns pubkeys of users with importer enabled
func (s *UserSettingsStore) ListUsersWithImporter(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT pubkey FROM haven_user_settings WHERE importer_enabled = true
	`)
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

// GetUsersForImport returns users with importer enabled along with their settings
func (s *UserSettingsStore) GetUsersForImport(ctx context.Context) ([]*UserSettings, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT pubkey, blastr_enabled, blastr_relays, importer_enabled, importer_relays,
		       importer_realtime, last_import_time, public_outbox_read, public_inbox_write,
		       require_auth_chat, require_auth_private, created_at, updated_at
		FROM haven_user_settings WHERE importer_enabled = true
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*UserSettings
	for rows.Next() {
		var settings UserSettings
		var lastImportTime sql.NullTime
		if err := rows.Scan(
			&settings.Pubkey,
			&settings.BlastrEnabled,
			pq.Array(&settings.BlastrRelays),
			&settings.ImporterEnabled,
			pq.Array(&settings.ImporterRelays),
			&settings.ImporterRealtime,
			&lastImportTime,
			&settings.PublicOutboxRead,
			&settings.PublicInboxWrite,
			&settings.RequireAuthChat,
			&settings.RequireAuthPrivate,
			&settings.CreatedAt,
			&settings.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if lastImportTime.Valid {
			settings.LastImportTime = &lastImportTime.Time
		}
		users = append(users, &settings)
	}
	return users, rows.Err()
}

// CountUsersWithBlastr returns count of users with blastr enabled
func (s *UserSettingsStore) CountUsersWithBlastr(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM haven_user_settings WHERE blastr_enabled = true
	`).Scan(&count)
	return count, err
}

// CountUsersWithImporter returns count of users with importer enabled
func (s *UserSettingsStore) CountUsersWithImporter(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM haven_user_settings WHERE importer_enabled = true
	`).Scan(&count)
	return count, err
}

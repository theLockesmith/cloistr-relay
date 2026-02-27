package management

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// Store handles database operations for relay management
type Store struct {
	db *sql.DB
}

// NewStore creates a new management store with the given database connection
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Init creates the management tables if they don't exist
func (s *Store) Init() error {
	schema := `
		-- Banned pubkeys
		CREATE TABLE IF NOT EXISTS management_banned_pubkeys (
			pubkey TEXT PRIMARY KEY,
			reason TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		-- Allowed pubkeys (whitelist mode)
		CREATE TABLE IF NOT EXISTS management_allowed_pubkeys (
			pubkey TEXT PRIMARY KEY,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		-- Banned events by ID
		CREATE TABLE IF NOT EXISTS management_banned_events (
			event_id TEXT PRIMARY KEY,
			reason TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		-- Content moderation queue
		CREATE TABLE IF NOT EXISTS management_moderation_queue (
			id SERIAL PRIMARY KEY,
			event_id TEXT NOT NULL UNIQUE,
			event_json JSONB NOT NULL,
			reported_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			status TEXT DEFAULT 'pending'
		);

		-- Allowed kinds (kind restriction mode)
		CREATE TABLE IF NOT EXISTS management_allowed_kinds (
			kind INTEGER PRIMARY KEY
		);

		-- Blocked IPs
		CREATE TABLE IF NOT EXISTS management_blocked_ips (
			ip TEXT PRIMARY KEY,
			reason TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		-- Relay settings (key-value)
		CREATE TABLE IF NOT EXISTS management_relay_settings (
			key TEXT PRIMARY KEY,
			value TEXT
		);

		-- Create indexes for faster lookups
		CREATE INDEX IF NOT EXISTS idx_moderation_status ON management_moderation_queue(status);
	`

	_, err := s.db.Exec(schema)
	return err
}

// BanPubkey adds a pubkey to the banned list
func (s *Store) BanPubkey(pubkey, reason string) error {
	_, err := s.db.Exec(
		`INSERT INTO management_banned_pubkeys (pubkey, reason)
		 VALUES ($1, $2)
		 ON CONFLICT (pubkey) DO UPDATE SET reason = $2`,
		pubkey, reason,
	)
	return err
}

// UnbanPubkey removes a pubkey from the banned list
func (s *Store) UnbanPubkey(pubkey string) error {
	_, err := s.db.Exec(`DELETE FROM management_banned_pubkeys WHERE pubkey = $1`, pubkey)
	return err
}

// IsPubkeyBanned checks if a pubkey is banned
func (s *Store) IsPubkeyBanned(pubkey string) bool {
	var exists bool
	err := s.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM management_banned_pubkeys WHERE pubkey = $1)`,
		pubkey,
	).Scan(&exists)
	return err == nil && exists
}

// ListBannedPubkeys returns all banned pubkeys
func (s *Store) ListBannedPubkeys(limit, offset int) ([]BannedPubkey, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(
		`SELECT pubkey, COALESCE(reason, ''), created_at
		 FROM management_banned_pubkeys
		 ORDER BY created_at DESC
		 LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []BannedPubkey
	for rows.Next() {
		var bp BannedPubkey
		if err := rows.Scan(&bp.Pubkey, &bp.Reason, &bp.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, bp)
	}
	return result, rows.Err()
}

// AllowPubkey adds a pubkey to the allowed list (whitelist)
func (s *Store) AllowPubkey(pubkey string) error {
	_, err := s.db.Exec(
		`INSERT INTO management_allowed_pubkeys (pubkey) VALUES ($1) ON CONFLICT DO NOTHING`,
		pubkey,
	)
	return err
}

// RemoveAllowedPubkey removes a pubkey from the allowed list
func (s *Store) RemoveAllowedPubkey(pubkey string) error {
	_, err := s.db.Exec(`DELETE FROM management_allowed_pubkeys WHERE pubkey = $1`, pubkey)
	return err
}

// IsPubkeyAllowed checks if a pubkey is in the allowed list
func (s *Store) IsPubkeyAllowed(pubkey string) bool {
	var exists bool
	err := s.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM management_allowed_pubkeys WHERE pubkey = $1)`,
		pubkey,
	).Scan(&exists)
	return err == nil && exists
}

// ListAllowedPubkeys returns all allowed pubkeys
func (s *Store) ListAllowedPubkeys(limit, offset int) ([]AllowedPubkey, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(
		`SELECT pubkey, created_at
		 FROM management_allowed_pubkeys
		 ORDER BY created_at DESC
		 LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []AllowedPubkey
	for rows.Next() {
		var ap AllowedPubkey
		if err := rows.Scan(&ap.Pubkey, &ap.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, ap)
	}
	return result, rows.Err()
}

// BanEvent adds an event ID to the banned list
func (s *Store) BanEvent(eventID, reason string) error {
	_, err := s.db.Exec(
		`INSERT INTO management_banned_events (event_id, reason)
		 VALUES ($1, $2)
		 ON CONFLICT (event_id) DO UPDATE SET reason = $2`,
		eventID, reason,
	)
	return err
}

// UnbanEvent removes an event ID from the banned list
func (s *Store) UnbanEvent(eventID string) error {
	_, err := s.db.Exec(`DELETE FROM management_banned_events WHERE event_id = $1`, eventID)
	return err
}

// IsEventBanned checks if an event is banned
func (s *Store) IsEventBanned(eventID string) bool {
	var exists bool
	err := s.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM management_banned_events WHERE event_id = $1)`,
		eventID,
	).Scan(&exists)
	return err == nil && exists
}

// ListBannedEvents returns all banned events
func (s *Store) ListBannedEvents(limit, offset int) ([]BannedEvent, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(
		`SELECT event_id, COALESCE(reason, ''), created_at
		 FROM management_banned_events
		 ORDER BY created_at DESC
		 LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []BannedEvent
	for rows.Next() {
		var be BannedEvent
		if err := rows.Scan(&be.EventID, &be.Reason, &be.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, be)
	}
	return result, rows.Err()
}

// AddToModerationQueue adds an event to the moderation queue
func (s *Store) AddToModerationQueue(eventID string, eventJSON json.RawMessage) error {
	_, err := s.db.Exec(
		`INSERT INTO management_moderation_queue (event_id, event_json)
		 VALUES ($1, $2)
		 ON CONFLICT (event_id) DO NOTHING`,
		eventID, eventJSON,
	)
	return err
}

// ListModerationQueue returns pending events in the moderation queue
func (s *Store) ListModerationQueue(limit, offset int) ([]ModerationItem, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(
		`SELECT id, event_id, event_json, reported_at, status
		 FROM management_moderation_queue
		 WHERE status = 'pending'
		 ORDER BY reported_at ASC
		 LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []ModerationItem
	for rows.Next() {
		var item ModerationItem
		if err := rows.Scan(&item.ID, &item.EventID, &item.EventJSON, &item.ReportedAt, &item.Status); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// UpdateModerationStatus updates the status of an event in the moderation queue
func (s *Store) UpdateModerationStatus(eventID, status string) error {
	result, err := s.db.Exec(
		`UPDATE management_moderation_queue SET status = $1 WHERE event_id = $2`,
		status, eventID,
	)
	if err != nil {
		return err
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("event not found in moderation queue")
	}
	return nil
}

// AllowKind adds a kind to the allowed kinds list
func (s *Store) AllowKind(kind int) error {
	_, err := s.db.Exec(
		`INSERT INTO management_allowed_kinds (kind) VALUES ($1) ON CONFLICT DO NOTHING`,
		kind,
	)
	return err
}

// DisallowKind removes a kind from the allowed kinds list
func (s *Store) DisallowKind(kind int) error {
	_, err := s.db.Exec(`DELETE FROM management_allowed_kinds WHERE kind = $1`, kind)
	return err
}

// IsKindAllowed checks if a kind is allowed (returns true if no restriction or kind is in list)
func (s *Store) IsKindAllowed(kind int) bool {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM management_allowed_kinds`).Scan(&count)
	if err != nil || count == 0 {
		return true // No restrictions if table is empty
	}

	var exists bool
	err = s.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM management_allowed_kinds WHERE kind = $1)`,
		kind,
	).Scan(&exists)
	return err == nil && exists
}

// ListAllowedKinds returns all allowed kinds
func (s *Store) ListAllowedKinds() ([]int, error) {
	rows, err := s.db.Query(`SELECT kind FROM management_allowed_kinds ORDER BY kind`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []int
	for rows.Next() {
		var kind int
		if err := rows.Scan(&kind); err != nil {
			return nil, err
		}
		result = append(result, kind)
	}
	return result, rows.Err()
}

// BlockIP adds an IP to the blocked list
func (s *Store) BlockIP(ip, reason string) error {
	_, err := s.db.Exec(
		`INSERT INTO management_blocked_ips (ip, reason)
		 VALUES ($1, $2)
		 ON CONFLICT (ip) DO UPDATE SET reason = $2`,
		ip, reason,
	)
	return err
}

// UnblockIP removes an IP from the blocked list
func (s *Store) UnblockIP(ip string) error {
	_, err := s.db.Exec(`DELETE FROM management_blocked_ips WHERE ip = $1`, ip)
	return err
}

// IsIPBlocked checks if an IP is blocked
func (s *Store) IsIPBlocked(ip string) bool {
	var exists bool
	err := s.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM management_blocked_ips WHERE ip = $1)`,
		ip,
	).Scan(&exists)
	return err == nil && exists
}

// ListBlockedIPs returns all blocked IPs
func (s *Store) ListBlockedIPs(limit, offset int) ([]BlockedIP, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(
		`SELECT ip, COALESCE(reason, ''), created_at
		 FROM management_blocked_ips
		 ORDER BY created_at DESC
		 LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []BlockedIP
	for rows.Next() {
		var bi BlockedIP
		if err := rows.Scan(&bi.IP, &bi.Reason, &bi.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, bi)
	}
	return result, rows.Err()
}

// GetSetting retrieves a relay setting by key
func (s *Store) GetSetting(key string) (string, error) {
	var value string
	err := s.db.QueryRow(
		`SELECT value FROM management_relay_settings WHERE key = $1`,
		key,
	).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// SetSetting stores a relay setting
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO management_relay_settings (key, value)
		 VALUES ($1, $2)
		 ON CONFLICT (key) DO UPDATE SET value = $2`,
		key, value,
	)
	return err
}

package groups

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

var (
	ErrGroupNotFound     = errors.New("group not found")
	ErrGroupExists       = errors.New("group already exists")
	ErrNotMember         = errors.New("not a member of this group")
	ErrNotAdmin          = errors.New("not an admin of this group")
	ErrInvalidInvite     = errors.New("invalid or expired invite code")
	ErrJoinNotAllowed    = errors.New("join requests not allowed for this group")
	ErrPermissionDenied  = errors.New("permission denied")
	ErrGroupLimitReached = errors.New("maximum groups per user reached")
)

// Store manages NIP-29 group data
type Store struct {
	db     *sql.DB
	config *Config
	mu     sync.RWMutex
	cache  map[string]*Group // In-memory cache for fast lookups
}

// NewStore creates a new group store
func NewStore(db *sql.DB, cfg *Config) (*Store, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	store := &Store{
		db:     db,
		config: cfg,
		cache:  make(map[string]*Group),
	}

	// Create tables if they don't exist
	if err := store.initTables(); err != nil {
		return nil, fmt.Errorf("failed to init group tables: %w", err)
	}

	// Load groups into cache
	if err := store.loadCache(); err != nil {
		log.Printf("Warning: failed to load groups cache: %v", err)
	}

	return store, nil
}

// initTables creates the required database tables
func (s *Store) initTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS nip29_groups (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			picture TEXT,
			about TEXT,
			privacy TEXT NOT NULL DEFAULT 'restricted',
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			created_by TEXT NOT NULL,
			relay_url TEXT,
			metadata JSONB
		)`,
		`CREATE TABLE IF NOT EXISTS nip29_members (
			group_id TEXT NOT NULL,
			pubkey TEXT NOT NULL,
			role TEXT,
			joined_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			added_by TEXT,
			PRIMARY KEY (group_id, pubkey),
			FOREIGN KEY (group_id) REFERENCES nip29_groups(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS nip29_invites (
			code TEXT PRIMARY KEY,
			group_id TEXT NOT NULL,
			created_by TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at TIMESTAMP,
			max_uses INTEGER DEFAULT 0,
			uses INTEGER DEFAULT 0,
			FOREIGN KEY (group_id) REFERENCES nip29_groups(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_nip29_members_pubkey ON nip29_members(pubkey)`,
		`CREATE INDEX IF NOT EXISTS idx_nip29_invites_group ON nip29_invites(group_id)`,
	}

	for _, query := range queries {
		if _, err := s.db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute: %s: %w", query[:50], err)
		}
	}

	return nil
}

// loadCache loads all groups into memory
func (s *Store) loadCache() error {
	rows, err := s.db.Query(`
		SELECT id, name, picture, about, privacy, created_at, created_by, relay_url
		FROM nip29_groups
	`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	s.mu.Lock()
	defer s.mu.Unlock()

	for rows.Next() {
		var g Group
		var picture, about, relayURL sql.NullString
		if err := rows.Scan(&g.ID, &g.Name, &picture, &about, &g.Privacy, &g.CreatedAt, &g.CreatedBy, &relayURL); err != nil {
			return err
		}
		g.Picture = picture.String
		g.About = about.String
		g.RelayURL = relayURL.String
		g.Admins = make(map[string]string)
		g.Members = make(map[string]bool)

		// Load members
		memberRows, err := s.db.Query(`
			SELECT pubkey, role FROM nip29_members WHERE group_id = $1
		`, g.ID)
		if err != nil {
			return err
		}
		for memberRows.Next() {
			var pubkey string
			var role sql.NullString
			if err := memberRows.Scan(&pubkey, &role); err != nil {
				_ = memberRows.Close()
				return err
			}
			g.Members[pubkey] = true
			if role.String != "" {
				g.Admins[pubkey] = role.String
			}
		}
		_ = memberRows.Close()

		s.cache[g.ID] = &g
	}

	return nil
}

// generateID creates a random group ID
func generateID() string {
	bytes := make([]byte, 16)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// generateInviteCode creates a random invite code
func generateInviteCode() string {
	bytes := make([]byte, 12)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// CreateGroup creates a new group
func (s *Store) CreateGroup(ctx context.Context, creatorPubkey, name string, privacy Privacy) (*Group, error) {
	// Check if user has reached group limit
	if s.config.MaxGroupsPerUser > 0 {
		count, err := s.CountGroupsByCreator(ctx, creatorPubkey)
		if err != nil {
			return nil, err
		}
		if count >= s.config.MaxGroupsPerUser {
			return nil, ErrGroupLimitReached
		}
	}

	group := &Group{
		ID:        generateID(),
		Name:      name,
		Privacy:   privacy,
		CreatedAt: time.Now(),
		CreatedBy: creatorPubkey,
		RelayURL:  s.config.RelayURL,
		Admins:    map[string]string{creatorPubkey: "admin"},
		Members:   map[string]bool{creatorPubkey: true},
	}

	// Insert group
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO nip29_groups (id, name, privacy, created_at, created_by, relay_url)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, group.ID, group.Name, group.Privacy, group.CreatedAt, group.CreatedBy, group.RelayURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create group: %w", err)
	}

	// Add creator as admin member
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO nip29_members (group_id, pubkey, role, joined_at, added_by)
		VALUES ($1, $2, $3, $4, $5)
	`, group.ID, creatorPubkey, "admin", group.CreatedAt, creatorPubkey)
	if err != nil {
		return nil, fmt.Errorf("failed to add creator as member: %w", err)
	}

	// Update cache
	s.mu.Lock()
	s.cache[group.ID] = group
	s.mu.Unlock()

	log.Printf("NIP-29: created group %s (%s) by %s", group.ID[:8], group.Name, creatorPubkey[:8])
	return group, nil
}

// GetGroup retrieves a group by ID
func (s *Store) GetGroup(ctx context.Context, groupID string) (*Group, error) {
	s.mu.RLock()
	if g, ok := s.cache[groupID]; ok {
		s.mu.RUnlock()
		return g, nil
	}
	s.mu.RUnlock()

	return nil, ErrGroupNotFound
}

// DeleteGroup removes a group
func (s *Store) DeleteGroup(ctx context.Context, groupID string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM nip29_groups WHERE id = $1`, groupID)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrGroupNotFound
	}

	// Update cache
	s.mu.Lock()
	delete(s.cache, groupID)
	s.mu.Unlock()

	log.Printf("NIP-29: deleted group %s", groupID[:8])
	return nil
}

// UpdateGroupMetadata updates group metadata
func (s *Store) UpdateGroupMetadata(ctx context.Context, groupID string, name, picture, about string, privacy Privacy) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE nip29_groups SET name = $1, picture = $2, about = $3, privacy = $4
		WHERE id = $5
	`, name, picture, about, privacy, groupID)
	if err != nil {
		return err
	}

	// Update cache
	s.mu.Lock()
	if g, ok := s.cache[groupID]; ok {
		g.Name = name
		g.Picture = picture
		g.About = about
		g.Privacy = privacy
	}
	s.mu.Unlock()

	return nil
}

// AddMember adds a member to a group
func (s *Store) AddMember(ctx context.Context, groupID, pubkey, role, addedBy string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO nip29_members (group_id, pubkey, role, joined_at, added_by)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (group_id, pubkey) DO UPDATE SET role = $3
	`, groupID, pubkey, role, time.Now(), addedBy)
	if err != nil {
		return err
	}

	// Update cache
	s.mu.Lock()
	if g, ok := s.cache[groupID]; ok {
		g.Members[pubkey] = true
		if role != "" {
			g.Admins[pubkey] = role
		}
	}
	s.mu.Unlock()

	log.Printf("NIP-29: added %s to group %s with role %s", pubkey[:8], groupID[:8], role)
	return nil
}

// RemoveMember removes a member from a group
func (s *Store) RemoveMember(ctx context.Context, groupID, pubkey string) error {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM nip29_members WHERE group_id = $1 AND pubkey = $2
	`, groupID, pubkey)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotMember
	}

	// Update cache
	s.mu.Lock()
	if g, ok := s.cache[groupID]; ok {
		delete(g.Members, pubkey)
		delete(g.Admins, pubkey)
	}
	s.mu.Unlock()

	log.Printf("NIP-29: removed %s from group %s", pubkey[:8], groupID[:8])
	return nil
}

// IsMember checks if a pubkey is a member of a group
func (s *Store) IsMember(ctx context.Context, groupID, pubkey string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if g, ok := s.cache[groupID]; ok {
		return g.Members[pubkey]
	}
	return false
}

// IsAdmin checks if a pubkey is an admin of a group
func (s *Store) IsAdmin(ctx context.Context, groupID, pubkey string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if g, ok := s.cache[groupID]; ok {
		_, isAdmin := g.Admins[pubkey]
		return isAdmin
	}
	return false
}

// GetMemberRole returns the role of a member
func (s *Store) GetMemberRole(ctx context.Context, groupID, pubkey string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if g, ok := s.cache[groupID]; ok {
		return g.Admins[pubkey]
	}
	return ""
}

// CreateInvite creates an invite code for a group
func (s *Store) CreateInvite(ctx context.Context, groupID, creatorPubkey string, maxUses int, expiresAt time.Time) (*InviteCode, error) {
	invite := &InviteCode{
		Code:      generateInviteCode(),
		CreatedBy: creatorPubkey,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
		MaxUses:   maxUses,
		Uses:      0,
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO nip29_invites (code, group_id, created_by, created_at, expires_at, max_uses, uses)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, invite.Code, groupID, invite.CreatedBy, invite.CreatedAt, invite.ExpiresAt, invite.MaxUses, invite.Uses)
	if err != nil {
		return nil, err
	}

	log.Printf("NIP-29: created invite %s for group %s", invite.Code[:8], groupID[:8])
	return invite, nil
}

// UseInvite uses an invite code to join a group
func (s *Store) UseInvite(ctx context.Context, code, pubkey string) (string, error) {
	// Get invite
	var groupID string
	var expiresAt sql.NullTime
	var maxUses, uses int

	err := s.db.QueryRowContext(ctx, `
		SELECT group_id, expires_at, max_uses, uses FROM nip29_invites WHERE code = $1
	`, code).Scan(&groupID, &expiresAt, &maxUses, &uses)
	if err == sql.ErrNoRows {
		return "", ErrInvalidInvite
	}
	if err != nil {
		return "", err
	}

	// Check expiry
	if expiresAt.Valid && time.Now().After(expiresAt.Time) {
		return "", ErrInvalidInvite
	}

	// Check max uses
	if maxUses > 0 && uses >= maxUses {
		return "", ErrInvalidInvite
	}

	// Increment uses
	_, err = s.db.ExecContext(ctx, `UPDATE nip29_invites SET uses = uses + 1 WHERE code = $1`, code)
	if err != nil {
		return "", err
	}

	// Add member
	if err := s.AddMember(ctx, groupID, pubkey, "", "invite:"+code); err != nil {
		return "", err
	}

	return groupID, nil
}

// ListGroups returns all groups (optionally filtered by member)
func (s *Store) ListGroups(ctx context.Context, memberPubkey string) ([]*Group, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var groups []*Group
	for _, g := range s.cache {
		if memberPubkey == "" || g.Members[memberPubkey] {
			groups = append(groups, g)
		}
	}
	return groups, nil
}

// ListMembers returns all members of a group
func (s *Store) ListMembers(ctx context.Context, groupID string) ([]Member, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT pubkey, role, joined_at, added_by FROM nip29_members WHERE group_id = $1
	`, groupID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var members []Member
	for rows.Next() {
		var m Member
		var role, addedBy sql.NullString
		if err := rows.Scan(&m.Pubkey, &role, &m.JoinedAt, &addedBy); err != nil {
			return nil, err
		}
		m.Role = role.String
		m.AddedBy = addedBy.String
		members = append(members, m)
	}

	return members, nil
}

// CountGroupsByCreator returns the number of groups created by a pubkey
func (s *Store) CountGroupsByCreator(ctx context.Context, pubkey string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM nip29_groups WHERE created_by = $1
	`, pubkey).Scan(&count)
	return count, err
}

// GetGroupMetadataJSON returns group metadata as JSON for kind 39000
func (s *Store) GetGroupMetadataJSON(ctx context.Context, groupID string) ([]byte, error) {
	group, err := s.GetGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}

	meta := GroupMetadata{
		ID:      group.ID,
		Name:    group.Name,
		Picture: group.Picture,
		About:   group.About,
		Privacy: group.Privacy,
	}

	return json.Marshal(meta)
}

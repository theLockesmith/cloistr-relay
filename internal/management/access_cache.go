package management

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

// The ban, kind-allow and rate-limit-exempt lists are read on EVERY incoming
// event, and before 2026-10-29 each read was its own Postgres query:
// IsRateLimitExempt, IsPubkeyBanned, IsEventBanned and IsKindAllowed (two
// queries) -- five sequential round trips with no deadline, ahead of the auth
// check. On the shared Postgres server a primary-key lookup has a ~20ms median
// and a tail past 250ms, so these alone could push an event past the 0.5s p95
// alert. The lists are tiny and change only on an admin action, so they are
// held in memory and the per-event checks never touch the database.
//
// Freshness: a write through this Store (NIP-86 or the admin UI) reloads the
// lists before returning, so this process sees it at once. Other processes
// (the second relay pod, cmd/admin) pick it up on their next periodic refresh,
// every AccessRefreshInterval.
//
// Failure: a refresh that fails keeps the last good lists rather than
// clearing them -- an unreachable database must not silently unban everyone.
// Before the first successful load the lists are empty, which matches the old
// per-query behaviour on a database error (not banned, kind allowed, not exempt).

// AccessRefreshInterval is how often StartAccessRefresh reloads the lists.
const AccessRefreshInterval = 30 * time.Second

// accessRefreshTimeout bounds one reload, so a hung connection cannot stall
// the refresher or an admin write behind it.
const accessRefreshTimeout = 5 * time.Second

type accessSets struct {
	bannedPubkeys map[string]struct{}
	bannedEvents  map[string]struct{}
	exemptPubkeys map[string]struct{}
	allowedKinds  map[int]struct{} // empty = no kind restriction
}

var emptyAccess = &accessSets{}

func (s *Store) access() *accessSets {
	if a := s.accessSnap.Load(); a != nil {
		return a
	}
	return emptyAccess
}

// RefreshAccess reloads the ban, kind-allow and rate-limit-exempt lists from
// the database. On error the previous lists stay in place.
func (s *Store) RefreshAccess(ctx context.Context) error {
	// Serialise reloads so a periodic reload that read the tables BEFORE an
	// admin write cannot publish its stale snapshot AFTER the write's own reload.
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, accessRefreshTimeout)
	defer cancel()

	next := &accessSets{}
	var err error
	if next.bannedPubkeys, err = loadStringSet(ctx, s.db, `SELECT pubkey FROM management_banned_pubkeys`); err != nil {
		return fmt.Errorf("load banned pubkeys: %w", err)
	}
	if next.bannedEvents, err = loadStringSet(ctx, s.db, `SELECT event_id FROM management_banned_events`); err != nil {
		return fmt.Errorf("load banned events: %w", err)
	}
	if next.exemptPubkeys, err = loadStringSet(ctx, s.db, `SELECT pubkey FROM management_rate_limit_exempt_pubkeys`); err != nil {
		return fmt.Errorf("load rate-limit exemptions: %w", err)
	}
	if next.allowedKinds, err = loadIntSet(ctx, s.db, `SELECT kind FROM management_allowed_kinds`); err != nil {
		return fmt.Errorf("load allowed kinds: %w", err)
	}

	s.accessSnap.Store(next)
	return nil
}

// StartAccessRefresh reloads the lists every interval until the returned stop
// function is called.
func (s *Store) StartAccessRefresh(interval time.Duration) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := s.RefreshAccess(ctx); err != nil && ctx.Err() == nil {
					log.Printf("management: access list refresh failed, keeping previous lists: %v", err)
				}
			}
		}
	}()
	return cancel
}

// afterWrite reloads the lists after a successful admin write so the change
// applies to the very next event on this process.
func (s *Store) afterWrite(err error) error {
	if err != nil {
		return err
	}
	if rerr := s.RefreshAccess(context.Background()); rerr != nil {
		log.Printf("management: access list reload after write failed (periodic refresh will retry): %v", rerr)
	}
	return nil
}

func loadStringSet(ctx context.Context, db *sql.DB, query string) (map[string]struct{}, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	set := make(map[string]struct{})
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		set[v] = struct{}{}
	}
	return set, rows.Err()
}

func loadIntSet(ctx context.Context, db *sql.DB, query string) (map[int]struct{}, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	set := make(map[int]struct{})
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		set[v] = struct{}{}
	}
	return set, rows.Err()
}

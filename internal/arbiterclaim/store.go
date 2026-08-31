package arbiterclaim

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Store persists claim ownership. Postgres is the serialization point: the relay
// runs two replicas against one database, so a conditional INSERT is what turns
// the advisory client protocol into genuine first-writer-wins.
type Store struct {
	db           *sql.DB
	settleWindow time.Duration
}

// DefaultSettleWindow bounds how long a lower event id may still displace a
// higher one. Inside it, concurrent claimants converge on the lowest id. Outside
// it, the claim is final -- otherwise a late arrival could revoke a claim whose
// holder has already acted on it.
const DefaultSettleWindow = 10 * time.Second

func NewStore(db *sql.DB, settleWindow time.Duration) *Store {
	if settleWindow <= 0 {
		settleWindow = DefaultSettleWindow
	}
	return &Store{db: db, settleWindow: settleWindow}
}

// Init creates the claims table.
func (s *Store) Init() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS arbiter_claims (
			task_id    TEXT PRIMARY KEY,
			event_id   TEXT NOT NULL,
			claimant   TEXT NOT NULL,
			claimed_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	return err
}

// ErrLost reports that a claim lost to an existing holder. It carries the winning
// event id, which is the fencing token the loser needs in order to recognise that
// it lost -- and which the effect site must verify before acting.
type ErrLost struct {
	TaskID  string
	WinnerE string
}

func (e ErrLost) Error() string {
	return fmt.Sprintf("task %s already claimed by event %s", e.TaskID, e.WinnerE)
}

// TryClaim attempts to take ownership of a task, atomically.
//
// First writer wins. Within the settle window a LOWER event id displaces a higher
// one, implementing the lowest-event-id tie-break for genuinely concurrent
// claims; after the window the incumbent is final. Returns ErrLost with the
// winner's event id if the claim did not take.
func (s *Store) TryClaim(c Claim) error {
	var holder string
	err := s.db.QueryRow(`
		INSERT INTO arbiter_claims (task_id, event_id, claimant)
		VALUES ($1, $2, $3)
		ON CONFLICT (task_id) DO UPDATE
		   SET event_id = EXCLUDED.event_id,
		       claimant = EXCLUDED.claimant,
		       claimed_at = now()
		 WHERE arbiter_claims.event_id > EXCLUDED.event_id
		   AND arbiter_claims.claimed_at > now() - $4::interval
		RETURNING event_id`,
		c.TaskID, c.EventID, c.Claimant, fmt.Sprintf("%d milliseconds", s.settleWindow.Milliseconds()),
	).Scan(&holder)

	if errors.Is(err, sql.ErrNoRows) {
		// The ON CONFLICT ... WHERE did not fire: someone else holds it and this
		// claim did not beat them. Read the incumbent so the caller learns the
		// fencing token.
		return ErrLost{TaskID: c.TaskID, WinnerE: s.holderOf(c.TaskID)}
	}
	if err != nil {
		return err
	}
	if holder != c.EventID {
		return ErrLost{TaskID: c.TaskID, WinnerE: holder}
	}
	return nil
}

// Holder returns the event id currently holding a task, or "" if unclaimed.
// This is the fencing-token lookup for the effect site.
func (s *Store) Holder(taskID string) string { return s.holderOf(taskID) }

func (s *Store) holderOf(taskID string) string {
	var id string
	if err := s.db.QueryRow(`SELECT event_id FROM arbiter_claims WHERE task_id = $1`, taskID).Scan(&id); err != nil {
		return ""
	}
	return id
}

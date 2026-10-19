package expiry

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

type Config struct {
	DryRun        bool
	Interval      time.Duration
	BatchSize     int
	BatchPause    time.Duration
	GiftWrapGrace time.Duration
}

func DefaultConfig() Config {
	return Config{
		DryRun:        true,
		Interval:      time.Hour,
		BatchSize:     1000,
		BatchPause:    time.Second,
		GiftWrapGrace: 30 * 24 * time.Hour,
	}
}

type KindCount struct {
	Kind  int
	Count int64
}

type SweepResult struct {
	Deleted     int64
	WouldDelete int64
	Counts      []KindCount
	Malformed   int64
	Elapsed     time.Duration
}

type Sweeper struct {
	db  *sql.DB
	cfg Config
}

func New(db *sql.DB, cfg Config) *Sweeper {
	return &Sweeper{db: db, cfg: cfg}
}

const kindGiftWrap = 1059

// Sweep runs one pass: finds expired events and either logs (dry-run) or
// deletes them in batches. Kind 1059 events get an extended grace period
// so the keeper endpoint (P5b) can serve them to clients catching up on
// gaps in private thread history.
func (s *Sweeper) Sweep(ctx context.Context) (SweepResult, error) {
	start := time.Now()
	nowUnix := start.Unix()
	graceUnix := nowUnix - int64(s.cfg.GiftWrapGrace.Seconds())

	if s.cfg.DryRun {
		return s.dryRun(ctx, nowUnix, graceUnix, start)
	}
	return s.liveRun(ctx, nowUnix, graceUnix, start)
}

func (s *Sweeper) dryRun(ctx context.Context, nowUnix, graceUnix int64, start time.Time) (SweepResult, error) {
	var result SweepResult

	counts, malformed, err := s.countExpired(ctx, nowUnix, graceUnix)
	if err != nil {
		return result, fmt.Errorf("count expired events: %w", err)
	}

	result.Counts = counts
	result.Malformed = malformed
	result.Elapsed = time.Since(start)

	var total int64
	for _, kc := range counts {
		total += kc.Count
	}
	result.WouldDelete = total

	log.Printf("expiry-sweeper [dry-run]: %d expired events across %d kinds, %d malformed tags (took %v)",
		total, len(counts), malformed, result.Elapsed)
	for _, kc := range counts {
		label := "standard"
		if kc.Kind == kindGiftWrap {
			label = fmt.Sprintf("kind 1059 (grace %v)", s.cfg.GiftWrapGrace)
		}
		log.Printf("expiry-sweeper [dry-run]:   kind %d (%s): %d events", kc.Kind, label, kc.Count)
	}

	return result, nil
}

func (s *Sweeper) liveRun(ctx context.Context, nowUnix, graceUnix int64, start time.Time) (SweepResult, error) {
	var result SweepResult

	counts, malformed, err := s.countExpired(ctx, nowUnix, graceUnix)
	if err != nil {
		return result, fmt.Errorf("count expired events: %w", err)
	}
	result.Counts = counts
	result.Malformed = malformed

	var totalToDelete int64
	for _, kc := range counts {
		totalToDelete += kc.Count
	}

	log.Printf("expiry-sweeper: starting deletion of %d expired events (%d malformed skipped)", totalToDelete, malformed)

	var totalDeleted int64
	for ctx.Err() == nil {
		n, err := s.deleteBatch(ctx, nowUnix, graceUnix, s.cfg.BatchSize)
		if err != nil {
			result.Deleted = totalDeleted
			result.Elapsed = time.Since(start)
			return result, fmt.Errorf("delete batch after %d rows: %w", totalDeleted, err)
		}
		if n == 0 {
			break
		}
		totalDeleted += n

		if ctx.Err() != nil {
			break
		}

		select {
		case <-time.After(s.cfg.BatchPause):
		case <-ctx.Done():
		}
	}

	result.Deleted = totalDeleted
	result.Elapsed = time.Since(start)

	log.Printf("expiry-sweeper: deleted %d events (took %v)", totalDeleted, result.Elapsed)
	return result, nil
}

// countExpired returns per-kind counts of events eligible for deletion, plus
// the number of events with malformed (non-numeric) expiration tags.
func (s *Sweeper) countExpired(ctx context.Context, nowUnix, graceUnix int64) ([]KindCount, int64, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, "SET LOCAL statement_timeout = '30s'"); err != nil {
		return nil, 0, fmt.Errorf("set statement_timeout: %w", err)
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT kind, COUNT(*) AS cnt
		FROM event e
		WHERE EXISTS (
			SELECT 1 FROM jsonb_array_elements(e.tags) AS t
			WHERE t->>0 = 'expiration'
			  AND t->>1 ~ '^\d{1,18}$'
			  AND (t->>1)::bigint < CASE WHEN e.kind = 1059 THEN $1::bigint ELSE $2::bigint END
		)
		GROUP BY kind
		ORDER BY cnt DESC
	`, graceUnix, nowUnix)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	var counts []KindCount
	for rows.Next() {
		var kc KindCount
		if err := rows.Scan(&kc.Kind, &kc.Count); err != nil {
			return nil, 0, err
		}
		counts = append(counts, kc)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var malformed int64
	err = tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM event e
		WHERE EXISTS (
			SELECT 1 FROM jsonb_array_elements(e.tags) AS t
			WHERE t->>0 = 'expiration'
			  AND NOT (t->>1 ~ '^\d{1,18}$')
		)
		AND NOT EXISTS (
			SELECT 1 FROM jsonb_array_elements(e.tags) AS t
			WHERE t->>0 = 'expiration'
			  AND t->>1 ~ '^\d{1,18}$'
		)
	`).Scan(&malformed)
	if err != nil {
		return counts, 0, err
	}

	if err := tx.Commit(); err != nil {
		return counts, malformed, err
	}

	return counts, malformed, nil
}

// deleteBatch removes up to limit expired events in one statement. It skips
// events with malformed expiration tags (non-numeric values).
func (s *Sweeper) deleteBatch(ctx context.Context, nowUnix, graceUnix int64, limit int) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, "SET LOCAL statement_timeout = '30s'"); err != nil {
		return 0, fmt.Errorf("set statement_timeout: %w", err)
	}

	res, err := tx.ExecContext(ctx, `
		DELETE FROM event
		WHERE id IN (
			SELECT e.id FROM event e
			WHERE EXISTS (
				SELECT 1 FROM jsonb_array_elements(e.tags) AS t
				WHERE t->>0 = 'expiration'
				  AND t->>1 ~ '^\d{1,18}$'
				  AND (t->>1)::bigint < CASE WHEN e.kind = 1059 THEN $1::bigint ELSE $2::bigint END
			)
			LIMIT $3
		)
	`, graceUnix, nowUnix, limit)
	if err != nil {
		return 0, err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}

// Start runs the sweep loop until ctx is cancelled. It logs each pass.
func (s *Sweeper) Start(ctx context.Context) {
	log.Printf("expiry-sweeper: started (dry_run=%v interval=%v batch=%d gift_wrap_grace=%v)",
		s.cfg.DryRun, s.cfg.Interval, s.cfg.BatchSize, s.cfg.GiftWrapGrace)

	if ctx.Err() == nil {
		if _, err := s.Sweep(ctx); err != nil {
			log.Printf("expiry-sweeper: initial sweep error: %v", err)
		}
	}

	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if _, err := s.Sweep(ctx); err != nil {
				log.Printf("expiry-sweeper: error: %v", err)
			}
		case <-ctx.Done():
			log.Println("expiry-sweeper: stopped")
			return
		}
	}
}

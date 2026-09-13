package membership

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// PaymentStatus tracks the lifecycle of a Lightning invoice.
type PaymentStatus string

const (
	PaymentPending  PaymentStatus = "pending"
	PaymentSettled  PaymentStatus = "settled"
	PaymentExpired  PaymentStatus = "expired"
)

// PendingPayment is an invoice waiting for settlement that, once paid,
// upgrades a member's tier.
type PendingPayment struct {
	PaymentHash string
	Pubkey      string
	Tier        MemberTier
	AmountSats  int64
	PeriodDays  int
	Status      PaymentStatus
	CreatedAt   time.Time
	SettledAt   time.Time // zero until settled
}

// PaymentStore handles pending_payments persistence.
type PaymentStore struct {
	db *sql.DB
}

// NewPaymentStore creates a new payment store.
func NewPaymentStore(db *sql.DB) *PaymentStore {
	return &PaymentStore{db: db}
}

// InitSchema creates the pending_payments table and indexes.
func (s *PaymentStore) InitSchema(ctx context.Context) error {
	schema := `
		CREATE TABLE IF NOT EXISTS pending_payments (
			payment_hash   TEXT PRIMARY KEY,
			pubkey         TEXT NOT NULL,
			tier           TEXT NOT NULL,
			amount_sats    BIGINT NOT NULL,
			period_days    INT NOT NULL,
			status         TEXT NOT NULL DEFAULT 'pending',
			created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			settled_at     TIMESTAMPTZ
		);

		CREATE INDEX IF NOT EXISTS idx_pending_payments_pubkey
			ON pending_payments(pubkey);
		CREATE INDEX IF NOT EXISTS idx_pending_payments_status
			ON pending_payments(status);
	`
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// Create inserts a new pending payment.
func (s *PaymentStore) Create(ctx context.Context, p PendingPayment) error {
	if p.PaymentHash == "" {
		return fmt.Errorf("payments: empty payment hash")
	}
	if p.Pubkey == "" {
		return fmt.Errorf("payments: empty pubkey")
	}
	if !p.Tier.IsValid() {
		return fmt.Errorf("payments: invalid tier %q", p.Tier)
	}
	if p.AmountSats <= 0 {
		return fmt.Errorf("payments: amount must be positive, got %d", p.AmountSats)
	}
	if p.PeriodDays <= 0 {
		return fmt.Errorf("payments: period must be positive, got %d", p.PeriodDays)
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO pending_payments (payment_hash, pubkey, tier, amount_sats, period_days, status, created_at)
		VALUES ($1, $2, $3, $4, $5, 'pending', $6)
	`, p.PaymentHash, p.Pubkey, p.Tier, p.AmountSats, p.PeriodDays, time.Now())
	return err
}

// GetByHash retrieves a payment by its hash.
func (s *PaymentStore) GetByHash(ctx context.Context, hash string) (*PendingPayment, error) {
	var p PendingPayment
	var settledAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT payment_hash, pubkey, tier, amount_sats, period_days, status, created_at, settled_at
		FROM pending_payments WHERE payment_hash = $1
	`, hash).Scan(&p.PaymentHash, &p.Pubkey, &p.Tier, &p.AmountSats, &p.PeriodDays, &p.Status, &p.CreatedAt, &settledAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if settledAt.Valid {
		p.SettledAt = settledAt.Time
	}
	return &p, nil
}

// MarkSettled transitions a pending payment to settled. Returns sql.ErrNoRows
// if the payment does not exist or is not in pending status (idempotent
// guard against double-grant on webhook retries).
func (s *PaymentStore) MarkSettled(ctx context.Context, hash string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE pending_payments
		SET status = 'settled', settled_at = $1
		WHERE payment_hash = $2 AND status = 'pending'
	`, time.Now(), hash)
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

// MarkExpired transitions stale pending payments to expired. Returns the
// number of payments expired.
func (s *PaymentStore) MarkExpired(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	result, err := s.db.ExecContext(ctx, `
		UPDATE pending_payments
		SET status = 'expired'
		WHERE status = 'pending' AND created_at < $1
	`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ListByPubkey returns all payments for a pubkey, newest first.
func (s *PaymentStore) ListByPubkey(ctx context.Context, pubkey string) ([]PendingPayment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT payment_hash, pubkey, tier, amount_sats, period_days, status, created_at, settled_at
		FROM pending_payments WHERE pubkey = $1 ORDER BY created_at DESC
	`, pubkey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanPayments(rows)
}

// ListPending returns all pending payments, oldest first (for expiry sweeps).
func (s *PaymentStore) ListPending(ctx context.Context) ([]PendingPayment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT payment_hash, pubkey, tier, amount_sats, period_days, status, created_at, settled_at
		FROM pending_payments WHERE status = 'pending' ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanPayments(rows)
}

// CountByStatus returns counts grouped by status.
func (s *PaymentStore) CountByStatus(ctx context.Context) (map[PaymentStatus]int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT status, COUNT(*) FROM pending_payments GROUP BY status
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[PaymentStatus]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[PaymentStatus(status)] = count
	}
	return counts, rows.Err()
}

func scanPayments(rows *sql.Rows) ([]PendingPayment, error) {
	var payments []PendingPayment
	for rows.Next() {
		var p PendingPayment
		var settledAt sql.NullTime

		if err := rows.Scan(&p.PaymentHash, &p.Pubkey, &p.Tier, &p.AmountSats, &p.PeriodDays, &p.Status, &p.CreatedAt, &settledAt); err != nil {
			return nil, err
		}
		if settledAt.Valid {
			p.SettledAt = settledAt.Time
		}
		payments = append(payments, p)
	}
	return payments, rows.Err()
}

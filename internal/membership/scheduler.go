package membership

import (
	"context"
	"log"
	"sync"
	"time"

	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/metrics"
)

// ExpiryScheduler periodically downgrades expired tiers and marks stale
// pending payments as expired. It runs in a background goroutine with a
// configurable interval (default: 1 hour).
type ExpiryScheduler struct {
	memberStore  *Store
	paymentStore *PaymentStore

	interval       time.Duration
	invoiceMaxAge  time.Duration // pending invoices older than this are expired
	stopCh         chan struct{}
	wg             sync.WaitGroup
}

// ExpirySchedulerConfig configures the scheduler.
type ExpirySchedulerConfig struct {
	MemberStore   *Store
	PaymentStore  *PaymentStore
	Interval      time.Duration // how often to check (default: 1h)
	InvoiceMaxAge time.Duration // pending invoices older than this are expired (default: 24h)
}

// NewExpiryScheduler creates a scheduler. Call Start() to begin.
func NewExpiryScheduler(cfg ExpirySchedulerConfig) *ExpiryScheduler {
	interval := cfg.Interval
	if interval <= 0 {
		interval = time.Hour
	}
	invoiceMaxAge := cfg.InvoiceMaxAge
	if invoiceMaxAge <= 0 {
		invoiceMaxAge = 24 * time.Hour
	}
	return &ExpiryScheduler{
		memberStore:   cfg.MemberStore,
		paymentStore:  cfg.PaymentStore,
		interval:      interval,
		invoiceMaxAge: invoiceMaxAge,
		stopCh:        make(chan struct{}),
	}
}

// Start begins the background expiry sweep. Safe to call once.
func (s *ExpiryScheduler) Start() {
	s.wg.Add(1)
	go s.run()
	log.Printf("Expiry scheduler started (interval=%v, invoice_max_age=%v)", s.interval, s.invoiceMaxAge)
}

// Stop signals the scheduler to stop and waits for it to finish.
func (s *ExpiryScheduler) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	log.Println("Expiry scheduler stopped")
}

func (s *ExpiryScheduler) run() {
	defer s.wg.Done()

	// Run once immediately on start, then on interval.
	s.sweep()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.sweep()
		case <-s.stopCh:
			return
		}
	}
}

func (s *ExpiryScheduler) sweep() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Downgrade expired member tiers.
	if s.memberStore != nil {
		n, err := s.memberStore.ResetExpiredTiers(ctx)
		if err != nil {
			log.Printf("expiry-scheduler: ResetExpiredTiers error: %v", err)
		} else if n > 0 {
			log.Printf("expiry-scheduler: downgraded %d expired tier(s) to free", n)
			metrics.TierExpirations.Add(float64(n))
		}
	}

	// Expire stale pending invoices.
	if s.paymentStore != nil {
		n, err := s.paymentStore.MarkExpired(ctx, s.invoiceMaxAge)
		if err != nil {
			log.Printf("expiry-scheduler: MarkExpired error: %v", err)
		} else if n > 0 {
			log.Printf("expiry-scheduler: expired %d stale pending invoice(s)", n)
			metrics.PaymentsPending.Sub(float64(n))
		}
	}
}

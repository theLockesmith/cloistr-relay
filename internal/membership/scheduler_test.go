package membership

import (
	"testing"
	"time"
)

func TestExpiryScheduler_Defaults(t *testing.T) {
	s := NewExpiryScheduler(ExpirySchedulerConfig{})

	if s.interval != time.Hour {
		t.Errorf("interval = %v, want 1h", s.interval)
	}
	if s.invoiceMaxAge != 24*time.Hour {
		t.Errorf("invoiceMaxAge = %v, want 24h", s.invoiceMaxAge)
	}
}

func TestExpiryScheduler_CustomConfig(t *testing.T) {
	s := NewExpiryScheduler(ExpirySchedulerConfig{
		Interval:      15 * time.Minute,
		InvoiceMaxAge: 6 * time.Hour,
	})

	if s.interval != 15*time.Minute {
		t.Errorf("interval = %v, want 15m", s.interval)
	}
	if s.invoiceMaxAge != 6*time.Hour {
		t.Errorf("invoiceMaxAge = %v, want 6h", s.invoiceMaxAge)
	}
}

func TestExpiryScheduler_StartStop(t *testing.T) {
	// Verify Start/Stop does not hang or panic with nil stores.
	s := NewExpiryScheduler(ExpirySchedulerConfig{
		Interval: 50 * time.Millisecond,
	})
	s.Start()

	// Let it tick at least once.
	time.Sleep(100 * time.Millisecond)

	// Stop should not hang.
	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() hung")
	}
}

func TestExpiryScheduler_ZeroInterval(t *testing.T) {
	s := NewExpiryScheduler(ExpirySchedulerConfig{
		Interval: 0,
	})
	if s.interval != time.Hour {
		t.Errorf("zero interval should default to 1h, got %v", s.interval)
	}
}

func TestExpiryScheduler_NegativeInvoiceMaxAge(t *testing.T) {
	s := NewExpiryScheduler(ExpirySchedulerConfig{
		InvoiceMaxAge: -1 * time.Hour,
	})
	if s.invoiceMaxAge != 24*time.Hour {
		t.Errorf("negative invoiceMaxAge should default to 24h, got %v", s.invoiceMaxAge)
	}
}

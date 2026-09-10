package membership

import (
	"testing"
)

func TestPaymentStatusConstants(t *testing.T) {
	// Verify the status strings match what the SQL schema expects.
	if PaymentPending != "pending" {
		t.Errorf("PaymentPending = %q, want \"pending\"", PaymentPending)
	}
	if PaymentSettled != "settled" {
		t.Errorf("PaymentSettled = %q, want \"settled\"", PaymentSettled)
	}
	if PaymentExpired != "expired" {
		t.Errorf("PaymentExpired = %q, want \"expired\"", PaymentExpired)
	}
}

func TestPendingPayment_Validation(t *testing.T) {
	// PaymentStore.Create validates its inputs before hitting the DB.
	// We can't test the full Create without a DB, but we test the
	// validation logic by constructing a PaymentStore with a nil DB
	// and checking that validation errors fire before any DB call.

	store := &PaymentStore{db: nil}

	cases := []struct {
		name    string
		payment PendingPayment
		wantErr string
	}{
		{
			name:    "empty hash",
			payment: PendingPayment{PaymentHash: "", Pubkey: "abc", Tier: TierPremium, AmountSats: 100, PeriodDays: 30},
			wantErr: "empty payment hash",
		},
		{
			name:    "empty pubkey",
			payment: PendingPayment{PaymentHash: "hash1", Pubkey: "", Tier: TierPremium, AmountSats: 100, PeriodDays: 30},
			wantErr: "empty pubkey",
		},
		{
			name:    "invalid tier",
			payment: PendingPayment{PaymentHash: "hash1", Pubkey: "abc", Tier: "bogus", AmountSats: 100, PeriodDays: 30},
			wantErr: "invalid tier",
		},
		{
			name:    "zero amount",
			payment: PendingPayment{PaymentHash: "hash1", Pubkey: "abc", Tier: TierPremium, AmountSats: 0, PeriodDays: 30},
			wantErr: "amount must be positive",
		},
		{
			name:    "negative amount",
			payment: PendingPayment{PaymentHash: "hash1", Pubkey: "abc", Tier: TierPremium, AmountSats: -10, PeriodDays: 30},
			wantErr: "amount must be positive",
		},
		{
			name:    "zero period",
			payment: PendingPayment{PaymentHash: "hash1", Pubkey: "abc", Tier: TierPremium, AmountSats: 100, PeriodDays: 0},
			wantErr: "period must be positive",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := store.Create(t.Context(), tc.payment)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !containsString(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestTierIsValid_PaymentContext(t *testing.T) {
	valid := []MemberTier{TierFree, TierHybrid, TierPremium, TierEnterprise}
	for _, tier := range valid {
		if !tier.IsValid() {
			t.Errorf("IsValid(%q) = false, want true", tier)
		}
	}

	invalid := []MemberTier{"bogus", "", "gold", "PREMIUM"}
	for _, tier := range invalid {
		if tier.IsValid() {
			t.Errorf("IsValid(%q) = true, want false", tier)
		}
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

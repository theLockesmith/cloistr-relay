package zaps

import (
	"context"
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

func TestKindConstants(t *testing.T) {
	if KindZapRequest != 9734 {
		t.Errorf("KindZapRequest should be 9734, got %d", KindZapRequest)
	}
	if KindZapReceipt != 9735 {
		t.Errorf("KindZapReceipt should be 9735, got %d", KindZapReceipt)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Enabled {
		t.Error("Expected Enabled to be true by default")
	}
	if !cfg.ValidateReceipts {
		t.Error("Expected ValidateReceipts to be true by default")
	}
	if cfg.RequireBolt11 {
		t.Error("Expected RequireBolt11 to be false by default")
	}
}

func TestParseZapReceipt(t *testing.T) {
	// Create a valid zap receipt
	zapRequest := `{"kind":9734,"tags":[["p","recipient123"],["amount","21000"]]}`
	event := &nostr.Event{
		Kind: KindZapReceipt,
		Tags: nostr.Tags{
			{"p", "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"},
			{"P", "sender12sender12sender12sender12sender12sender12sender12sender12"},
			{"e", "eventid123"},
			{"bolt11", "lnbc210n1ptest"},
			{"description", zapRequest},
			{"preimage", "preimage123"},
		},
	}

	zr, err := ParseZapReceipt(event)
	if err != nil {
		t.Fatalf("Failed to parse zap receipt: %v", err)
	}

	if zr.RecipientPubkey != "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234" {
		t.Errorf("Wrong recipient pubkey: %s", zr.RecipientPubkey)
	}
	if zr.SenderPubkey != "sender12sender12sender12sender12sender12sender12sender12sender12" {
		t.Errorf("Wrong sender pubkey: %s", zr.SenderPubkey)
	}
	if zr.EventID != "eventid123" {
		t.Errorf("Wrong event ID: %s", zr.EventID)
	}
	if zr.Bolt11 != "lnbc210n1ptest" {
		t.Errorf("Wrong bolt11: %s", zr.Bolt11)
	}
	if zr.AmountMsat != 21000 {
		t.Errorf("Wrong amount: %d", zr.AmountMsat)
	}
}

func TestParseZapReceipt_WrongKind(t *testing.T) {
	event := &nostr.Event{Kind: 1}
	_, err := ParseZapReceipt(event)
	if err == nil {
		t.Error("Expected error for wrong kind")
	}
}

func TestZapReceipt_Validate(t *testing.T) {
	tests := []struct {
		name      string
		zr        *ZapReceipt
		expectErr bool
	}{
		{
			name: "valid receipt",
			zr: &ZapReceipt{
				RecipientPubkey: "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234",
				Description:     `{"kind":9734}`,
			},
			expectErr: false,
		},
		{
			name: "missing recipient",
			zr: &ZapReceipt{
				Description: `{"kind":9734}`,
			},
			expectErr: true,
		},
		{
			name: "invalid recipient length",
			zr: &ZapReceipt{
				RecipientPubkey: "tooshort",
				Description:     `{"kind":9734}`,
			},
			expectErr: true,
		},
		{
			name: "missing description",
			zr: &ZapReceipt{
				RecipientPubkey: "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234",
			},
			expectErr: true,
		},
		{
			name: "invalid JSON in description",
			zr: &ZapReceipt{
				RecipientPubkey: "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234",
				Description:     "not valid json",
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.zr.Validate()
			if tt.expectErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestRejectInvalidZapReceipt(t *testing.T) {
	handler := NewHandler(DefaultConfig())
	rejectFn := handler.RejectInvalidZapReceipt()

	tests := []struct {
		name         string
		event        *nostr.Event
		expectReject bool
	}{
		{
			name:         "non-zap event passes",
			event:        &nostr.Event{Kind: 1},
			expectReject: false,
		},
		{
			name: "valid zap receipt passes",
			event: &nostr.Event{
				Kind: KindZapReceipt,
				Tags: nostr.Tags{
					{"p", "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"},
					{"description", `{"kind":9734}`},
				},
			},
			expectReject: false,
		},
		{
			name: "invalid zap receipt rejected",
			event: &nostr.Event{
				Kind: KindZapReceipt,
				Tags: nostr.Tags{
					// Missing p tag and description
				},
			},
			expectReject: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reject, _ := rejectFn(context.Background(), tt.event)
			if reject != tt.expectReject {
				t.Errorf("Expected reject=%v, got %v", tt.expectReject, reject)
			}
		})
	}
}

func TestIsZapReceipt(t *testing.T) {
	if !IsZapReceipt(&nostr.Event{Kind: 9735}) {
		t.Error("Should be zap receipt")
	}
	if IsZapReceipt(&nostr.Event{Kind: 1}) {
		t.Error("Should not be zap receipt")
	}
}

func TestIsZapRequest(t *testing.T) {
	if !IsZapRequest(&nostr.Event{Kind: 9734}) {
		t.Error("Should be zap request")
	}
	if IsZapRequest(&nostr.Event{Kind: 1}) {
		t.Error("Should not be zap request")
	}
}

func TestValidateBolt11Basic(t *testing.T) {
	tests := []struct {
		bolt11 string
		valid  bool
	}{
		{"lnbc210n1ptest", true},
		{"LNBC210n1ptest", true},
		{"lntb100n1ptest", true},
		{"lnbcrt1ptest", true},
		{"lnsb1ptest", true},
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.bolt11, func(t *testing.T) {
			if ValidateBolt11Basic(tt.bolt11) != tt.valid {
				t.Errorf("Expected valid=%v for %s", tt.valid, tt.bolt11)
			}
		})
	}
}

func TestExtractLNURLFromProfile(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "with lud06",
			content:  `{"lud06":"lnurl1dp68gurn8ghj7..."}`,
			expected: "lnurl1dp68gurn8ghj7...",
		},
		{
			name:     "with lud16",
			content:  `{"lud16":"user@getalby.com"}`,
			expected: "lightning:user@getalby.com",
		},
		{
			name:     "lud06 takes precedence",
			content:  `{"lud06":"lnurl1...","lud16":"user@example.com"}`,
			expected: "lnurl1...",
		},
		{
			name:     "no lnurl",
			content:  `{"name":"test"}`,
			expected: "",
		},
		{
			name:     "invalid json",
			content:  "not json",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractLNURLFromProfile(tt.content)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestGetZappedEventID(t *testing.T) {
	event := &nostr.Event{
		Kind: KindZapReceipt,
		Tags: nostr.Tags{
			{"p", "pubkey"},
			{"e", "eventid123"},
		},
	}

	if GetZappedEventID(event) != "eventid123" {
		t.Error("Failed to get zapped event ID")
	}

	eventNoE := &nostr.Event{Kind: KindZapReceipt, Tags: nostr.Tags{}}
	if GetZappedEventID(eventNoE) != "" {
		t.Error("Should return empty for no e tag")
	}
}

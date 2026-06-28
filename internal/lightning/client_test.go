package lightning

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateInvoice(t *testing.T) {
	var gotKey, gotPath, gotMethod string
	var gotBody createInvoiceRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Api-Key")
		gotPath = r.URL.Path
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(createInvoiceResponse{
			PaymentHash:    "abc123",
			PaymentRequest: "lnbc100n1pjexample",
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "invoice-key-xyz")
	inv, err := c.CreateInvoice(context.Background(), 100, "premium tier", "https://relay/payments/webhook")
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}

	if gotKey != "invoice-key-xyz" {
		t.Errorf("X-Api-Key = %q, want %q", gotKey, "invoice-key-xyz")
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/payments" {
		t.Errorf("request = %s %s, want POST /api/v1/payments", gotMethod, gotPath)
	}
	if gotBody.Out || gotBody.Amount != 100 || gotBody.Webhook == "" {
		t.Errorf("request body = %+v, want out:false amount:100 with webhook", gotBody)
	}
	if inv.PaymentHash != "abc123" || inv.Bolt11 != "lnbc100n1pjexample" {
		t.Errorf("invoice = %+v, want hash abc123 / bolt11 set", inv)
	}
}

func TestCreateInvoiceBolt11Fallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Newer LNbits: bolt11 field instead of payment_request.
		_, _ = w.Write([]byte(`{"payment_hash":"hh","bolt11":"lnbc1newfield"}`))
	}))
	defer srv.Close()

	inv, err := NewClient(srv.URL, "k").CreateInvoice(context.Background(), 50, "memo", "")
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	if inv.Bolt11 != "lnbc1newfield" {
		t.Errorf("bolt11 = %q, want lnbc1newfield", inv.Bolt11)
	}
}

func TestCreateInvoiceRejectsNonPositive(t *testing.T) {
	c := NewClient("https://unused", "k")
	if _, err := c.CreateInvoice(context.Background(), 0, "m", ""); err == nil {
		t.Error("expected error for zero amount, got nil")
	}
}

func TestCreateInvoiceServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"detail":"insufficient permissions"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL, "bad").CreateInvoice(context.Background(), 100, "m", ""); err == nil {
		t.Error("expected error on 401, got nil")
	}
}

func TestCheckPayment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/payments/hash99" {
			t.Errorf("path = %q, want /api/v1/payments/hash99", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"paid":true,"details":{"amount":150000}}`)) // 150000 msat = 150 sats
	}))
	defer srv.Close()

	st, err := NewClient(srv.URL, "k").CheckPayment(context.Background(), "hash99")
	if err != nil {
		t.Fatalf("CheckPayment: %v", err)
	}
	if !st.Paid {
		t.Error("Paid = false, want true")
	}
	if st.AmountSats != 150 {
		t.Errorf("AmountSats = %d, want 150", st.AmountSats)
	}
}

func TestCheckPaymentEmptyHash(t *testing.T) {
	if _, err := NewClient("https://unused", "k").CheckPayment(context.Background(), ""); err == nil {
		t.Error("expected error for empty hash, got nil")
	}
}

func TestTrimTrailingSlash(t *testing.T) {
	c := NewClient("https://lnbits.example.com///", "k")
	if c.baseURL != "https://lnbits.example.com" {
		t.Errorf("baseURL = %q, want trailing slashes trimmed", c.baseURL)
	}
}

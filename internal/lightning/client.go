// Package lightning implements a client for a self-hosted LNbits instance,
// used to collect Bitcoin payments that grant or upgrade membership tiers.
//
// The relay creates BOLT-11 invoices via LNbits and is notified of settlement
// via an LNbits webhook. Webhook bodies are treated as a trigger only — tier
// grants always re-query LNbits for the authoritative settle status before
// acting (see CheckPayment). This guards against forged webhook calls.
package lightning

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client talks to the LNbits HTTP API.
type Client struct {
	baseURL    string
	invoiceKey string
	httpClient *http.Client
}

// NewClient creates an LNbits client. baseURL is the LNbits root (e.g.
// "https://lnbits.example.com"); invoiceKey is the wallet's invoice/read key,
// sent as the X-Api-Key header.
func NewClient(baseURL, invoiceKey string) *Client {
	return &Client{
		baseURL:    trimTrailingSlash(baseURL),
		invoiceKey: invoiceKey,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Invoice is the result of creating a BOLT-11 invoice.
type Invoice struct {
	PaymentHash string // hash used to look up settle status
	Bolt11      string // the payable invoice string
}

// PaymentStatus is the settle state of a previously created invoice.
type PaymentStatus struct {
	Paid       bool
	AmountSats int64 // settled amount in sats (msat/1000)
}

// createInvoiceRequest matches the LNbits POST /api/v1/payments body for an
// incoming (out:false) invoice.
type createInvoiceRequest struct {
	Out     bool   `json:"out"`
	Amount  int64  `json:"amount"` // sats
	Memo    string `json:"memo"`
	Webhook string `json:"webhook,omitempty"`
}

type createInvoiceResponse struct {
	PaymentHash    string `json:"payment_hash"`
	PaymentRequest string `json:"payment_request"`
	// Newer LNbits versions return "bolt11"; accept both.
	Bolt11 string `json:"bolt11"`
}

// CreateInvoice creates an incoming BOLT-11 invoice for amountSats. webhookURL,
// if non-empty, is the URL LNbits will POST to on settlement.
func (c *Client) CreateInvoice(ctx context.Context, amountSats int64, memo, webhookURL string) (Invoice, error) {
	if amountSats <= 0 {
		return Invoice{}, fmt.Errorf("lightning: invoice amount must be positive, got %d", amountSats)
	}

	reqBody := createInvoiceRequest{Out: false, Amount: amountSats, Memo: memo, Webhook: webhookURL}
	var resp createInvoiceResponse
	if err := c.do(ctx, http.MethodPost, "/api/v1/payments", reqBody, &resp); err != nil {
		return Invoice{}, err
	}

	bolt11 := resp.PaymentRequest
	if bolt11 == "" {
		bolt11 = resp.Bolt11
	}
	if resp.PaymentHash == "" || bolt11 == "" {
		return Invoice{}, fmt.Errorf("lightning: LNbits returned an incomplete invoice (hash=%q bolt11=%q)", resp.PaymentHash, bolt11)
	}
	return Invoice{PaymentHash: resp.PaymentHash, Bolt11: bolt11}, nil
}

type paymentStatusResponse struct {
	Paid    bool `json:"paid"`
	Details struct {
		Amount int64 `json:"amount"` // msat (negative for outgoing)
	} `json:"details"`
}

// CheckPayment queries LNbits for the authoritative settle status of an invoice
// by its payment hash. This is the source of truth for granting a tier — never
// trust a webhook body alone.
func (c *Client) CheckPayment(ctx context.Context, paymentHash string) (PaymentStatus, error) {
	if paymentHash == "" {
		return PaymentStatus{}, fmt.Errorf("lightning: empty payment hash")
	}
	var resp paymentStatusResponse
	if err := c.do(ctx, http.MethodGet, "/api/v1/payments/"+paymentHash, nil, &resp); err != nil {
		return PaymentStatus{}, err
	}
	// LNbits reports amount in msat; incoming payments are positive.
	sats := resp.Details.Amount / 1000
	if sats < 0 {
		sats = -sats
	}
	return PaymentStatus{Paid: resp.Paid, AmountSats: sats}, nil
}

// do performs an LNbits API request, encoding body (if non-nil) as JSON and
// decoding a successful response into out (if non-nil).
func (c *Client) do(ctx context.Context, method, path string, body, out interface{}) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("lightning: marshal request: %w", err)
		}
		reader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("lightning: build request: %w", err)
	}
	req.Header.Set("X-Api-Key", c.invoiceKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("lightning: request to %s failed: %w", path, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("lightning: LNbits %s %s returned %d: %s", method, path, resp.StatusCode, string(respBody))
	}
	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("lightning: decode response: %w", err)
		}
	}
	return nil
}

func trimTrailingSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

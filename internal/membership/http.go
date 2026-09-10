package membership

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"

	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/lightning"
)

// PaymentHandler serves the invoice and webhook HTTP endpoints that connect
// LNbits payments to tier upgrades.
type PaymentHandler struct {
	memberStore  *Store
	paymentStore *PaymentStore
	lnClient     *lightning.Client

	// webhookSecret is the shared secret that LNbits includes in the
	// webhook URL path. Empty disables path-segment verification.
	webhookSecret string

	// publicURL is the externally reachable base URL (e.g.
	// "https://relay.cloistr.xyz") so we can construct the webhook
	// callback URL for LNbits.
	publicURL string

	// tierPrices maps purchasable tiers to their price in sats.
	// A zero or missing entry means the tier is not self-serve.
	tierPrices map[MemberTier]int64

	// periodDays is the default subscription period.
	periodDays int
}

// PaymentHandlerConfig configures the payment HTTP handler.
type PaymentHandlerConfig struct {
	MemberStore   *Store
	PaymentStore  *PaymentStore
	LNClient      *lightning.Client
	WebhookSecret string
	PublicURL     string
	TierPrices    map[MemberTier]int64
	PeriodDays    int
}

// NewPaymentHandler creates the invoice/webhook HTTP handler.
func NewPaymentHandler(cfg PaymentHandlerConfig) *PaymentHandler {
	return &PaymentHandler{
		memberStore:   cfg.MemberStore,
		paymentStore:  cfg.PaymentStore,
		lnClient:      cfg.LNClient,
		webhookSecret: cfg.WebhookSecret,
		publicURL:     strings.TrimRight(cfg.PublicURL, "/"),
		tierPrices:    cfg.TierPrices,
		periodDays:    cfg.PeriodDays,
	}
}

// RegisterRoutes mounts the payment endpoints on mux.
func (h *PaymentHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/payments/invoice", h.handleInvoice)
	if h.webhookSecret != "" {
		mux.HandleFunc("/payments/webhook/"+h.webhookSecret, h.handleWebhook)
	} else {
		mux.HandleFunc("/payments/webhook", h.handleWebhook)
	}
	log.Println("Payment endpoints registered at /payments/")
}

// invoiceRequest is the JSON body for POST /payments/invoice.
type invoiceRequest struct {
	Tier string `json:"tier"`
}

// invoiceResponse is returned on successful invoice creation.
type invoiceResponse struct {
	PaymentHash string `json:"payment_hash"`
	Bolt11      string `json:"bolt11"`
	AmountSats  int64  `json:"amount_sats"`
	Tier        string `json:"tier"`
	PeriodDays  int    `json:"period_days"`
}

// handleInvoice creates a BOLT-11 invoice for a tier upgrade.
// POST /payments/invoice with NIP-98 auth.
func (h *PaymentHandler) handleInvoice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
		return
	}

	// NIP-98 user auth (any authenticated user, not admin-gated).
	pubkey, err := validateNIP98UserAuth(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}

	// Parse request body.
	var req invoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	tier := MemberTier(req.Tier)
	if !tier.IsValid() || tier == TierFree {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid or non-purchasable tier: %q", req.Tier)})
		return
	}

	price, ok := h.tierPrices[tier]
	if !ok || price <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("tier %q is not available for self-serve purchase", req.Tier)})
		return
	}

	// User must be a member (joined via NIP-43) to upgrade.
	isMember, err := h.memberStore.IsMember(r.Context(), pubkey)
	if err != nil {
		log.Printf("payments: membership check failed for %s: %v", pubkey[:8], err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if !isMember {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "you must join the relay first (kind 28934)"})
		return
	}

	// Build the webhook callback URL.
	webhookURL := h.publicURL + "/payments/webhook"
	if h.webhookSecret != "" {
		webhookURL += "/" + h.webhookSecret
	}

	memo := fmt.Sprintf("cloistr relay %s tier (%d days)", tier, h.periodDays)
	invoice, err := h.lnClient.CreateInvoice(r.Context(), price, memo, webhookURL)
	if err != nil {
		log.Printf("payments: LNbits CreateInvoice failed for %s: %v", pubkey[:8], err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to create invoice"})
		return
	}

	// Record the pending payment.
	if err := h.paymentStore.Create(r.Context(), PendingPayment{
		PaymentHash: invoice.PaymentHash,
		Pubkey:      pubkey,
		Tier:        tier,
		AmountSats:  price,
		PeriodDays:  h.periodDays,
		Status:      PaymentPending,
	}); err != nil {
		log.Printf("payments: failed to record pending payment %s for %s: %v", invoice.PaymentHash, pubkey[:8], err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	log.Printf("payments: invoice created for %s tier=%s hash=%s sats=%d", pubkey[:8], tier, invoice.PaymentHash[:8], price)

	writeJSON(w, http.StatusOK, invoiceResponse{
		PaymentHash: invoice.PaymentHash,
		Bolt11:      invoice.Bolt11,
		AmountSats:  price,
		Tier:        string(tier),
		PeriodDays:  h.periodDays,
	})
}

// handleWebhook processes LNbits settlement notifications.
// POST /payments/webhook[/<secret>]
func (h *PaymentHandler) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Parse the webhook body. LNbits sends {"payment_hash": "..."} among
	// other fields. We only use payment_hash as a trigger; settlement is
	// verified against LNbits before granting.
	var body struct {
		PaymentHash string `json:"payment_hash"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PaymentHash == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Look up the pending payment.
	payment, err := h.paymentStore.GetByHash(r.Context(), body.PaymentHash)
	if err != nil {
		log.Printf("payments: webhook lookup failed for hash %s: %v", body.PaymentHash[:8], err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if payment == nil {
		// Unknown hash. Could be a payment we did not create.
		log.Printf("payments: webhook for unknown hash %s", body.PaymentHash[:8])
		w.WriteHeader(http.StatusOK) // 200 so LNbits does not retry
		return
	}
	if payment.Status != PaymentPending {
		// Already settled or expired. Idempotent.
		w.WriteHeader(http.StatusOK)
		return
	}

	// Re-verify payment against LNbits (never trust webhook body alone).
	status, err := h.lnClient.CheckPayment(r.Context(), body.PaymentHash)
	if err != nil {
		log.Printf("payments: LNbits check failed for %s: %v", body.PaymentHash[:8], err)
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	if !status.Paid {
		log.Printf("payments: webhook fired but LNbits says not paid for %s", body.PaymentHash[:8])
		w.WriteHeader(http.StatusOK)
		return
	}

	// Verify amount matches (prevent underpayment).
	if status.AmountSats < payment.AmountSats {
		log.Printf("payments: underpayment for %s: got %d sats, want %d", body.PaymentHash[:8], status.AmountSats, payment.AmountSats)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Mark settled (idempotent guard: only transitions pending -> settled).
	if err := h.paymentStore.MarkSettled(r.Context(), body.PaymentHash); err != nil {
		log.Printf("payments: mark settled failed for %s: %v", body.PaymentHash[:8], err)
		w.WriteHeader(http.StatusOK) // don't retry
		return
	}

	// Upgrade the member's tier. Extend from max(now, current_expiry).
	member, err := h.memberStore.GetMember(r.Context(), payment.Pubkey)
	if err != nil || member == nil {
		log.Printf("payments: member lookup failed for %s after settle: %v", payment.Pubkey[:8], err)
		w.WriteHeader(http.StatusOK)
		return
	}

	baseTime := time.Now()
	if !member.TierExpiresAt.IsZero() && member.TierExpiresAt.After(baseTime) {
		baseTime = member.TierExpiresAt
	}
	newExpiry := baseTime.Add(time.Duration(payment.PeriodDays) * 24 * time.Hour)

	if err := h.memberStore.UpdateTier(r.Context(), payment.Pubkey, payment.Tier, newExpiry); err != nil {
		log.Printf("payments: tier upgrade failed for %s: %v", payment.Pubkey[:8], err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	log.Printf("payments: settled! %s upgraded to %s until %s (hash=%s)",
		payment.Pubkey[:8], payment.Tier, newExpiry.Format(time.DateOnly), body.PaymentHash[:8])

	w.WriteHeader(http.StatusOK)
}

// --- NIP-98 user auth (non-admin) ---

const authEventKind = 27235

// validateNIP98UserAuth validates a NIP-98 HTTP Auth event and returns the
// signer's pubkey. Unlike the admin variant in the management package, this
// does not gate on an admin pubkeys list. Any valid NIP-98 event passes.
func validateNIP98UserAuth(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", fmt.Errorf("missing Nostr authorization header")
	}
	if !strings.HasPrefix(authHeader, "Nostr ") {
		return "", fmt.Errorf("invalid authorization format")
	}

	eventJSON, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(authHeader, "Nostr "))
	if err != nil {
		return "", fmt.Errorf("invalid authorization encoding")
	}

	var event nostr.Event
	if err := json.Unmarshal(eventJSON, &event); err != nil {
		return "", fmt.Errorf("invalid auth event")
	}

	if event.Kind != authEventKind {
		return "", fmt.Errorf("wrong event kind for NIP-98 auth")
	}

	eventTime := event.CreatedAt.Time()
	now := time.Now()
	if now.Sub(eventTime) > 60*time.Second || eventTime.Sub(now) > 60*time.Second {
		return "", fmt.Errorf("auth event expired")
	}

	uTag := event.Tags.Find("u")
	if len(uTag) < 2 {
		return "", fmt.Errorf("missing URL tag")
	}
	if uTag[1] != requestURL(r) {
		return "", fmt.Errorf("URL mismatch")
	}

	methodTag := event.Tags.Find("method")
	if len(methodTag) < 2 {
		return "", fmt.Errorf("missing method tag")
	}
	if strings.ToUpper(methodTag[1]) != r.Method {
		return "", fmt.Errorf("method mismatch")
	}

	valid, err := event.CheckSignature()
	if err != nil || !valid {
		return "", fmt.Errorf("invalid signature")
	}

	return event.PubKey, nil
}

func requestURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = fwd
	}
	return scheme + "://" + host + r.URL.Path
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Package zaps implements NIP-57 Lightning Zaps support
//
// NIP-57 defines two event kinds:
// - Kind 9734 (Zap Request): Sent directly to LNURL callback, NOT published to relays
// - Kind 9735 (Zap Receipt): Published to relays by recipient's Lightning wallet
//
// Relay responsibilities:
// - Accept and store kind 9735 zap receipt events
// - Validate zap receipt structure (optional but recommended)
// - Serve zap receipts for efficient client queries
//
// NOTE: Full testing requires an operational Lightning node (lnd).
// This implementation is validated structurally but not integration-tested.
package zaps

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"
)

const (
	// KindZapRequest is the kind for zap requests (not published to relays)
	KindZapRequest = 9734
	// KindZapReceipt is the kind for zap receipts (published to relays)
	KindZapReceipt = 9735
)

// Config holds NIP-57 configuration
type Config struct {
	// Enabled activates NIP-57 zap support
	Enabled bool
	// ValidateReceipts enables structural validation of zap receipts
	ValidateReceipts bool
	// RequireBolt11 requires the bolt11 tag to be present
	RequireBolt11 bool
}

// DefaultConfig returns sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Enabled:          true,
		ValidateReceipts: true,
		RequireBolt11:    false, // Some wallets may not include this
	}
}

// Handler manages NIP-57 zap event handling
type Handler struct {
	config *Config
}

// NewHandler creates a new NIP-57 handler
func NewHandler(cfg *Config) *Handler {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Handler{config: cfg}
}

// ZapReceipt represents a parsed zap receipt (kind 9735)
type ZapReceipt struct {
	// Event is the original nostr event
	Event *nostr.Event
	// RecipientPubkey is the zap recipient (from p tag)
	RecipientPubkey string
	// SenderPubkey is the zap sender (from P tag, if present)
	SenderPubkey string
	// EventID is the zapped event ID (from e tag, if present)
	EventID string
	// Bolt11 is the Lightning invoice (from bolt11 tag)
	Bolt11 string
	// Description is the JSON-encoded zap request (from description tag)
	Description string
	// Preimage is the payment preimage (from preimage tag, if present)
	Preimage string
	// Amount in millisatoshis (parsed from zap request if available)
	AmountMsat int64
}

// ParseZapReceipt parses a kind 9735 event into a ZapReceipt
func ParseZapReceipt(event *nostr.Event) (*ZapReceipt, error) {
	if event.Kind != KindZapReceipt {
		return nil, fmt.Errorf("expected kind %d, got %d", KindZapReceipt, event.Kind)
	}

	zr := &ZapReceipt{Event: event}

	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}
		switch tag[0] {
		case "p":
			zr.RecipientPubkey = tag[1]
		case "P":
			zr.SenderPubkey = tag[1]
		case "e":
			zr.EventID = tag[1]
		case "bolt11":
			zr.Bolt11 = tag[1]
		case "description":
			zr.Description = tag[1]
		case "preimage":
			zr.Preimage = tag[1]
		}
	}

	// Try to parse amount from zap request in description
	if zr.Description != "" {
		zr.AmountMsat = parseAmountFromDescription(zr.Description)
	}

	return zr, nil
}

// parseAmountFromDescription extracts the amount from a zap request JSON
func parseAmountFromDescription(desc string) int64 {
	var zapRequest struct {
		Tags [][]string `json:"tags"`
	}
	if err := json.Unmarshal([]byte(desc), &zapRequest); err != nil {
		return 0
	}

	for _, tag := range zapRequest.Tags {
		if len(tag) >= 2 && tag[0] == "amount" {
			var amount int64
			fmt.Sscanf(tag[1], "%d", &amount)
			return amount
		}
	}
	return 0
}

// Validate checks if a zap receipt has the required structure
func (zr *ZapReceipt) Validate() error {
	if zr.RecipientPubkey == "" {
		return fmt.Errorf("missing recipient pubkey (p tag)")
	}
	if len(zr.RecipientPubkey) != 64 {
		return fmt.Errorf("invalid recipient pubkey length")
	}
	if zr.Description == "" {
		return fmt.Errorf("missing description tag (zap request)")
	}
	// Validate that description is valid JSON
	var zapRequest map[string]interface{}
	if err := json.Unmarshal([]byte(zr.Description), &zapRequest); err != nil {
		return fmt.Errorf("invalid zap request JSON in description: %w", err)
	}
	return nil
}

// RejectInvalidZapReceipt returns a handler that validates zap receipts
func (h *Handler) RejectInvalidZapReceipt() func(context.Context, *nostr.Event) (bool, string) {
	return func(ctx context.Context, event *nostr.Event) (bool, string) {
		// Only validate zap receipts
		if event.Kind != KindZapReceipt {
			return false, ""
		}

		if !h.config.ValidateReceipts {
			return false, ""
		}

		zr, err := ParseZapReceipt(event)
		if err != nil {
			return true, fmt.Sprintf("invalid: %v", err)
		}

		if err := zr.Validate(); err != nil {
			return true, fmt.Sprintf("invalid: %v", err)
		}

		if h.config.RequireBolt11 && zr.Bolt11 == "" {
			return true, "invalid: missing bolt11 tag"
		}

		return false, ""
	}
}

// OnZapReceiptSaved logs zap receipts for monitoring
func (h *Handler) OnZapReceiptSaved() func(context.Context, *nostr.Event) {
	return func(ctx context.Context, event *nostr.Event) {
		if event.Kind != KindZapReceipt {
			return
		}

		zr, err := ParseZapReceipt(event)
		if err != nil {
			return
		}

		recipient := zr.RecipientPubkey
		if len(recipient) > 8 {
			recipient = recipient[:8] + "..."
		}

		sender := "anonymous"
		if zr.SenderPubkey != "" && len(zr.SenderPubkey) > 8 {
			sender = zr.SenderPubkey[:8] + "..."
		}

		amountStr := "unknown"
		if zr.AmountMsat > 0 {
			sats := zr.AmountMsat / 1000
			amountStr = fmt.Sprintf("%d sats", sats)
		}

		log.Printf("NIP-57: Zap receipt stored - %s from %s to %s", amountStr, sender, recipient)
	}
}

// RegisterHandlers registers NIP-57 handlers with the relay
func RegisterHandlers(relay *khatru.Relay, cfg *Config) *Handler {
	handler := NewHandler(cfg)

	// Validate zap receipts on publish
	relay.RejectEvent = append(relay.RejectEvent, handler.RejectInvalidZapReceipt())

	// Log zap receipts
	relay.OnEventSaved = append(relay.OnEventSaved, handler.OnZapReceiptSaved())

	log.Printf("NIP-57 zaps enabled (validate: %v, require bolt11: %v)",
		cfg.ValidateReceipts, cfg.RequireBolt11)

	return handler
}

// GetZapStats represents zap statistics for a pubkey or event
type GetZapStats struct {
	TotalZaps   int   `json:"total_zaps"`
	TotalAmount int64 `json:"total_amount_msat"`
}

// Helper functions for clients

// IsZapReceipt checks if an event is a zap receipt
func IsZapReceipt(event *nostr.Event) bool {
	return event.Kind == KindZapReceipt
}

// IsZapRequest checks if an event is a zap request
func IsZapRequest(event *nostr.Event) bool {
	return event.Kind == KindZapRequest
}

// GetZappedEventID extracts the zapped event ID from a zap receipt
func GetZappedEventID(event *nostr.Event) string {
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "e" {
			return tag[1]
		}
	}
	return ""
}

// GetZapAmount extracts the amount in millisatoshis from a zap receipt
func GetZapAmount(event *nostr.Event) int64 {
	zr, err := ParseZapReceipt(event)
	if err != nil {
		return 0
	}
	return zr.AmountMsat
}

// ExtractLNURLFromProfile extracts the LNURL from a kind 0 profile
func ExtractLNURLFromProfile(content string) string {
	var profile struct {
		Lud06 string `json:"lud06"` // LNURL
		Lud16 string `json:"lud16"` // Lightning Address
	}
	if err := json.Unmarshal([]byte(content), &profile); err != nil {
		return ""
	}
	if profile.Lud06 != "" {
		return profile.Lud06
	}
	if profile.Lud16 != "" {
		// Convert lightning address to LNURL format hint
		// Actual LNURL resolution is client-side
		return "lightning:" + profile.Lud16
	}
	return ""
}

// ValidateBolt11Basic performs basic validation on a bolt11 invoice string
// This is a simple check - full validation requires a Lightning library
func ValidateBolt11Basic(bolt11 string) bool {
	// Bolt11 invoices start with "ln" followed by network prefix
	lower := strings.ToLower(bolt11)
	return strings.HasPrefix(lower, "lnbc") || // mainnet
		strings.HasPrefix(lower, "lntb") || // testnet
		strings.HasPrefix(lower, "lnbcrt") || // regtest
		strings.HasPrefix(lower, "lnsb") // signet
}

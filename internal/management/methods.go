package management

import (
	"encoding/json"
	"fmt"
)

// MethodHandler processes a management method and returns the result
type MethodHandler struct {
	store *Store
}

// NewMethodHandler creates a new method handler
func NewMethodHandler(store *Store) *MethodHandler {
	return &MethodHandler{store: store}
}

// Dispatch routes a method call to its handler
func (h *MethodHandler) Dispatch(method string, params []json.RawMessage) (interface{}, error) {
	switch method {
	case "supportedmethods":
		return h.SupportedMethods()
	case "banpubkey":
		return h.BanPubkey(params)
	case "listbannedpubkeys":
		return h.ListBannedPubkeys(params)
	case "allowpubkey":
		return h.AllowPubkey(params)
	case "listallowedpubkeys":
		return h.ListAllowedPubkeys(params)
	case "listeventsneedingmoderation":
		return h.ListEventsNeedingModeration(params)
	case "allowevent":
		return h.AllowEvent(params)
	case "banevent":
		return h.BanEvent(params)
	case "listbannedevents":
		return h.ListBannedEvents(params)
	case "changerelayname":
		return h.ChangeRelaySetting("relay_name", params)
	case "changerelaydescription":
		return h.ChangeRelaySetting("relay_description", params)
	case "changerelayicon":
		return h.ChangeRelaySetting("relay_icon", params)
	case "allowkind":
		return h.AllowKind(params)
	case "disallowkind":
		return h.DisallowKind(params)
	case "listallowedkinds":
		return h.ListAllowedKinds()
	case "blockip":
		return h.BlockIP(params)
	case "unblockip":
		return h.UnblockIP(params)
	case "listblockedips":
		return h.ListBlockedIPs(params)
	default:
		return nil, fmt.Errorf("unsupported method: %s", method)
	}
}

// SupportedMethods returns the list of supported management methods
func (h *MethodHandler) SupportedMethods() (interface{}, error) {
	return SupportedMethods, nil
}

// BanPubkey bans a pubkey from the relay
func (h *MethodHandler) BanPubkey(params []json.RawMessage) (interface{}, error) {
	if len(params) < 1 {
		return nil, fmt.Errorf("missing pubkey parameter")
	}

	var pubkey string
	if err := json.Unmarshal(params[0], &pubkey); err != nil {
		return nil, fmt.Errorf("invalid pubkey parameter: %w", err)
	}

	reason := ""
	if len(params) > 1 {
		json.Unmarshal(params[1], &reason)
	}

	if err := h.store.BanPubkey(pubkey, reason); err != nil {
		return nil, fmt.Errorf("failed to ban pubkey: %w", err)
	}

	return true, nil
}

// ListBannedPubkeys returns the list of banned pubkeys
func (h *MethodHandler) ListBannedPubkeys(params []json.RawMessage) (interface{}, error) {
	limit, offset := parseListParams(params)
	return h.store.ListBannedPubkeys(limit, offset)
}

// AllowPubkey adds a pubkey to the whitelist
func (h *MethodHandler) AllowPubkey(params []json.RawMessage) (interface{}, error) {
	if len(params) < 1 {
		return nil, fmt.Errorf("missing pubkey parameter")
	}

	var pubkey string
	if err := json.Unmarshal(params[0], &pubkey); err != nil {
		return nil, fmt.Errorf("invalid pubkey parameter: %w", err)
	}

	if err := h.store.AllowPubkey(pubkey); err != nil {
		return nil, fmt.Errorf("failed to allow pubkey: %w", err)
	}

	return true, nil
}

// ListAllowedPubkeys returns the list of allowed pubkeys
func (h *MethodHandler) ListAllowedPubkeys(params []json.RawMessage) (interface{}, error) {
	limit, offset := parseListParams(params)
	return h.store.ListAllowedPubkeys(limit, offset)
}

// ListEventsNeedingModeration returns events in the moderation queue
func (h *MethodHandler) ListEventsNeedingModeration(params []json.RawMessage) (interface{}, error) {
	limit, offset := parseListParams(params)
	return h.store.ListModerationQueue(limit, offset)
}

// AllowEvent approves an event in the moderation queue
func (h *MethodHandler) AllowEvent(params []json.RawMessage) (interface{}, error) {
	if len(params) < 1 {
		return nil, fmt.Errorf("missing event_id parameter")
	}

	var eventID string
	if err := json.Unmarshal(params[0], &eventID); err != nil {
		return nil, fmt.Errorf("invalid event_id parameter: %w", err)
	}

	if err := h.store.UpdateModerationStatus(eventID, "approved"); err != nil {
		return nil, fmt.Errorf("failed to allow event: %w", err)
	}

	return true, nil
}

// BanEvent bans an event by ID
func (h *MethodHandler) BanEvent(params []json.RawMessage) (interface{}, error) {
	if len(params) < 1 {
		return nil, fmt.Errorf("missing event_id parameter")
	}

	var eventID string
	if err := json.Unmarshal(params[0], &eventID); err != nil {
		return nil, fmt.Errorf("invalid event_id parameter: %w", err)
	}

	reason := ""
	if len(params) > 1 {
		json.Unmarshal(params[1], &reason)
	}

	if err := h.store.BanEvent(eventID, reason); err != nil {
		return nil, fmt.Errorf("failed to ban event: %w", err)
	}

	// Also update moderation queue if present
	h.store.UpdateModerationStatus(eventID, "rejected")

	return true, nil
}

// ListBannedEvents returns the list of banned events
func (h *MethodHandler) ListBannedEvents(params []json.RawMessage) (interface{}, error) {
	limit, offset := parseListParams(params)
	return h.store.ListBannedEvents(limit, offset)
}

// ChangeRelaySetting updates a relay setting
func (h *MethodHandler) ChangeRelaySetting(key string, params []json.RawMessage) (interface{}, error) {
	if len(params) < 1 {
		return nil, fmt.Errorf("missing value parameter")
	}

	var value string
	if err := json.Unmarshal(params[0], &value); err != nil {
		return nil, fmt.Errorf("invalid value parameter: %w", err)
	}

	if err := h.store.SetSetting(key, value); err != nil {
		return nil, fmt.Errorf("failed to update setting: %w", err)
	}

	return true, nil
}

// AllowKind adds a kind to the allowed kinds list
func (h *MethodHandler) AllowKind(params []json.RawMessage) (interface{}, error) {
	if len(params) < 1 {
		return nil, fmt.Errorf("missing kind parameter")
	}

	var kind int
	if err := json.Unmarshal(params[0], &kind); err != nil {
		return nil, fmt.Errorf("invalid kind parameter: %w", err)
	}

	if err := h.store.AllowKind(kind); err != nil {
		return nil, fmt.Errorf("failed to allow kind: %w", err)
	}

	return true, nil
}

// DisallowKind removes a kind from the allowed kinds list
func (h *MethodHandler) DisallowKind(params []json.RawMessage) (interface{}, error) {
	if len(params) < 1 {
		return nil, fmt.Errorf("missing kind parameter")
	}

	var kind int
	if err := json.Unmarshal(params[0], &kind); err != nil {
		return nil, fmt.Errorf("invalid kind parameter: %w", err)
	}

	if err := h.store.DisallowKind(kind); err != nil {
		return nil, fmt.Errorf("failed to disallow kind: %w", err)
	}

	return true, nil
}

// ListAllowedKinds returns the list of allowed kinds
func (h *MethodHandler) ListAllowedKinds() (interface{}, error) {
	return h.store.ListAllowedKinds()
}

// BlockIP blocks an IP address
func (h *MethodHandler) BlockIP(params []json.RawMessage) (interface{}, error) {
	if len(params) < 1 {
		return nil, fmt.Errorf("missing ip parameter")
	}

	var ip string
	if err := json.Unmarshal(params[0], &ip); err != nil {
		return nil, fmt.Errorf("invalid ip parameter: %w", err)
	}

	reason := ""
	if len(params) > 1 {
		json.Unmarshal(params[1], &reason)
	}

	if err := h.store.BlockIP(ip, reason); err != nil {
		return nil, fmt.Errorf("failed to block IP: %w", err)
	}

	return true, nil
}

// UnblockIP unblocks an IP address
func (h *MethodHandler) UnblockIP(params []json.RawMessage) (interface{}, error) {
	if len(params) < 1 {
		return nil, fmt.Errorf("missing ip parameter")
	}

	var ip string
	if err := json.Unmarshal(params[0], &ip); err != nil {
		return nil, fmt.Errorf("invalid ip parameter: %w", err)
	}

	if err := h.store.UnblockIP(ip); err != nil {
		return nil, fmt.Errorf("failed to unblock IP: %w", err)
	}

	return true, nil
}

// ListBlockedIPs returns the list of blocked IPs
func (h *MethodHandler) ListBlockedIPs(params []json.RawMessage) (interface{}, error) {
	limit, offset := parseListParams(params)
	return h.store.ListBlockedIPs(limit, offset)
}

// parseListParams extracts limit and offset from params
func parseListParams(params []json.RawMessage) (limit, offset int) {
	limit = 100
	offset = 0

	if len(params) > 0 {
		json.Unmarshal(params[0], &limit)
	}
	if len(params) > 1 {
		json.Unmarshal(params[1], &offset)
	}

	return limit, offset
}

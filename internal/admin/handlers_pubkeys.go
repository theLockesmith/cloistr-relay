package admin

import (
	"fmt"
	"net/http"

	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/management"
)

// PubkeysListData holds data for the pubkeys list partials
type PubkeysListData struct {
	Items      interface{}
	Limit      int
	Offset     int
	HasMore    bool
	TotalCount int
	ListType   string // "banned" or "allowed"
}

func (h *Handler) handlePubkeysPage(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, r, "pubkeys.html", PageData{
		Title:     "Pubkey Management",
		ActiveNav: "pubkeys",
	})
}

func (h *Handler) handleListBannedPubkeys(w http.ResponseWriter, r *http.Request) {
	limit, offset := parseListParams(r)

	pubkeys, err := h.store.ListBannedPubkeys(limit+1, offset) // Get one extra to check for more
	if err != nil {
		h.renderError(w, r, fmt.Sprintf("Failed to list banned pubkeys: %v", err), http.StatusInternalServerError)
		return
	}

	hasMore := len(pubkeys) > limit
	if hasMore {
		pubkeys = pubkeys[:limit]
	}

	// Get total count
	allPubkeys, _ := h.store.ListBannedPubkeys(10000, 0)

	data := PubkeysListData{
		Items:      pubkeys,
		Limit:      limit,
		Offset:     offset,
		HasMore:    hasMore,
		TotalCount: len(allPubkeys),
		ListType:   "banned",
	}

	h.renderPartial(w, "banned_pubkeys_list.html", data)
}

func (h *Handler) handleListAllowedPubkeys(w http.ResponseWriter, r *http.Request) {
	limit, offset := parseListParams(r)

	pubkeys, err := h.store.ListAllowedPubkeys(limit+1, offset)
	if err != nil {
		h.renderError(w, r, fmt.Sprintf("Failed to list allowed pubkeys: %v", err), http.StatusInternalServerError)
		return
	}

	hasMore := len(pubkeys) > limit
	if hasMore {
		pubkeys = pubkeys[:limit]
	}

	allPubkeys, _ := h.store.ListAllowedPubkeys(10000, 0)

	data := PubkeysListData{
		Items:      pubkeys,
		Limit:      limit,
		Offset:     offset,
		HasMore:    hasMore,
		TotalCount: len(allPubkeys),
		ListType:   "allowed",
	}

	h.renderPartial(w, "allowed_pubkeys_list.html", data)
}

func (h *Handler) handleBanPubkey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.renderError(w, r, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, "Invalid form data", http.StatusBadRequest)
		return
	}

	pubkey := normalizeHexInput(r.FormValue("pubkey"))
	reason := r.FormValue("reason")

	if pubkey == "" {
		h.renderError(w, r, "Pubkey is required", http.StatusBadRequest)
		return
	}

	if !isValidHexPubkey(pubkey) {
		h.renderError(w, r, "Invalid pubkey format (must be 64 hex characters)", http.StatusBadRequest)
		return
	}

	if err := h.store.BanPubkey(pubkey, reason); err != nil {
		h.renderError(w, r, fmt.Sprintf("Failed to ban pubkey: %v", err), http.StatusInternalServerError)
		return
	}

	// Return updated list
	w.Header().Set("HX-Trigger", "refreshBannedPubkeys")
	h.renderSuccess(w, fmt.Sprintf("Banned pubkey %s", truncatePubkey(pubkey)))
}

func (h *Handler) handleUnbanPubkey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.renderError(w, r, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, "Invalid form data", http.StatusBadRequest)
		return
	}

	pubkey := normalizeHexInput(r.FormValue("pubkey"))

	if pubkey == "" {
		h.renderError(w, r, "Pubkey is required", http.StatusBadRequest)
		return
	}

	if err := h.store.UnbanPubkey(pubkey); err != nil {
		h.renderError(w, r, fmt.Sprintf("Failed to unban pubkey: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "refreshBannedPubkeys")
	h.renderSuccess(w, fmt.Sprintf("Unbanned pubkey %s", truncatePubkey(pubkey)))
}

func (h *Handler) handleAllowPubkey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.renderError(w, r, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, "Invalid form data", http.StatusBadRequest)
		return
	}

	pubkey := normalizeHexInput(r.FormValue("pubkey"))

	if pubkey == "" {
		h.renderError(w, r, "Pubkey is required", http.StatusBadRequest)
		return
	}

	if !isValidHexPubkey(pubkey) {
		h.renderError(w, r, "Invalid pubkey format (must be 64 hex characters)", http.StatusBadRequest)
		return
	}

	if err := h.store.AllowPubkey(pubkey); err != nil {
		h.renderError(w, r, fmt.Sprintf("Failed to allow pubkey: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "refreshAllowedPubkeys")
	h.renderSuccess(w, fmt.Sprintf("Allowed pubkey %s", truncatePubkey(pubkey)))
}

func (h *Handler) handleDisallowPubkey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.renderError(w, r, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, "Invalid form data", http.StatusBadRequest)
		return
	}

	pubkey := normalizeHexInput(r.FormValue("pubkey"))

	if pubkey == "" {
		h.renderError(w, r, "Pubkey is required", http.StatusBadRequest)
		return
	}

	if err := h.store.RemoveAllowedPubkey(pubkey); err != nil {
		h.renderError(w, r, fmt.Sprintf("Failed to disallow pubkey: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "refreshAllowedPubkeys")
	h.renderSuccess(w, fmt.Sprintf("Removed pubkey %s from allowed list", truncatePubkey(pubkey)))
}

// Helper to format banned pubkeys for templates
// TODO: Wire these helpers into pubkey list templates
var _ = formatBannedPubkeys  // Silence unused warning
var _ = formatAllowedPubkeys // Silence unused warning

func formatBannedPubkeys(pubkeys []management.BannedPubkey) []map[string]interface{} {
	result := make([]map[string]interface{}, len(pubkeys))
	for i, p := range pubkeys {
		result[i] = map[string]interface{}{
			"Pubkey":    p.Pubkey,
			"Display":   truncatePubkey(p.Pubkey),
			"Reason":    p.Reason,
			"CreatedAt": p.CreatedAt,
		}
	}
	return result
}

// Helper to format allowed pubkeys for templates
func formatAllowedPubkeys(pubkeys []management.AllowedPubkey) []map[string]interface{} {
	result := make([]map[string]interface{}, len(pubkeys))
	for i, p := range pubkeys {
		result[i] = map[string]interface{}{
			"Pubkey":    p.Pubkey,
			"Display":   truncatePubkey(p.Pubkey),
			"CreatedAt": p.CreatedAt,
		}
	}
	return result
}

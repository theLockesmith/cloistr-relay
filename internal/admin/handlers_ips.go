package admin

import (
	"fmt"
	"net"
	"net/http"
)

// IPsListData holds data for the IPs list partial
type IPsListData struct {
	Items      interface{}
	Limit      int
	Offset     int
	HasMore    bool
	TotalCount int
}

func (h *Handler) handleIPsPage(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, r, "ips.html", PageData{
		Title:     "IP Management",
		ActiveNav: "ips",
	})
}

func (h *Handler) handleListBlockedIPs(w http.ResponseWriter, r *http.Request) {
	limit, offset := parseListParams(r)

	ips, err := h.store.ListBlockedIPs(limit+1, offset)
	if err != nil {
		h.renderError(w, r, fmt.Sprintf("Failed to list blocked IPs: %v", err), http.StatusInternalServerError)
		return
	}

	hasMore := len(ips) > limit
	if hasMore {
		ips = ips[:limit]
	}

	allIPs, _ := h.store.ListBlockedIPs(10000, 0)

	data := IPsListData{
		Items:      ips,
		Limit:      limit,
		Offset:     offset,
		HasMore:    hasMore,
		TotalCount: len(allIPs),
	}

	h.renderPartial(w, "blocked_ips_list.html", data)
}

func (h *Handler) handleBlockIP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.renderError(w, r, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, "Invalid form data", http.StatusBadRequest)
		return
	}

	ip := r.FormValue("ip")
	reason := r.FormValue("reason")

	if ip == "" {
		h.renderError(w, r, "IP address is required", http.StatusBadRequest)
		return
	}

	// Validate IP address or CIDR
	if !isValidIP(ip) {
		h.renderError(w, r, "Invalid IP address or CIDR format", http.StatusBadRequest)
		return
	}

	if err := h.store.BlockIP(ip, reason); err != nil {
		h.renderError(w, r, fmt.Sprintf("Failed to block IP: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "refreshBlockedIPs")
	h.renderSuccess(w, fmt.Sprintf("Blocked IP %s", ip))
}

func (h *Handler) handleUnblockIP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.renderError(w, r, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, "Invalid form data", http.StatusBadRequest)
		return
	}

	ip := r.FormValue("ip")

	if ip == "" {
		h.renderError(w, r, "IP address is required", http.StatusBadRequest)
		return
	}

	if err := h.store.UnblockIP(ip); err != nil {
		h.renderError(w, r, fmt.Sprintf("Failed to unblock IP: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "refreshBlockedIPs")
	h.renderSuccess(w, fmt.Sprintf("Unblocked IP %s", ip))
}

// isValidIP checks if a string is a valid IP address or CIDR notation
func isValidIP(s string) bool {
	// Try parsing as IP
	if ip := net.ParseIP(s); ip != nil {
		return true
	}

	// Try parsing as CIDR
	if _, _, err := net.ParseCIDR(s); err == nil {
		return true
	}

	return false
}

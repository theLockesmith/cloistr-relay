package admin

import (
	"context"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"

	"gitlab.com/coldforge/coldforge-relay/internal/management"
	"gitlab.com/coldforge/coldforge-relay/web"
)

// contextKey is used for storing values in request context
type contextKey string

const (
	// PubkeyContextKey is the context key for authenticated admin pubkey
	PubkeyContextKey contextKey = "admin_pubkey"
)

// Handler handles admin UI routes
type Handler struct {
	store        *management.Store
	adminPubkeys []string
	templates    *template.Template
}

// NewHandler creates a new admin UI handler
func NewHandler(store *management.Store, adminPubkeys []string) *Handler {
	h := &Handler{
		store:        store,
		adminPubkeys: adminPubkeys,
	}
	h.loadTemplates()
	return h
}

// RegisterRoutes registers all admin UI routes on the given mux
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Static files - serve from embedded web.Static filesystem
	staticFS, err := fs.Sub(web.Static, "static")
	if err != nil {
		log.Printf("Warning: failed to create static sub-filesystem: %v", err)
	} else {
		mux.Handle("/admin/static/", http.StripPrefix("/admin/static/", http.FileServer(http.FS(staticFS))))
	}

	// Dashboard
	mux.HandleFunc("/admin/", h.handleDashboard)
	mux.HandleFunc("/admin", h.handleDashboard)

	// Pubkeys
	mux.HandleFunc("/admin/pubkeys", h.handlePubkeysPage)
	mux.HandleFunc("/admin/pubkeys/banned", h.handleListBannedPubkeys)
	mux.HandleFunc("/admin/pubkeys/allowed", h.handleListAllowedPubkeys)
	mux.HandleFunc("/admin/pubkeys/ban", h.requireAuth(h.handleBanPubkey))
	mux.HandleFunc("/admin/pubkeys/unban", h.requireAuth(h.handleUnbanPubkey))
	mux.HandleFunc("/admin/pubkeys/allow", h.requireAuth(h.handleAllowPubkey))
	mux.HandleFunc("/admin/pubkeys/disallow", h.requireAuth(h.handleDisallowPubkey))

	// Events
	mux.HandleFunc("/admin/events", h.handleEventsPage)
	mux.HandleFunc("/admin/events/banned", h.handleListBannedEvents)
	mux.HandleFunc("/admin/events/ban", h.requireAuth(h.handleBanEvent))
	mux.HandleFunc("/admin/events/unban", h.requireAuth(h.handleUnbanEvent))

	// Moderation
	mux.HandleFunc("/admin/moderation", h.handleModerationPage)
	mux.HandleFunc("/admin/moderation/queue", h.handleListModerationQueue)
	mux.HandleFunc("/admin/moderation/approve", h.requireAuth(h.handleApproveEvent))
	mux.HandleFunc("/admin/moderation/reject", h.requireAuth(h.handleRejectEvent))

	// IPs
	mux.HandleFunc("/admin/ips", h.handleIPsPage)
	mux.HandleFunc("/admin/ips/blocked", h.handleListBlockedIPs)
	mux.HandleFunc("/admin/ips/block", h.requireAuth(h.handleBlockIP))
	mux.HandleFunc("/admin/ips/unblock", h.requireAuth(h.handleUnblockIP))

	// Kinds
	mux.HandleFunc("/admin/kinds", h.handleKindsPage)
	mux.HandleFunc("/admin/kinds/allowed", h.handleListAllowedKinds)
	mux.HandleFunc("/admin/kinds/allow", h.requireAuth(h.handleAllowKind))
	mux.HandleFunc("/admin/kinds/disallow", h.requireAuth(h.handleDisallowKind))

	// Settings
	mux.HandleFunc("/admin/settings", h.handleSettingsPage)
	mux.HandleFunc("/admin/settings/update", h.requireAuth(h.handleUpdateSettings))

	log.Println("Admin UI enabled at /admin/")
}

// requireAuth wraps a handler to require NIP-98 authentication
func (h *Handler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pubkey, err := management.ValidateNIP98Auth(r, h.adminPubkeys)
		if err != nil {
			h.renderError(w, r, "Authentication required: "+err.Error(), http.StatusUnauthorized)
			return
		}
		// Store pubkey in context
		ctx := context.WithValue(r.Context(), PubkeyContextKey, pubkey)
		next(w, r.WithContext(ctx))
	}
}

// parseListParams extracts limit and offset from query params
func parseListParams(r *http.Request) (limit, offset int) {
	limit = 50
	offset = 0

	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 200 {
			limit = v
		}
	}

	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	return limit, offset
}

// isHtmxRequest checks if request is from htmx
func isHtmxRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// truncatePubkey returns a shortened display version of a pubkey
func truncatePubkey(pubkey string) string {
	if len(pubkey) <= 16 {
		return pubkey
	}
	return pubkey[:8] + "..." + pubkey[len(pubkey)-8:]
}

// truncateEventID returns a shortened display version of an event ID
func truncateEventID(id string) string {
	if len(id) <= 16 {
		return id
	}
	return id[:8] + "..." + id[len(id)-8:]
}

// formatPubkey tries to format a pubkey as npub, falls back to truncated hex
func formatPubkey(pubkey string) string {
	// For now, just truncate. Could add bech32 encoding later.
	return truncatePubkey(pubkey)
}

// isValidHexPubkey checks if a string looks like a valid hex pubkey
func isValidHexPubkey(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// isValidHexEventID checks if a string looks like a valid hex event ID
func isValidHexEventID(s string) bool {
	return isValidHexPubkey(s) // Same format
}

// normalizeHexInput normalizes a potential hex input (lowercase)
func normalizeHexInput(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

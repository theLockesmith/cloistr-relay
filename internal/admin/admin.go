package admin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.coldforge.xyz/coldforge/cloistr-relay/internal/haven"
	"git.coldforge.xyz/coldforge/cloistr-relay/internal/management"
	"git.coldforge.xyz/coldforge/cloistr-relay/web"
)

// contextKey is used for storing values in request context
type contextKey string

const (
	// PubkeyContextKey is the context key for authenticated admin pubkey
	PubkeyContextKey contextKey = "admin_pubkey"

	// SessionCookieName is the name of the admin session cookie
	SessionCookieName = "cloistr_admin_session"

	// SessionTTL is how long sessions last
	SessionTTL = 24 * time.Hour
)

// adminSession represents an authenticated admin session
type adminSession struct {
	Token     string
	Pubkey    string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// sessionStore manages admin sessions (in-memory with mutex for thread safety)
type sessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*adminSession
}

var sessions = &sessionStore{
	sessions: make(map[string]*adminSession),
}

// createSession creates a new admin session and returns the token
func (s *sessionStore) createSession(pubkey string) (string, error) {
	// Generate random token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	token := hex.EncodeToString(tokenBytes)

	session := &adminSession{
		Token:     token,
		Pubkey:    pubkey,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(SessionTTL),
	}

	s.mu.Lock()
	s.sessions[token] = session
	s.mu.Unlock()

	return token, nil
}

// getSession retrieves a session by token
func (s *sessionStore) getSession(token string) *adminSession {
	s.mu.RLock()
	session, exists := s.sessions[token]
	s.mu.RUnlock()

	if !exists {
		return nil
	}

	// Check if expired
	if time.Now().After(session.ExpiresAt) {
		s.deleteSession(token)
		return nil
	}

	return session
}

// deleteSession removes a session
func (s *sessionStore) deleteSession(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

// cleanupExpired removes expired sessions (call periodically)
func (s *sessionStore) cleanupExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for token, session := range s.sessions {
		if now.After(session.ExpiresAt) {
			delete(s.sessions, token)
		}
	}
}

// Handler handles admin UI routes
type Handler struct {
	store        *management.Store
	adminPubkeys []string
	pages        map[string]*template.Template
	havenSystem  *haven.HavenSystem
	havenConfig  *haven.Config
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

// SetHavenSystem sets the HAVEN system reference for stats display
func (h *Handler) SetHavenSystem(system *haven.HavenSystem, config *haven.Config) {
	h.havenSystem = system
	h.havenConfig = config
}

// RegisterRoutes registers all admin UI routes on the given mux
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Start session cleanup goroutine
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			sessions.cleanupExpired()
		}
	}()

	// Static files - serve from embedded web.Static filesystem (public)
	staticFS, err := fs.Sub(web.Static, "static")
	if err != nil {
		log.Printf("Warning: failed to create static sub-filesystem: %v", err)
	} else {
		mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	}

	// Login page and verification (public - no auth required)
	mux.HandleFunc("/login", h.handleLoginPage)
	mux.HandleFunc("/login/verify", h.handleLoginVerify)
	mux.HandleFunc("/logout", h.handleLogout)

	// Dashboard (auth required)
	mux.HandleFunc("/", h.requireAuth(h.handleDashboard))

	// Pubkeys (all auth required)
	mux.HandleFunc("/pubkeys", h.requireAuth(h.handlePubkeysPage))
	mux.HandleFunc("/pubkeys/banned", h.requireAuth(h.handleListBannedPubkeys))
	mux.HandleFunc("/pubkeys/allowed", h.requireAuth(h.handleListAllowedPubkeys))
	mux.HandleFunc("/pubkeys/ban", h.requireAuth(h.handleBanPubkey))
	mux.HandleFunc("/pubkeys/unban", h.requireAuth(h.handleUnbanPubkey))
	mux.HandleFunc("/pubkeys/allow", h.requireAuth(h.handleAllowPubkey))
	mux.HandleFunc("/pubkeys/disallow", h.requireAuth(h.handleDisallowPubkey))

	// Events (all auth required)
	mux.HandleFunc("/events", h.requireAuth(h.handleEventsPage))
	mux.HandleFunc("/events/banned", h.requireAuth(h.handleListBannedEvents))
	mux.HandleFunc("/events/ban", h.requireAuth(h.handleBanEvent))
	mux.HandleFunc("/events/unban", h.requireAuth(h.handleUnbanEvent))

	// Moderation (all auth required)
	mux.HandleFunc("/moderation", h.requireAuth(h.handleModerationPage))
	mux.HandleFunc("/moderation/queue", h.requireAuth(h.handleListModerationQueue))
	mux.HandleFunc("/moderation/approve", h.requireAuth(h.handleApproveEvent))
	mux.HandleFunc("/moderation/reject", h.requireAuth(h.handleRejectEvent))

	// IPs (all auth required)
	mux.HandleFunc("/ips", h.requireAuth(h.handleIPsPage))
	mux.HandleFunc("/ips/blocked", h.requireAuth(h.handleListBlockedIPs))
	mux.HandleFunc("/ips/block", h.requireAuth(h.handleBlockIP))
	mux.HandleFunc("/ips/unblock", h.requireAuth(h.handleUnblockIP))

	// Kinds (all auth required)
	mux.HandleFunc("/kinds", h.requireAuth(h.handleKindsPage))
	mux.HandleFunc("/kinds/allowed", h.requireAuth(h.handleListAllowedKinds))
	mux.HandleFunc("/kinds/allow", h.requireAuth(h.handleAllowKind))
	mux.HandleFunc("/kinds/disallow", h.requireAuth(h.handleDisallowKind))

	// Settings (all auth required)
	mux.HandleFunc("/settings", h.requireAuth(h.handleSettingsPage))
	mux.HandleFunc("/settings/update", h.requireAuth(h.handleUpdateSettings))

	// HAVEN (all auth required)
	mux.HandleFunc("/haven", h.requireAuth(h.handleHavenPage))
	mux.HandleFunc("/haven/stats", h.requireAuth(h.handleHavenStats))

	log.Println("Admin UI enabled at /")
}

// requireAuth wraps a handler to require authentication (NIP-98 or session cookie)
func (h *Handler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var pubkey string

		// First, try NIP-98 auth (for HTMX requests with signed headers)
		if nip98Pubkey, err := management.ValidateNIP98Auth(r, h.adminPubkeys); err == nil {
			pubkey = nip98Pubkey
		} else {
			// Try session cookie
			cookie, err := r.Cookie(SessionCookieName)
			if err == nil && cookie.Value != "" {
				session := sessions.getSession(cookie.Value)
				if session != nil && slices.Contains(h.adminPubkeys, session.Pubkey) {
					pubkey = session.Pubkey
				}
			}
		}

		if pubkey == "" {
			// For HTMX requests, return error message
			if isHtmxRequest(r) {
				h.renderError(w, r, "Authentication required", http.StatusUnauthorized)
				return
			}
			// For regular page requests, redirect to login
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Store pubkey in context
		ctx := context.WithValue(r.Context(), PubkeyContextKey, pubkey)
		next(w, r.WithContext(ctx))
	}
}

// handleLoginPage renders the login page
func (h *Handler) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	// If already authenticated via cookie, redirect to dashboard
	if cookie, err := r.Cookie(SessionCookieName); err == nil && cookie.Value != "" {
		if session := sessions.getSession(cookie.Value); session != nil {
			if slices.Contains(h.adminPubkeys, session.Pubkey) {
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
		}
	}

	h.renderPage(w, r, "login.html", PageData{
		Title:     "Login",
		ActiveNav: "",
	})
}

// handleLoginVerify validates NIP-98 auth and creates a session cookie
func (h *Handler) handleLoginVerify(w http.ResponseWriter, r *http.Request) {
	pubkey, err := management.ValidateNIP98Auth(r, h.adminPubkeys)
	if err != nil {
		http.Error(w, "Authentication failed: "+err.Error(), http.StatusUnauthorized)
		return
	}

	// Create session
	token, err := sessions.createSession(pubkey)
	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(SessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
	})

	// Return success JSON
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success": true, "pubkey": "` + pubkey + `"}`))
}

// handleLogout clears the session cookie
func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	// Delete session from store
	if cookie, err := r.Cookie(SessionCookieName); err == nil && cookie.Value != "" {
		sessions.deleteSession(cookie.Value)
	}

	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	// Redirect to login
	http.Redirect(w, r, "/login", http.StatusSeeOther)
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

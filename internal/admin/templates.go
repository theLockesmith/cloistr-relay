package admin

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"git.coldforge.xyz/coldforge/cloistr-relay/web"
)

// PageData is the base data structure for full page renders
type PageData struct {
	Title       string
	ActiveNav   string
	Content     interface{}
	Error       string
	Success     string
	AdminPubkey string
}

// loadTemplates loads and parses templates using per-page cloning.
// Each page gets its own template set so that {{define "content"}} blocks
// don't override each other across different page templates.
func (h *Handler) loadTemplates() {
	funcMap := template.FuncMap{
		"formatTime":      formatTime,
		"formatPubkey":    formatPubkey,
		"truncatePubkey":  truncatePubkey,
		"truncateEventID": truncateEventID,
		"add":             func(a, b int) int { return a + b },
		"sub":             func(a, b int) int { return a - b },
	}

	// Parse shared templates (layout + partials) as a base
	base, err := template.New("").Funcs(funcMap).ParseFS(web.Templates, "templates/layout.html", "templates/partials/*.html")
	if err != nil {
		log.Fatalf("Failed to parse base templates: %v", err)
	}

	// Build per-page template sets by cloning the base and adding each page
	h.pages = make(map[string]*template.Template)
	pageFiles, err := fs.Glob(web.Templates, "templates/*.html")
	if err != nil {
		log.Fatalf("Failed to glob page templates: %v", err)
	}

	for _, f := range pageFiles {
		name := filepath.Base(f)
		if name == "layout.html" {
			continue
		}
		clone, err := template.Must(base.Clone()).ParseFS(web.Templates, f)
		if err != nil {
			log.Fatalf("Failed to parse page template %s: %v", name, err)
		}
		h.pages[name] = clone
	}
}

// renderPage renders a full page with layout
func (h *Handler) renderPage(w http.ResponseWriter, r *http.Request, name string, data PageData) {
	// Get admin pubkey from context if authenticated
	if pubkey, ok := r.Context().Value(PubkeyContextKey).(string); ok {
		data.AdminPubkey = formatPubkey(pubkey)
	}

	tmpl, ok := h.pages[name]
	if !ok {
		log.Printf("Template not found: %s", name)
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("Template error (%s): %v", name, err)
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

// renderPartial renders a partial template (for htmx responses)
func (h *Handler) renderPartial(w http.ResponseWriter, name string, data interface{}) {
	// Partials exist in all page template sets, use any one
	for _, tmpl := range h.pages {
		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
			log.Printf("Template error (%s): %v", name, err)
			http.Error(w, "Template error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = buf.WriteTo(w)
		return
	}
	log.Printf("No template sets available for partial: %s", name)
	http.Error(w, "Template error", http.StatusInternalServerError)
}

// renderError renders an error response (partial for htmx, full page otherwise)
func (h *Handler) renderError(w http.ResponseWriter, r *http.Request, message string, statusCode int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)

	if isHtmxRequest(r) {
		h.renderPartial(w, "toast.html", map[string]interface{}{
			"Type":    "error",
			"Message": message,
		})
	} else {
		h.renderPage(w, r, "error.html", PageData{
			Title: "Error",
			Error: message,
		})
	}
}

// renderSuccess renders a success toast (for htmx responses)
func (h *Handler) renderSuccess(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderPartial(w, "toast.html", map[string]interface{}{
		"Type":    "success",
		"Message": message,
	})
}

// formatTime formats a time for display
func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02 15:04")
}

// Dashboard handler
func (h *Handler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	// Get counts for dashboard
	bannedPubkeys, _ := h.store.ListBannedPubkeys(1, 0)
	allowedPubkeys, _ := h.store.ListAllowedPubkeys(1, 0)
	bannedEvents, _ := h.store.ListBannedEvents(1, 0)
	blockedIPs, _ := h.store.ListBlockedIPs(1, 0)
	allowedKinds, _ := h.store.ListAllowedKinds()
	modQueue, _ := h.store.ListModerationQueue(1, 0)

	// Count totals by querying with large limit
	allBannedPubkeys, _ := h.store.ListBannedPubkeys(10000, 0)
	allAllowedPubkeys, _ := h.store.ListAllowedPubkeys(10000, 0)
	allBannedEvents, _ := h.store.ListBannedEvents(10000, 0)
	allBlockedIPs, _ := h.store.ListBlockedIPs(10000, 0)
	allModQueue, _ := h.store.ListModerationQueue(10000, 0)

	data := struct {
		BannedPubkeysCount  int
		AllowedPubkeysCount int
		BannedEventsCount   int
		BlockedIPsCount     int
		AllowedKindsCount   int
		ModerationCount     int
		HasBannedPubkeys    bool
		HasAllowedPubkeys   bool
		HasBannedEvents     bool
		HasBlockedIPs       bool
		HasAllowedKinds     bool
		HasModeration       bool
	}{
		BannedPubkeysCount:  len(allBannedPubkeys),
		AllowedPubkeysCount: len(allAllowedPubkeys),
		BannedEventsCount:   len(allBannedEvents),
		BlockedIPsCount:     len(allBlockedIPs),
		AllowedKindsCount:   len(allowedKinds),
		ModerationCount:     len(allModQueue),
		HasBannedPubkeys:    len(bannedPubkeys) > 0,
		HasAllowedPubkeys:   len(allowedPubkeys) > 0,
		HasBannedEvents:     len(bannedEvents) > 0,
		HasBlockedIPs:       len(blockedIPs) > 0,
		HasAllowedKinds:     len(allowedKinds) > 0,
		HasModeration:       len(modQueue) > 0,
	}

	h.renderPage(w, r, "index.html", PageData{
		Title:     "Dashboard",
		ActiveNav: "dashboard",
		Content:   data,
	})
}

// Settings handlers

// RelaySettings holds current relay settings for the form
type RelaySettings struct {
	Name        string
	Description string
	Icon        string
}

func (h *Handler) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	name, _ := h.store.GetSetting("relay_name")
	desc, _ := h.store.GetSetting("relay_description")
	icon, _ := h.store.GetSetting("relay_icon")

	settings := RelaySettings{
		Name:        name,
		Description: desc,
		Icon:        icon,
	}

	h.renderPage(w, r, "settings.html", PageData{
		Title:     "Settings",
		ActiveNav: "settings",
		Content:   settings,
	})
}

func (h *Handler) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.renderError(w, r, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, "Invalid form data", http.StatusBadRequest)
		return
	}

	field := r.FormValue("field")
	value := r.FormValue("value")

	var settingKey string
	switch field {
	case "name":
		settingKey = "relay_name"
	case "description":
		settingKey = "relay_description"
	case "icon":
		settingKey = "relay_icon"
	default:
		h.renderError(w, r, "Unknown setting field", http.StatusBadRequest)
		return
	}

	if err := h.store.SetSetting(settingKey, value); err != nil {
		h.renderError(w, r, fmt.Sprintf("Failed to update setting: %v", err), http.StatusInternalServerError)
		return
	}

	h.renderSuccess(w, fmt.Sprintf("Updated %s", field))
}

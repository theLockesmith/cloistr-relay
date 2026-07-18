package management

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/haven"
)

// APIHandler serves NIP-98 authenticated JSON REST endpoints for the admin console.
// These complement the NIP-86 RPC API and cover read-only screens that NIP-86 does not.
type APIHandler struct {
	store        *Store
	havenSystem  *haven.HavenSystem
	havenConfig  *haven.Config
	adminPubkeys []string
}

// NewAPIHandler creates a new admin REST API handler
func NewAPIHandler(store *Store, adminPubkeys []string) *APIHandler {
	return &APIHandler{
		store:        store,
		adminPubkeys: adminPubkeys,
	}
}

// SetHavenSystem injects a live HAVEN system reference for stats endpoints.
// Call before RegisterRoutes. Nil is safe — the haven/stats endpoint returns
// {"enabled": false} when no system is present.
func (a *APIHandler) SetHavenSystem(system *haven.HavenSystem, config *haven.Config) {
	a.havenSystem = system
	a.havenConfig = config
}

// RegisterRoutes mounts all admin REST endpoints on mux.
// Every route requires NIP-98 authentication from an admin pubkey.
func (a *APIHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/admin/events", a.requireNIP98(a.handleEvents))
	mux.HandleFunc("/api/admin/stats", a.requireNIP98(a.handleStats))
	mux.HandleFunc("/api/admin/wot/graph", a.requireNIP98(a.handleWoTGraph))
	mux.HandleFunc("/api/admin/wot/stats", a.requireNIP98(a.handleWoTStats))
	mux.HandleFunc("/api/admin/haven/stats", a.requireNIP98(a.handleHavenStats))
	log.Println("Admin REST API enabled at /api/admin/")
}

// requireNIP98 is middleware that validates a NIP-98 Authorization header and
// confirms the signer is in the admin pubkeys list. Returns 401 on failure.
func (a *APIHandler) requireNIP98(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := ValidateNIP98Auth(r, a.adminPubkeys); err != nil {
			log.Printf("Admin API auth failed (%s %s): %v", r.Method, r.URL.Path, err)
			apiWriteJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "unauthorized: " + err.Error(),
			})
			return
		}
		next(w, r)
	}
}

// apiWriteJSON writes v as JSON with the given HTTP status
func apiWriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// handleEvents handles GET /api/admin/events
//
// Query params:
//
//	pubkey  string  — filter by exact pubkey (hex)
//	kind    int     — filter by event kind
//	search  string  — ILIKE match against content
//	start   string  — ISO date (YYYY-MM-DD) lower bound on created_at
//	end     string  — ISO date (YYYY-MM-DD) upper bound on created_at (end of day)
//	limit   int     — page size, default 50, max 200
//	offset  int     — row offset for pagination, default 0
//
// Response: BrowserResult JSON
func (a *APIHandler) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiWriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	q := r.URL.Query()
	filters := BrowserFilter{
		Pubkey:    strings.TrimSpace(q.Get("pubkey")),
		Kind:      strings.TrimSpace(q.Get("kind")),
		Search:    strings.TrimSpace(q.Get("search")),
		StartDate: strings.TrimSpace(q.Get("start")),
		EndDate:   strings.TrimSpace(q.Get("end")),
	}

	limit := 50
	if l := q.Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	offset := 0
	if o := q.Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	result, err := a.store.QueryBrowserEvents(filters, limit, offset)
	if err != nil {
		apiWriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	apiWriteJSON(w, http.StatusOK, result)
}

// handleStats handles GET /api/admin/stats
//
// Response: StatsResult JSON
func (a *APIHandler) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiWriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	result, err := a.store.QueryStats()
	if err != nil {
		apiWriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	apiWriteJSON(w, http.StatusOK, result)
}

// handleWoTGraph handles GET /api/admin/wot/graph
//
// Response: WoTGraph JSON — {"nodes": [...], "links": [...]}
// Capped at 500 edges from the wot_follows table.
func (a *APIHandler) handleWoTGraph(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiWriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	ownerPubkey := ""
	if a.havenConfig != nil {
		ownerPubkey = a.havenConfig.OwnerPubkey
	}

	graph, err := a.store.QueryWoTGraph(ownerPubkey)
	if err != nil {
		apiWriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	apiWriteJSON(w, http.StatusOK, graph)
}

// handleWoTStats handles GET /api/admin/wot/stats
//
// Response: WoTStats JSON
func (a *APIHandler) handleWoTStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiWriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	ownerPubkey := ""
	if a.havenConfig != nil {
		ownerPubkey = a.havenConfig.OwnerPubkey
	}

	stats, err := a.store.QueryWoTStats(ownerPubkey)
	if err != nil {
		apiWriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	apiWriteJSON(w, http.StatusOK, stats)
}

// HavenStatsResult is the JSON response shape for GET /api/admin/haven/stats
type HavenStatsResult struct {
	Enabled  bool                 `json:"enabled"`
	Blastr   *HavenBlastrResult   `json:"blastr,omitempty"`
	Importer *HavenImporterResult `json:"importer,omitempty"`
}

// HavenBlastrResult holds blastr statistics for JSON serialization
type HavenBlastrResult struct {
	EventsBroadcast int64   `json:"events_broadcast"`
	EventsFailed    int64   `json:"events_failed"`
	RelaysConnected int     `json:"relays_connected"`
	LastBroadcast   *string `json:"last_broadcast,omitempty"` // RFC3339 or absent if never
}

// HavenImporterResult holds importer statistics for JSON serialization
type HavenImporterResult struct {
	EventsImported int64   `json:"events_imported"`
	EventsSkipped  int64   `json:"events_skipped"`
	FetchErrors    int64   `json:"fetch_errors"`
	RelaysPolled   int     `json:"relays_polled"`
	LastImport     *string `json:"last_import,omitempty"`   // RFC3339 or absent if never
	LastPollTime   *string `json:"last_poll_time,omitempty"` // RFC3339 or absent if never
}

// handleHavenStats handles GET /api/admin/haven/stats
//
// Returns {"enabled": false} when the HAVEN system is not running.
// Blastr and importer fields are omitted when their subsystems are absent.
func (a *APIHandler) handleHavenStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiWriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	result := HavenStatsResult{Enabled: false}

	if a.havenSystem == nil {
		apiWriteJSON(w, http.StatusOK, result)
		return
	}

	result.Enabled = true
	stats := a.havenSystem.Stats()

	if raw, ok := stats["blastr"]; ok {
		if bs, ok := raw.(haven.BlastrStats); ok {
			br := &HavenBlastrResult{
				EventsBroadcast: bs.EventsBroadcast,
				EventsFailed:    bs.EventsFailed,
				RelaysConnected: bs.RelaysConnected,
			}
			if !bs.LastBroadcast.IsZero() {
				s := bs.LastBroadcast.UTC().Format(time.RFC3339)
				br.LastBroadcast = &s
			}
			result.Blastr = br
		}
	}

	if raw, ok := stats["importer"]; ok {
		if is, ok := raw.(haven.ImporterStats); ok {
			ir := &HavenImporterResult{
				EventsImported: is.EventsImported,
				EventsSkipped:  is.EventsSkipped,
				FetchErrors:    is.FetchErrors,
				RelaysPolled:   is.RelaysPolled,
			}
			if !is.LastImport.IsZero() {
				s := is.LastImport.UTC().Format(time.RFC3339)
				ir.LastImport = &s
			}
			if !is.LastPollTime.IsZero() {
				s := is.LastPollTime.UTC().Format(time.RFC3339)
				ir.LastPollTime = &s
			}
			result.Importer = ir
		}
	}

	apiWriteJSON(w, http.StatusOK, result)
}

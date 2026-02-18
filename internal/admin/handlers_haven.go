package admin

import (
	"fmt"
	"net/http"
	"time"

	"git.coldforge.xyz/coldforge/cloistr-relay/internal/haven"
)

// HavenPageData holds data for the HAVEN admin page
type HavenPageData struct {
	Enabled         bool
	OwnerPubkey     string
	OwnerDisplay    string
	BlastrEnabled   bool
	BlastrRelays    []string
	ImporterEnabled bool
	ImporterRelays  []string

	// Access policies
	AllowPublicOutboxRead bool
	AllowPublicInboxWrite bool
	RequireAuthForChat    bool
	RequireAuthForPrivate bool

	// Stats (if system is running)
	HasStats       bool
	BlastrStats    *BlastrStatsData
	ImporterStats  *ImporterStatsData
	SystemStats    map[string]interface{}
}

// BlastrStatsData holds Blastr statistics for display
type BlastrStatsData struct {
	EventsBroadcast int64
	EventsFailed    int64
	RelaysConnected int
	LastBroadcast   string
}

// ImporterStatsData holds Importer statistics for display
type ImporterStatsData struct {
	EventsImported int64
	EventsSkipped  int64
	FetchErrors    int64
	RelaysPolled   int
	LastImport     string
	LastPollTime   string
}

func (h *Handler) handleHavenPage(w http.ResponseWriter, r *http.Request) {
	data := h.buildHavenPageData()

	h.renderPage(w, r, "haven.html", PageData{
		Title:     "HAVEN Box Routing",
		ActiveNav: "haven",
		Content:   data,
	})
}

func (h *Handler) handleHavenStats(w http.ResponseWriter, r *http.Request) {
	data := h.buildHavenPageData()

	// For htmx partial refresh, just render the stats section
	h.renderPartial(w, "haven_stats.html", data)
}

func (h *Handler) buildHavenPageData() HavenPageData {
	data := HavenPageData{
		Enabled: false,
	}

	// If no config, HAVEN is disabled
	if h.havenConfig == nil {
		return data
	}

	cfg := h.havenConfig
	data.Enabled = cfg.Enabled
	data.OwnerPubkey = cfg.OwnerPubkey
	data.OwnerDisplay = truncatePubkey(cfg.OwnerPubkey)
	data.BlastrEnabled = cfg.BlastrEnabled
	data.BlastrRelays = cfg.BlastrRelays
	data.ImporterEnabled = cfg.ImporterEnabled
	data.ImporterRelays = cfg.ImporterRelays
	data.AllowPublicOutboxRead = cfg.AllowPublicOutboxRead
	data.AllowPublicInboxWrite = cfg.AllowPublicInboxWrite
	data.RequireAuthForChat = cfg.RequireAuthForChat
	data.RequireAuthForPrivate = cfg.RequireAuthForPrivate

	// Get live stats if system is running
	if h.havenSystem != nil {
		data.HasStats = true
		stats := h.havenSystem.Stats()
		data.SystemStats = stats

		// Extract Blastr stats
		if blastrStats, ok := stats["blastr"]; ok {
			if bs, ok := blastrStats.(haven.BlastrStats); ok {
				data.BlastrStats = &BlastrStatsData{
					EventsBroadcast: bs.EventsBroadcast,
					EventsFailed:    bs.EventsFailed,
					RelaysConnected: bs.RelaysConnected,
					LastBroadcast:   formatTimeSince(bs.LastBroadcast),
				}
			}
		}

		// Extract Importer stats
		if importerStats, ok := stats["importer"]; ok {
			if is, ok := importerStats.(haven.ImporterStats); ok {
				data.ImporterStats = &ImporterStatsData{
					EventsImported: is.EventsImported,
					EventsSkipped:  is.EventsSkipped,
					FetchErrors:    is.FetchErrors,
					RelaysPolled:   is.RelaysPolled,
					LastImport:     formatTimeSince(is.LastImport),
					LastPollTime:   formatTimeSince(is.LastPollTime),
				}
			}
		}
	}

	return data
}

// formatTimeSince formats a time as "X ago" or "-" if zero
func formatTimeSince(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return formatDuration(mins, "minute")
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return formatDuration(hours, "hour")
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day ago"
	}
	return formatDuration(days, "day")
}

func formatDuration(n int, unit string) string {
	if n == 1 {
		return "1 " + unit + " ago"
	}
	return fmt.Sprintf("%d %ss ago", n, unit)
}

package admin

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"
)

// ConnectionStatsData holds data for the connection stats page
type ConnectionStatsData struct {
	// Connection metrics
	TotalConnections  int64
	ActiveConnections int64

	// Event metrics
	EventsReceived int64
	EventsStored   int64
	EventsRejected int64

	// Query metrics
	QueriesTotal    int64
	QueriesRejected int64

	// Database pool stats
	DBOpenConns     int
	DBInUseConns    int
	DBIdleConns     int
	DBWaitCount     int64
	DBWaitDuration  time.Duration
	DBMaxOpenConns  int
	DBMaxIdleConns  int
	DBMaxLifetime   time.Duration
	DBMaxIdleTime   time.Duration

	// Event distribution
	TopKinds []KindCount

	// Database size info
	TotalEvents   int64
	DatabaseSize  string
	OldestEvent   time.Time
	NewestEvent   time.Time

	// Server info
	Uptime      string
	StartTime   time.Time
	LastUpdated time.Time
}

// KindCount represents event count by kind
type KindCount struct {
	Kind  int
	Name  string
	Count int64
}

var serverStartTime = time.Now()

func (h *Handler) handleConnectionStatsPage(w http.ResponseWriter, r *http.Request) {
	data := h.buildConnectionStats()

	h.renderPage(w, r, "stats.html", PageData{
		Title:     "Connection Stats",
		ActiveNav: "stats",
		Content:   data,
	})
}

func (h *Handler) handleConnectionStatsRefresh(w http.ResponseWriter, r *http.Request) {
	data := h.buildConnectionStats()
	h.renderPartial(w, "stats_cards.html", data)
}

func (h *Handler) buildConnectionStats() ConnectionStatsData {
	data := ConnectionStatsData{
		LastUpdated: time.Now(),
		StartTime:   serverStartTime,
		Uptime:      formatUptime(time.Since(serverStartTime)),
	}

	db := h.store.DB()
	if db == nil {
		return data
	}

	// Get database pool stats
	stats := db.Stats()
	data.DBOpenConns = stats.OpenConnections
	data.DBInUseConns = stats.InUse
	data.DBIdleConns = stats.Idle
	data.DBWaitCount = stats.WaitCount
	data.DBWaitDuration = stats.WaitDuration
	data.DBMaxOpenConns = stats.MaxOpenConnections

	// Get total event count
	if err := db.QueryRow("SELECT COUNT(*) FROM event").Scan(&data.TotalEvents); err == nil {
		data.EventsStored = data.TotalEvents
	}

	// Get database size (PostgreSQL)
	var dbSize string
	if err := db.QueryRow("SELECT pg_size_pretty(pg_database_size(current_database()))").Scan(&dbSize); err == nil {
		data.DatabaseSize = dbSize
	}

	// Get oldest and newest events
	var oldest, newest sql.NullInt64
	_ = db.QueryRow("SELECT MIN(created_at) FROM event").Scan(&oldest)
	_ = db.QueryRow("SELECT MAX(created_at) FROM event").Scan(&newest)
	if oldest.Valid {
		data.OldestEvent = time.Unix(oldest.Int64, 0)
	}
	if newest.Valid {
		data.NewestEvent = time.Unix(newest.Int64, 0)
	}

	// Get top kinds
	data.TopKinds = h.getTopKinds(db, 10)

	return data
}

func (h *Handler) getTopKinds(db *sql.DB, limit int) []KindCount {
	rows, err := db.Query(`
		SELECT kind, COUNT(*) as count
		FROM event
		GROUP BY kind
		ORDER BY count DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()

	var kinds []KindCount
	for rows.Next() {
		var kc KindCount
		if err := rows.Scan(&kc.Kind, &kc.Count); err != nil {
			continue
		}
		kc.Name = kindToName(kc.Kind)
		kinds = append(kinds, kc)
	}
	return kinds
}

func kindToName(kind int) string {
	names := map[int]string{
		0:     "Metadata",
		1:     "Text Note",
		3:     "Contacts",
		4:     "DM (NIP-04)",
		5:     "Deletion",
		6:     "Repost",
		7:     "Reaction",
		13:    "Seal",
		14:    "Chat (NIP-17)",
		15:    "File (NIP-17)",
		1059:  "Gift Wrap",
		1060:  "Gift Wrap Alt",
		9735:  "Zap Receipt",
		10002: "Relay List",
		10050: "DM Relay List",
		22242: "Auth",
		24133: "NIP-46",
		30023: "Long-form",
	}

	if name, ok := names[kind]; ok {
		return name
	}

	// Handle ranges
	if kind >= 10000 && kind < 20000 {
		return fmt.Sprintf("Replaceable (%d)", kind)
	}
	if kind >= 20000 && kind < 30000 {
		return fmt.Sprintf("Ephemeral (%d)", kind)
	}
	if kind >= 30000 && kind < 40000 {
		return fmt.Sprintf("Param. Repl. (%d)", kind)
	}
	if kind >= 9000 && kind < 9100 {
		return fmt.Sprintf("Group Admin (%d)", kind)
	}
	if kind >= 39000 && kind < 40000 {
		return fmt.Sprintf("Group Meta (%d)", kind)
	}

	return fmt.Sprintf("Kind %d", kind)
}

func formatUptime(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}

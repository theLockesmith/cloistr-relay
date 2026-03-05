package admin

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// EventBrowserData holds data for the event browser page
type EventBrowserData struct {
	Events     []BrowserEvent
	Filters    EventFilters
	Limit      int
	Offset     int
	HasMore    bool
	TotalCount int
}

// BrowserEvent represents an event for display in the browser
type BrowserEvent struct {
	ID        string
	Pubkey    string
	Kind      int
	Content   string
	CreatedAt time.Time
	Tags      string
	IsBanned  bool
}

// EventFilters holds the filter form values
type EventFilters struct {
	Pubkey    string
	Kind      string
	Search    string
	StartDate string
	EndDate   string
}

func (h *Handler) handleEventBrowserPage(w http.ResponseWriter, r *http.Request) {
	filters := parseEventFilters(r)
	limit, offset := parseListParams(r)

	events, total, err := h.queryBrowserEvents(filters, limit+1, offset)
	if err != nil {
		h.renderPage(w, r, "browser.html", PageData{
			Title:     "Event Browser",
			ActiveNav: "browser",
			Error:     fmt.Sprintf("Failed to query events: %v", err),
		})
		return
	}

	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}

	data := EventBrowserData{
		Events:     events,
		Filters:    filters,
		Limit:      limit,
		Offset:     offset,
		HasMore:    hasMore,
		TotalCount: total,
	}

	h.renderPage(w, r, "browser.html", PageData{
		Title:     "Event Browser",
		ActiveNav: "browser",
		Content:   data,
	})
}

func (h *Handler) handleEventBrowserList(w http.ResponseWriter, r *http.Request) {
	filters := parseEventFilters(r)
	limit, offset := parseListParams(r)

	events, total, err := h.queryBrowserEvents(filters, limit+1, offset)
	if err != nil {
		h.renderError(w, r, fmt.Sprintf("Failed to query events: %v", err), http.StatusInternalServerError)
		return
	}

	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}

	data := EventBrowserData{
		Events:     events,
		Filters:    filters,
		Limit:      limit,
		Offset:     offset,
		HasMore:    hasMore,
		TotalCount: total,
	}

	h.renderPartial(w, "browser_list.html", data)
}

func (h *Handler) handleEventDetail(w http.ResponseWriter, r *http.Request) {
	eventID := r.URL.Query().Get("id")
	if eventID == "" {
		h.renderError(w, r, "Event ID required", http.StatusBadRequest)
		return
	}

	event, err := h.getEventByID(eventID)
	if err != nil {
		h.renderError(w, r, fmt.Sprintf("Event not found: %v", err), http.StatusNotFound)
		return
	}

	// Return JSON for modal display
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(event)
}

func parseEventFilters(r *http.Request) EventFilters {
	return EventFilters{
		Pubkey:    strings.TrimSpace(r.URL.Query().Get("pubkey")),
		Kind:      strings.TrimSpace(r.URL.Query().Get("kind")),
		Search:    strings.TrimSpace(r.URL.Query().Get("search")),
		StartDate: strings.TrimSpace(r.URL.Query().Get("start")),
		EndDate:   strings.TrimSpace(r.URL.Query().Get("end")),
	}
}

func (h *Handler) queryBrowserEvents(filters EventFilters, limit, offset int) ([]BrowserEvent, int, error) {
	db := h.store.DB()
	if db == nil {
		return nil, 0, fmt.Errorf("database not available")
	}

	// Build query with filters
	where := []string{"1=1"}
	args := []interface{}{}
	argNum := 1

	if filters.Pubkey != "" {
		where = append(where, fmt.Sprintf("pubkey = $%d", argNum))
		args = append(args, strings.ToLower(filters.Pubkey))
		argNum++
	}

	if filters.Kind != "" {
		if kind, err := strconv.Atoi(filters.Kind); err == nil {
			where = append(where, fmt.Sprintf("kind = $%d", argNum))
			args = append(args, kind)
			argNum++
		}
	}

	if filters.Search != "" {
		where = append(where, fmt.Sprintf("content ILIKE $%d", argNum))
		args = append(args, "%"+filters.Search+"%")
		argNum++
	}

	if filters.StartDate != "" {
		if t, err := time.Parse("2006-01-02", filters.StartDate); err == nil {
			where = append(where, fmt.Sprintf("created_at >= $%d", argNum))
			args = append(args, t.Unix())
			argNum++
		}
	}

	if filters.EndDate != "" {
		if t, err := time.Parse("2006-01-02", filters.EndDate); err == nil {
			// End of day
			t = t.Add(24*time.Hour - time.Second)
			where = append(where, fmt.Sprintf("created_at <= $%d", argNum))
			args = append(args, t.Unix())
			argNum++
		}
	}

	whereClause := strings.Join(where, " AND ")

	// Count total
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM event WHERE %s", whereClause)
	if err := db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count query failed: %w", err)
	}

	// Get events with ban status in a single query using LEFT JOIN
	query := fmt.Sprintf(`
		SELECT e.id, e.pubkey, e.kind, e.content, e.created_at, e.tagvalues,
		       EXISTS(SELECT 1 FROM management_banned_events b WHERE b.event_id = e.id) as is_banned
		FROM event e
		WHERE %s
		ORDER BY e.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argNum, argNum+1)

	args = append(args, limit, offset)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var events []BrowserEvent
	for rows.Next() {
		var e BrowserEvent
		var createdAt int64
		var tags sql.NullString

		if err := rows.Scan(&e.ID, &e.Pubkey, &e.Kind, &e.Content, &createdAt, &tags, &e.IsBanned); err != nil {
			continue
		}

		e.CreatedAt = time.Unix(createdAt, 0)

		// Truncate content for display
		if len(e.Content) > 200 {
			e.Content = e.Content[:200] + "..."
		}

		if tags.Valid {
			e.Tags = tags.String
		}

		events = append(events, e)
	}

	return events, total, nil
}

func (h *Handler) getEventByID(id string) (map[string]interface{}, error) {
	db := h.store.DB()
	if db == nil {
		return nil, fmt.Errorf("database not available")
	}

	var event struct {
		ID        string
		Pubkey    string
		Kind      int
		Content   string
		CreatedAt int64
		Sig       string
		Tags      sql.NullString
	}

	err := db.QueryRow(`
		SELECT id, pubkey, kind, content, created_at, sig, tagvalues
		FROM event WHERE id = $1
	`, id).Scan(&event.ID, &event.Pubkey, &event.Kind, &event.Content, &event.CreatedAt, &event.Sig, &event.Tags)

	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"id":         event.ID,
		"pubkey":     event.Pubkey,
		"kind":       event.Kind,
		"content":    event.Content,
		"created_at": event.CreatedAt,
		"sig":        event.Sig,
	}

	if event.Tags.Valid {
		result["tags"] = event.Tags.String
	}

	return result, nil
}

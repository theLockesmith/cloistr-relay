package admin

import (
	"fmt"
	"net/http"
)

// EventsListData holds data for the events list partials
type EventsListData struct {
	Items      interface{}
	Limit      int
	Offset     int
	HasMore    bool
	TotalCount int
}

func (h *Handler) handleEventsPage(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, r, "events.html", PageData{
		Title:     "Event Management",
		ActiveNav: "events",
	})
}

func (h *Handler) handleListBannedEvents(w http.ResponseWriter, r *http.Request) {
	limit, offset := parseListParams(r)

	events, err := h.store.ListBannedEvents(limit+1, offset)
	if err != nil {
		h.renderError(w, r, fmt.Sprintf("Failed to list banned events: %v", err), http.StatusInternalServerError)
		return
	}

	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}

	allEvents, _ := h.store.ListBannedEvents(10000, 0)

	data := EventsListData{
		Items:      events,
		Limit:      limit,
		Offset:     offset,
		HasMore:    hasMore,
		TotalCount: len(allEvents),
	}

	h.renderPartial(w, "banned_events_list.html", data)
}

func (h *Handler) handleBanEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.renderError(w, r, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, "Invalid form data", http.StatusBadRequest)
		return
	}

	eventID := normalizeHexInput(r.FormValue("event_id"))
	reason := r.FormValue("reason")

	if eventID == "" {
		h.renderError(w, r, "Event ID is required", http.StatusBadRequest)
		return
	}

	if !isValidHexEventID(eventID) {
		h.renderError(w, r, "Invalid event ID format (must be 64 hex characters)", http.StatusBadRequest)
		return
	}

	if err := h.store.BanEvent(eventID, reason); err != nil {
		h.renderError(w, r, fmt.Sprintf("Failed to ban event: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "refreshBannedEvents")
	h.renderSuccess(w, fmt.Sprintf("Banned event %s", truncateEventID(eventID)))
}

func (h *Handler) handleUnbanEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.renderError(w, r, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, "Invalid form data", http.StatusBadRequest)
		return
	}

	eventID := normalizeHexInput(r.FormValue("event_id"))

	if eventID == "" {
		h.renderError(w, r, "Event ID is required", http.StatusBadRequest)
		return
	}

	if err := h.store.UnbanEvent(eventID); err != nil {
		h.renderError(w, r, fmt.Sprintf("Failed to unban event: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "refreshBannedEvents")
	h.renderSuccess(w, fmt.Sprintf("Unbanned event %s", truncateEventID(eventID)))
}

// Moderation handlers

func (h *Handler) handleModerationPage(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, r, "moderation.html", PageData{
		Title:     "Moderation Queue",
		ActiveNav: "moderation",
	})
}

func (h *Handler) handleListModerationQueue(w http.ResponseWriter, r *http.Request) {
	limit, offset := parseListParams(r)

	items, err := h.store.ListModerationQueue(limit+1, offset)
	if err != nil {
		h.renderError(w, r, fmt.Sprintf("Failed to list moderation queue: %v", err), http.StatusInternalServerError)
		return
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	allItems, _ := h.store.ListModerationQueue(10000, 0)

	data := EventsListData{
		Items:      items,
		Limit:      limit,
		Offset:     offset,
		HasMore:    hasMore,
		TotalCount: len(allItems),
	}

	h.renderPartial(w, "moderation_queue.html", data)
}

func (h *Handler) handleApproveEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.renderError(w, r, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, "Invalid form data", http.StatusBadRequest)
		return
	}

	eventID := normalizeHexInput(r.FormValue("event_id"))

	if eventID == "" {
		h.renderError(w, r, "Event ID is required", http.StatusBadRequest)
		return
	}

	if err := h.store.UpdateModerationStatus(eventID, "approved"); err != nil {
		h.renderError(w, r, fmt.Sprintf("Failed to approve event: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "refreshModerationQueue")
	h.renderSuccess(w, fmt.Sprintf("Approved event %s", truncateEventID(eventID)))
}

func (h *Handler) handleRejectEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.renderError(w, r, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, "Invalid form data", http.StatusBadRequest)
		return
	}

	eventID := normalizeHexInput(r.FormValue("event_id"))
	reason := r.FormValue("reason")

	if eventID == "" {
		h.renderError(w, r, "Event ID is required", http.StatusBadRequest)
		return
	}

	// Update moderation status to rejected
	if err := h.store.UpdateModerationStatus(eventID, "rejected"); err != nil {
		h.renderError(w, r, fmt.Sprintf("Failed to reject event: %v", err), http.StatusInternalServerError)
		return
	}

	// Also ban the event
	if err := h.store.BanEvent(eventID, reason); err != nil {
		// Log but don't fail - moderation status was updated
		fmt.Printf("Warning: failed to ban rejected event: %v\n", err)
	}

	w.Header().Set("HX-Trigger", "refreshModerationQueue")
	h.renderSuccess(w, fmt.Sprintf("Rejected event %s", truncateEventID(eventID)))
}

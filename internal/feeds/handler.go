package feeds

import (
	"context"
	"fmt"
	"html"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

// EventQuerier is an interface for querying events from the database
type EventQuerier interface {
	QueryEvents(ctx context.Context, filter nostr.Filter) (chan *nostr.Event, error)
}

// Handler handles RSS/Atom feed HTTP requests
type Handler struct {
	config  *Config
	querier EventQuerier
}

// NewHandler creates a new feed handler
func NewHandler(cfg *Config, querier EventQuerier) *Handler {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	// Ensure safe defaults to prevent fetching all events
	if cfg.DefaultLimit <= 0 {
		cfg.DefaultLimit = 20
	}
	if cfg.MaxLimit <= 0 {
		cfg.MaxLimit = 100
	}
	return &Handler{
		config:  cfg,
		querier: querier,
	}
}

// RegisterRoutes registers feed routes on the provided mux
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/feed/rss", h.handleRSS)
	mux.HandleFunc("/feed/atom", h.handleAtom)
	mux.HandleFunc("/feed/rss.xml", h.handleRSS)
	mux.HandleFunc("/feed/atom.xml", h.handleAtom)
}

// handleRSS serves the RSS 2.0 feed
func (h *Handler) handleRSS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	items, err := h.fetchFeedItems(r)
	if err != nil {
		log.Printf("Feed error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	meta := h.buildMetadata("rss")
	if len(items) > 0 {
		meta.Updated = items[0].Published
	}

	rssData, err := GenerateRSS(meta, items)
	if err != nil {
		log.Printf("RSS generation error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	h.setCacheHeaders(w)
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	_, _ = w.Write(rssData)
}

// handleAtom serves the Atom 1.0 feed
func (h *Handler) handleAtom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	items, err := h.fetchFeedItems(r)
	if err != nil {
		log.Printf("Feed error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	meta := h.buildMetadata("atom")
	if len(items) > 0 {
		meta.Updated = items[0].Published
	}

	atomData, err := GenerateAtom(meta, items)
	if err != nil {
		log.Printf("Atom generation error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	h.setCacheHeaders(w)
	w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
	_, _ = w.Write(atomData)
}

// fetchFeedItems queries events and converts them to feed items
func (h *Handler) fetchFeedItems(r *http.Request) ([]FeedItem, error) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Parse limit from query string
	limit := h.config.DefaultLimit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
			if limit > h.config.MaxLimit {
				limit = h.config.MaxLimit
			}
		}
	}

	// Build filter for notes (kind 1) from owner
	kinds := []int{1} // Short text notes
	if h.config.IncludeLongForm {
		kinds = append(kinds, 30023) // Long-form articles
	}

	filter := nostr.Filter{
		Authors: []string{h.config.OwnerPubkey},
		Kinds:   kinds,
		Limit:   limit,
	}

	// Query events
	eventCh, err := h.querier.QueryEvents(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}

	// Collect events
	var events []*nostr.Event
	for event := range eventCh {
		// Skip replies if configured
		if !h.config.IncludeReplies && hasReplyTag(event) {
			continue
		}
		events = append(events, event)
	}

	// Convert to feed items
	items := make([]FeedItem, 0, len(events))
	for _, event := range events {
		item := h.eventToFeedItem(event)
		items = append(items, item)
	}

	return items, nil
}

// eventToFeedItem converts a Nostr event to a FeedItem
func (h *Handler) eventToFeedItem(event *nostr.Event) FeedItem {
	// Extract title (first line or truncated content)
	title := extractTitle(event.Content, 80)

	// Generate item link (njump.me for now, could be configurable)
	link := fmt.Sprintf("https://njump.me/%s", event.ID)

	// Try to generate nevent link
	if nevent, err := nip19.EncodeEvent(event.ID, []string{h.config.RelayURL}, event.PubKey); err == nil {
		link = fmt.Sprintf("https://njump.me/%s", nevent)
	}

	// Get author display name (npub truncated)
	author := h.formatAuthor(event.PubKey)

	// Extract hashtags
	tags := extractHashtags(event)

	// Format content as HTML
	contentHTML := formatContentAsHTML(event.Content)

	// For long-form articles (kind 30023), extract title from tags
	if event.Kind == 30023 {
		if titleTag := getTagValue(event, "title"); titleTag != "" {
			title = titleTag
		}
	}

	return FeedItem{
		ID:           event.ID,
		Title:        title,
		Content:      event.Content,
		ContentHTML:  contentHTML,
		Link:         link,
		Author:       author,
		AuthorPubkey: event.PubKey,
		Published:    time.Unix(int64(event.CreatedAt), 0),
		Updated:      time.Unix(int64(event.CreatedAt), 0),
		Kind:         event.Kind,
		Tags:         tags,
	}
}

// buildMetadata creates feed metadata
func (h *Handler) buildMetadata(feedType string) FeedMetadata {
	// Strip trailing slash from RelayURL to avoid double-slash URLs
	baseURL := strings.TrimRight(h.config.RelayURL, "/")
	var feedLink string
	if feedType == "rss" {
		feedLink = baseURL + "/feed/rss"
	} else {
		feedLink = baseURL + "/feed/atom"
	}

	author := h.formatAuthor(h.config.OwnerPubkey)

	return FeedMetadata{
		Title:        h.config.RelayName + " - Nostr Feed",
		Description:  fmt.Sprintf("Posts from %s via %s", author, h.config.RelayName),
		Link:         h.config.RelayURL,
		FeedLink:     feedLink,
		Author:       author,
		AuthorPubkey: h.config.OwnerPubkey,
		Updated:      time.Now(),
		Generator:    "cloistr-relay",
		Language:     "en",
	}
}

// formatAuthor formats an author pubkey for display
func (h *Handler) formatAuthor(pubkey string) string {
	npub, err := nip19.EncodePublicKey(pubkey)
	if err != nil {
		// Guard against short pubkeys
		if len(pubkey) < 8 {
			return pubkey
		}
		return pubkey[:8] + "..."
	}
	// Guard against short npub (shouldn't happen but be safe)
	if len(npub) < 16 {
		return npub
	}
	// Return truncated npub
	return npub[:12] + "..." + npub[len(npub)-4:]
}

// setCacheHeaders sets appropriate cache headers for feed responses
func (h *Handler) setCacheHeaders(w http.ResponseWriter) {
	ttl := h.config.CacheTTL
	w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(ttl.Seconds())))
	w.Header().Set("Vary", "Accept")
}

// hasReplyTag checks if an event is a reply (has e-tag with reply marker)
func hasReplyTag(event *nostr.Event) bool {
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "e" {
			// Any e-tag indicates it references another event
			return true
		}
	}
	return false
}

// extractHashtags extracts hashtags from event tags
func extractHashtags(event *nostr.Event) []string {
	var tags []string
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "t" {
			tags = append(tags, tag[1])
		}
	}
	return tags
}

// getTagValue gets the first value of a tag by name
func getTagValue(event *nostr.Event, name string) string {
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == name {
			return tag[1]
		}
	}
	return ""
}

// formatContentAsHTML converts Nostr content to HTML safely
func formatContentAsHTML(content string) string {
	// Process content: linkify URLs first on raw content, then escape non-URL parts
	result := linkifyURLsSafe(content)

	// Convert newlines to <br>
	result = strings.ReplaceAll(result, "\n", "<br>\n")

	return "<p>" + result + "</p>"
}

// linkifyURLsSafe converts URLs in text to HTML links while properly escaping all content
func linkifyURLsSafe(text string) string {
	var result strings.Builder
	words := strings.Split(text, " ")

	for i, word := range words {
		if i > 0 {
			result.WriteString(" ")
		}

		// Check if word contains a URL
		cleanWord := strings.TrimRight(word, ".,;:!?)")
		suffix := word[len(cleanWord):]

		if strings.HasPrefix(cleanWord, "http://") || strings.HasPrefix(cleanWord, "https://") {
			// It's a URL - escape the URL for href and display, then construct the link
			escapedURL := html.EscapeString(cleanWord)
			escapedSuffix := html.EscapeString(suffix)
			result.WriteString(fmt.Sprintf(`<a href="%s">%s</a>%s`, escapedURL, escapedURL, escapedSuffix))
		} else {
			// Not a URL - just escape the content
			result.WriteString(html.EscapeString(word))
		}
	}
	return result.String()
}

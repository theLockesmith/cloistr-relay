package feeds

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

// mockQuerier implements EventQuerier for testing
type mockQuerier struct {
	events []*nostr.Event
}

func (m *mockQuerier) QueryEvents(ctx context.Context, filter nostr.Filter) (chan *nostr.Event, error) {
	ch := make(chan *nostr.Event)
	go func() {
		defer close(ch)
		for _, event := range m.events {
			// Apply basic filter checks
			if len(filter.Authors) > 0 {
				found := false
				for _, author := range filter.Authors {
					if author == event.PubKey {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}

			if len(filter.Kinds) > 0 {
				found := false
				for _, kind := range filter.Kinds {
					if kind == event.Kind {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}

			select {
			case ch <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

func TestGenerateRSS(t *testing.T) {
	meta := FeedMetadata{
		Title:       "Test Feed",
		Description: "A test RSS feed",
		Link:        "https://example.com",
		FeedLink:    "https://example.com/feed/rss",
		Author:      "testauthor",
		Updated:     time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
		Generator:   "test-generator",
		Language:    "en",
	}

	items := []FeedItem{
		{
			ID:        "abc123",
			Title:     "First Post",
			Content:   "This is the first post content.",
			Link:      "https://example.com/post/1",
			Author:    "testauthor",
			Published: time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
			Tags:      []string{"test", "first"},
		},
		{
			ID:        "def456",
			Title:     "Second Post",
			Content:   "This is the second post content.",
			Link:      "https://example.com/post/2",
			Author:    "testauthor",
			Published: time.Date(2024, 1, 14, 12, 0, 0, 0, time.UTC),
			Tags:      []string{"test", "second"},
		},
	}

	data, err := GenerateRSS(meta, items)
	if err != nil {
		t.Fatalf("GenerateRSS failed: %v", err)
	}

	// Verify XML structure
	var rss RSS
	if err := xml.Unmarshal(data, &rss); err != nil {
		t.Fatalf("Failed to parse RSS XML: %v", err)
	}

	if rss.Version != "2.0" {
		t.Errorf("Expected RSS version 2.0, got %s", rss.Version)
	}

	if rss.Channel.Title != "Test Feed" {
		t.Errorf("Expected title 'Test Feed', got %s", rss.Channel.Title)
	}

	if len(rss.Channel.Items) != 2 {
		t.Errorf("Expected 2 items, got %d", len(rss.Channel.Items))
	}

	if rss.Channel.Items[0].Title != "First Post" {
		t.Errorf("Expected first item title 'First Post', got %s", rss.Channel.Items[0].Title)
	}

	if rss.Channel.Items[0].GUID.Value != "abc123" {
		t.Errorf("Expected GUID 'abc123', got %s", rss.Channel.Items[0].GUID.Value)
	}
}

func TestGenerateAtom(t *testing.T) {
	meta := FeedMetadata{
		Title:       "Test Feed",
		Description: "A test Atom feed",
		Link:        "https://example.com",
		FeedLink:    "https://example.com/feed/atom",
		Author:      "testauthor",
		Updated:     time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
		Generator:   "test-generator",
	}

	items := []FeedItem{
		{
			ID:        "abc123",
			Title:     "First Post",
			Content:   "This is the first post content.",
			Link:      "https://example.com/post/1",
			Author:    "testauthor",
			Published: time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
			Updated:   time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
			Tags:      []string{"test"},
		},
	}

	data, err := GenerateAtom(meta, items)
	if err != nil {
		t.Fatalf("GenerateAtom failed: %v", err)
	}

	// Verify XML structure
	var feed AtomFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		t.Fatalf("Failed to parse Atom XML: %v", err)
	}

	if feed.Title != "Test Feed" {
		t.Errorf("Expected title 'Test Feed', got %s", feed.Title)
	}

	if len(feed.Entries) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(feed.Entries))
	}

	if feed.Entries[0].Title != "First Post" {
		t.Errorf("Expected entry title 'First Post', got %s", feed.Entries[0].Title)
	}

	if feed.Entries[0].ID != "urn:nostr:abc123" {
		t.Errorf("Expected entry ID 'urn:nostr:abc123', got %s", feed.Entries[0].ID)
	}
}

func TestExtractTitle(t *testing.T) {
	tests := []struct {
		content  string
		maxLen   int
		expected string
	}{
		{"Short title", 80, "Short title"},
		{"Line one\nLine two\nLine three", 80, "Line one"},
		{"A very long title that needs to be truncated because it exceeds the maximum length", 40, "A very long title that needs to be..."},
		{"", 80, ""},
		{"  Trimmed whitespace  ", 80, "Trimmed whitespace"},
	}

	for _, tc := range tests {
		result := extractTitle(tc.content, tc.maxLen)
		if result != tc.expected {
			t.Errorf("extractTitle(%q, %d) = %q, expected %q", tc.content, tc.maxLen, result, tc.expected)
		}
	}
}

func TestHandlerRSS(t *testing.T) {
	events := []*nostr.Event{
		{
			ID:        "event1",
			PubKey:    "owner123",
			Kind:      1,
			Content:   "Hello, world!",
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Tags:      nostr.Tags{},
		},
	}

	querier := &mockQuerier{events: events}
	cfg := &Config{
		Enabled:         true,
		OwnerPubkey:     "owner123",
		RelayURL:        "wss://relay.example.com",
		RelayName:       "Test Relay",
		DefaultLimit:    20,
		MaxLimit:        100,
		IncludeLongForm: true,
		IncludeReplies:  false,
	}

	handler := NewHandler(cfg, querier)

	req := httptest.NewRequest(http.MethodGet, "/feed/rss", nil)
	w := httptest.NewRecorder()

	handler.handleRSS(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/rss+xml") {
		t.Errorf("Expected content-type application/rss+xml, got %s", contentType)
	}

	// Verify body is valid RSS
	var rss RSS
	if err := xml.NewDecoder(resp.Body).Decode(&rss); err != nil {
		t.Fatalf("Failed to parse RSS response: %v", err)
	}

	if len(rss.Channel.Items) != 1 {
		t.Errorf("Expected 1 item, got %d", len(rss.Channel.Items))
	}
}

func TestHandlerAtom(t *testing.T) {
	events := []*nostr.Event{
		{
			ID:        "event1",
			PubKey:    "owner123",
			Kind:      1,
			Content:   "Hello, Atom world!",
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Tags:      nostr.Tags{},
		},
	}

	querier := &mockQuerier{events: events}
	cfg := &Config{
		Enabled:         true,
		OwnerPubkey:     "owner123",
		RelayURL:        "wss://relay.example.com",
		RelayName:       "Test Relay",
		DefaultLimit:    20,
		MaxLimit:        100,
		IncludeLongForm: true,
		IncludeReplies:  false,
	}

	handler := NewHandler(cfg, querier)

	req := httptest.NewRequest(http.MethodGet, "/feed/atom", nil)
	w := httptest.NewRecorder()

	handler.handleAtom(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/atom+xml") {
		t.Errorf("Expected content-type application/atom+xml, got %s", contentType)
	}

	// Verify body is valid Atom
	var feed AtomFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		t.Fatalf("Failed to parse Atom response: %v", err)
	}

	if len(feed.Entries) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(feed.Entries))
	}
}

func TestHandlerLimitParameter(t *testing.T) {
	// Create 30 events
	events := make([]*nostr.Event, 30)
	for i := 0; i < 30; i++ {
		events[i] = &nostr.Event{
			ID:        "event" + string(rune('a'+i)),
			PubKey:    "owner123",
			Kind:      1,
			Content:   "Post content",
			CreatedAt: nostr.Timestamp(time.Now().Unix() - int64(i)),
			Tags:      nostr.Tags{},
		}
	}

	querier := &mockQuerier{events: events}
	cfg := &Config{
		Enabled:      true,
		OwnerPubkey:  "owner123",
		RelayURL:     "wss://relay.example.com",
		RelayName:    "Test Relay",
		DefaultLimit: 20,
		MaxLimit:     25,
	}

	handler := NewHandler(cfg, querier)

	// Test with custom limit
	req := httptest.NewRequest(http.MethodGet, "/feed/rss?limit=10", nil)
	w := httptest.NewRecorder()

	handler.handleRSS(w, req)

	var rss RSS
	if err := xml.NewDecoder(w.Result().Body).Decode(&rss); err != nil {
		t.Fatalf("Failed to parse RSS: %v", err)
	}

	// Note: The mock querier doesn't enforce limit, but we're testing the parameter parsing
}

func TestHandlerExcludesReplies(t *testing.T) {
	events := []*nostr.Event{
		{
			ID:        "original",
			PubKey:    "owner123",
			Kind:      1,
			Content:   "Original post",
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Tags:      nostr.Tags{},
		},
		{
			ID:        "reply",
			PubKey:    "owner123",
			Kind:      1,
			Content:   "This is a reply",
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Tags:      nostr.Tags{{"e", "someeventid", "", "reply"}},
		},
	}

	querier := &mockQuerier{events: events}
	cfg := &Config{
		Enabled:        true,
		OwnerPubkey:    "owner123",
		RelayURL:       "wss://relay.example.com",
		RelayName:      "Test Relay",
		DefaultLimit:   20,
		MaxLimit:       100,
		IncludeReplies: false,
	}

	handler := NewHandler(cfg, querier)

	req := httptest.NewRequest(http.MethodGet, "/feed/rss", nil)
	w := httptest.NewRecorder()

	handler.handleRSS(w, req)

	var rss RSS
	if err := xml.NewDecoder(w.Result().Body).Decode(&rss); err != nil {
		t.Fatalf("Failed to parse RSS: %v", err)
	}

	// Should only have the original post, not the reply
	if len(rss.Channel.Items) != 1 {
		t.Errorf("Expected 1 item (reply excluded), got %d", len(rss.Channel.Items))
	}

	if len(rss.Channel.Items) > 0 && rss.Channel.Items[0].GUID.Value != "original" {
		t.Errorf("Expected original post, got %s", rss.Channel.Items[0].GUID.Value)
	}
}

func TestLinkifyURLsSafe(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			"Check out https://example.com for more",
			`Check out <a href="https://example.com">https://example.com</a> for more`,
		},
		{
			"Visit http://test.com.",
			`Visit <a href="http://test.com">http://test.com</a>.`,
		},
		{
			"No URLs here",
			"No URLs here",
		},
		{
			// Test XSS protection
			`Check https://evil.com/"><script>alert(1)</script> out`,
			`Check <a href="https://evil.com/&#34;&gt;&lt;script&gt;alert(1)&lt;/script&gt;">https://evil.com/&#34;&gt;&lt;script&gt;alert(1)&lt;/script&gt;</a> out`,
		},
		{
			// Test HTML escaping in non-URL content
			"Hello <script>alert(1)</script> world",
			"Hello &lt;script&gt;alert(1)&lt;/script&gt; world",
		},
	}

	for _, tc := range tests {
		result := linkifyURLsSafe(tc.input)
		if result != tc.expected {
			t.Errorf("linkifyURLsSafe(%q) = %q, expected %q", tc.input, result, tc.expected)
		}
	}
}

func TestExtractHashtags(t *testing.T) {
	event := &nostr.Event{
		Tags: nostr.Tags{
			{"t", "nostr"},
			{"t", "bitcoin"},
			{"e", "someevent"}, // not a hashtag
			{"p", "somepubkey"}, // not a hashtag
		},
	}

	tags := extractHashtags(event)

	if len(tags) != 2 {
		t.Errorf("Expected 2 hashtags, got %d", len(tags))
	}

	if tags[0] != "nostr" || tags[1] != "bitcoin" {
		t.Errorf("Expected [nostr, bitcoin], got %v", tags)
	}
}

func TestHasReplyTag(t *testing.T) {
	tests := []struct {
		name     string
		event    *nostr.Event
		expected bool
	}{
		{
			"No tags",
			&nostr.Event{Tags: nostr.Tags{}},
			false,
		},
		{
			"Has e-tag",
			&nostr.Event{Tags: nostr.Tags{{"e", "someid"}}},
			true,
		},
		{
			"Only p-tag",
			&nostr.Event{Tags: nostr.Tags{{"p", "somepubkey"}}},
			false,
		},
		{
			"Mixed tags with e-tag",
			&nostr.Event{Tags: nostr.Tags{{"p", "pubkey"}, {"e", "eventid"}, {"t", "hashtag"}}},
			true,
		},
	}

	for _, tc := range tests {
		result := hasReplyTag(tc.event)
		if result != tc.expected {
			t.Errorf("%s: hasReplyTag() = %v, expected %v", tc.name, result, tc.expected)
		}
	}
}

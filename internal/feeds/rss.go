package feeds

import (
	"encoding/xml"
	"html"
	"strings"
	"time"
)

// RSS represents an RSS 2.0 feed
type RSS struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel RSSChannel `xml:"channel"`
}

// RSSChannel represents the channel element of an RSS feed
type RSSChannel struct {
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	Description   string    `xml:"description"`
	Language      string    `xml:"language,omitempty"`
	LastBuildDate string    `xml:"lastBuildDate,omitempty"`
	Generator     string    `xml:"generator,omitempty"`
	Items         []RSSItem `xml:"item"`
}

// RSSItem represents a single item in an RSS feed
type RSSItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	Author      string `xml:"author,omitempty"`
	GUID        RSSGUID `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	Categories  []string `xml:"category,omitempty"`
}

// RSSGUID represents a unique identifier for an RSS item
type RSSGUID struct {
	IsPermaLink bool   `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}

// GenerateRSS creates an RSS 2.0 feed from metadata and items
func GenerateRSS(meta FeedMetadata, items []FeedItem) ([]byte, error) {
	rss := RSS{
		Version: "2.0",
		Channel: RSSChannel{
			Title:         meta.Title,
			Link:          meta.Link,
			Description:   meta.Description,
			Language:      meta.Language,
			LastBuildDate: formatRSSDate(meta.Updated),
			Generator:     meta.Generator,
			Items:         make([]RSSItem, 0, len(items)),
		},
	}

	for _, item := range items {
		rssItem := RSSItem{
			Title:       item.Title,
			Link:        item.Link,
			Description: formatRSSDescription(item),
			Author:      item.Author,
			GUID: RSSGUID{
				IsPermaLink: false,
				Value:       item.ID,
			},
			PubDate:    formatRSSDate(item.Published),
			Categories: item.Tags,
		}
		rss.Channel.Items = append(rss.Channel.Items, rssItem)
	}

	output, err := xml.MarshalIndent(rss, "", "  ")
	if err != nil {
		return nil, err
	}

	// Prepend XML declaration
	return append([]byte(xml.Header), output...), nil
}

// formatRSSDate formats a time for RSS (RFC 1123 format)
func formatRSSDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC1123Z)
}

// formatRSSDescription creates the HTML description for an RSS item
func formatRSSDescription(item FeedItem) string {
	// If we have pre-formatted HTML, use it
	if item.ContentHTML != "" {
		return item.ContentHTML
	}

	// Otherwise, convert plain text to HTML
	content := html.EscapeString(item.Content)

	// Convert newlines to <br> tags
	content = strings.ReplaceAll(content, "\n", "<br>\n")

	// Wrap in paragraph
	return "<p>" + content + "</p>"
}

// extractTitle extracts a title from content (first line or truncated)
func extractTitle(content string, maxLen int) string {
	// Trim whitespace
	content = strings.TrimSpace(content)

	// Get first line
	firstLine := content
	if idx := strings.Index(content, "\n"); idx != -1 {
		firstLine = content[:idx]
	}

	// Trim again
	firstLine = strings.TrimSpace(firstLine)

	// Truncate if too long
	if len(firstLine) > maxLen {
		// Try to truncate at a word boundary
		truncated := firstLine[:maxLen]
		if lastSpace := strings.LastIndex(truncated, " "); lastSpace > maxLen/2 {
			truncated = truncated[:lastSpace]
		}
		return truncated + "..."
	}

	return firstLine
}

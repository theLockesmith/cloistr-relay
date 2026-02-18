package feeds

import (
	"encoding/xml"
	"time"
)

// AtomFeed represents an Atom 1.0 feed
type AtomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	XMLNS   string      `xml:"xmlns,attr"`
	Title   string      `xml:"title"`
	ID      string      `xml:"id"`
	Updated string      `xml:"updated"`
	Links   []AtomLink  `xml:"link"`
	Author  *AtomPerson `xml:"author,omitempty"`
	Generator *AtomGenerator `xml:"generator,omitempty"`
	Entries []AtomEntry `xml:"entry"`
}

// AtomLink represents a link element in Atom
type AtomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr,omitempty"`
	Type string `xml:"type,attr,omitempty"`
}

// AtomPerson represents a person (author/contributor) in Atom
type AtomPerson struct {
	Name string `xml:"name"`
	URI  string `xml:"uri,omitempty"`
}

// AtomGenerator represents the generator element in Atom
type AtomGenerator struct {
	URI     string `xml:"uri,attr,omitempty"`
	Version string `xml:"version,attr,omitempty"`
	Value   string `xml:",chardata"`
}

// AtomEntry represents a single entry in an Atom feed
type AtomEntry struct {
	Title     string      `xml:"title"`
	ID        string      `xml:"id"`
	Updated   string      `xml:"updated"`
	Published string      `xml:"published,omitempty"`
	Links     []AtomLink  `xml:"link"`
	Author    *AtomPerson `xml:"author,omitempty"`
	Content   AtomContent `xml:"content"`
	Categories []AtomCategory `xml:"category,omitempty"`
}

// AtomContent represents content in an Atom entry
type AtomContent struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

// AtomCategory represents a category in an Atom entry
type AtomCategory struct {
	Term string `xml:"term,attr"`
}

// GenerateAtom creates an Atom 1.0 feed from metadata and items
func GenerateAtom(meta FeedMetadata, items []FeedItem) ([]byte, error) {
	feed := AtomFeed{
		XMLNS:   "http://www.w3.org/2005/Atom",
		Title:   meta.Title,
		ID:      meta.FeedLink,
		Updated: formatAtomDate(meta.Updated),
		Links: []AtomLink{
			{Href: meta.Link, Rel: "alternate", Type: "text/html"},
			{Href: meta.FeedLink, Rel: "self", Type: "application/atom+xml"},
		},
		Entries: make([]AtomEntry, 0, len(items)),
	}

	// Add author if available
	if meta.Author != "" {
		feed.Author = &AtomPerson{
			Name: meta.Author,
		}
	}

	// Add generator
	if meta.Generator != "" {
		feed.Generator = &AtomGenerator{
			Value: meta.Generator,
		}
	}

	for _, item := range items {
		entry := AtomEntry{
			Title:     item.Title,
			ID:        "urn:nostr:" + item.ID,
			Updated:   formatAtomDate(item.Updated),
			Published: formatAtomDate(item.Published),
			Links: []AtomLink{
				{Href: item.Link, Rel: "alternate", Type: "text/html"},
			},
			Content: AtomContent{
				Type:  "html",
				Value: formatAtomContent(item),
			},
		}

		// Add author
		if item.Author != "" {
			entry.Author = &AtomPerson{
				Name: item.Author,
			}
		}

		// Add categories
		for _, tag := range item.Tags {
			entry.Categories = append(entry.Categories, AtomCategory{Term: tag})
		}

		feed.Entries = append(feed.Entries, entry)
	}

	output, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		return nil, err
	}

	// Prepend XML declaration
	return append([]byte(xml.Header), output...), nil
}

// formatAtomDate formats a time for Atom (RFC 3339 format)
func formatAtomDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// formatAtomContent creates the HTML content for an Atom entry
func formatAtomContent(item FeedItem) string {
	// If we have pre-formatted HTML, use it
	if item.ContentHTML != "" {
		return item.ContentHTML
	}

	// Otherwise, use the RSS description formatter
	return formatRSSDescription(item)
}

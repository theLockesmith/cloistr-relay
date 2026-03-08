// Package external implements NIP-73 external content IDs
//
// NIP-73 allows events to reference external content by global identifiers:
// - ISBN (books)
// - DOI (academic papers)
// - ISAN (audiovisual works)
// - Podcast GUIDs
// - Movie/TV IDs
// - Music identifiers
//
// This enables cross-referencing Nostr content with external media.
//
// Reference: https://github.com/nostr-protocol/nips/blob/master/73.md
package external

import (
	"strings"

	"github.com/nbd-wtf/go-nostr"
)

// External ID types (i-tag prefixes)
const (
	TypeISBN       = "isbn"
	TypeDOI        = "doi"
	TypeISAN       = "isan"
	TypePodcast    = "podcast:guid"
	TypePodcastEp  = "podcast:item:guid"
	TypeIMDB       = "imdb"
	TypeTMDB       = "tmdb"
	TypeSpotify    = "spotify"
	TypeMusicBrainz = "musicbrainz"
	TypeOpenlibrary = "openlibrary"
)

// ExternalRef represents an external content reference
type ExternalRef struct {
	// Type is the identifier type (e.g., "isbn", "doi")
	Type string
	// ID is the actual identifier value
	ID string
	// Platform is an optional platform hint
	Platform string
}

// ParseExternalRefs extracts external references from an event
func ParseExternalRefs(event *nostr.Event) []ExternalRef {
	var refs []ExternalRef

	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}

		if tag[0] == "i" {
			ref := parseITag(tag[1])
			if ref != nil {
				refs = append(refs, *ref)
			}
		}
	}

	return refs
}

// parseITag parses an i-tag value like "isbn:9780141036144"
func parseITag(value string) *ExternalRef {
	parts := strings.SplitN(value, ":", 2)
	if len(parts) != 2 {
		return nil
	}

	return &ExternalRef{
		Type: parts[0],
		ID:   parts[1],
	}
}

// FormatITag formats an ExternalRef as an i-tag value
func FormatITag(ref ExternalRef) string {
	return ref.Type + ":" + ref.ID
}

// AddExternalRef adds an external reference tag to an event
func AddExternalRef(event *nostr.Event, ref ExternalRef) {
	event.Tags = append(event.Tags, nostr.Tag{"i", FormatITag(ref)})
}

// ParseKTags extracts k-tags (kind hints for external content)
func ParseKTags(event *nostr.Event) []string {
	var kinds []string

	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "k" {
			kinds = append(kinds, tag[1])
		}
	}

	return kinds
}

// IsValidISBN performs basic ISBN validation (length check)
func IsValidISBN(isbn string) bool {
	// Remove hyphens and spaces
	cleaned := strings.ReplaceAll(isbn, "-", "")
	cleaned = strings.ReplaceAll(cleaned, " ", "")

	// ISBN-10 or ISBN-13
	return len(cleaned) == 10 || len(cleaned) == 13
}

// IsValidDOI performs basic DOI validation
func IsValidDOI(doi string) bool {
	// DOI format: 10.prefix/suffix
	return strings.HasPrefix(doi, "10.") && strings.Contains(doi, "/")
}

// IsExternalRefTag returns true if the tag is an i-tag
func IsExternalRefTag(tag nostr.Tag) bool {
	return len(tag) >= 2 && tag[0] == "i"
}

// GetRefsOfType filters external refs by type
func GetRefsOfType(refs []ExternalRef, refType string) []ExternalRef {
	var filtered []ExternalRef
	for _, ref := range refs {
		if ref.Type == refType {
			filtered = append(filtered, ref)
		}
	}
	return filtered
}

// CommonExternalTypes returns commonly used external ID types
func CommonExternalTypes() []string {
	return []string{
		TypeISBN,
		TypeDOI,
		TypeISAN,
		TypePodcast,
		TypePodcastEp,
		TypeIMDB,
		TypeTMDB,
		TypeSpotify,
		TypeMusicBrainz,
		TypeOpenlibrary,
	}
}

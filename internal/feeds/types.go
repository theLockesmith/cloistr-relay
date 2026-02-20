package feeds

import (
	"time"
)

// Config holds RSS/Atom feed configuration
type Config struct {
	// Enabled enables RSS/Atom feed endpoints
	Enabled bool

	// OwnerPubkey is the pubkey of the relay owner (required for HAVEN mode)
	OwnerPubkey string

	// RelayURL is the public URL of the relay
	RelayURL string

	// RelayName is the name of the relay (used in feed title)
	RelayName string

	// DefaultLimit is the default number of items in feeds (default: 20)
	DefaultLimit int

	// MaxLimit is the maximum number of items allowed (default: 100)
	MaxLimit int

	// CacheTTL is how long to cache feed responses (default: 5 minutes)
	CacheTTL time.Duration

	// IncludeLongForm includes kind 30023 long-form articles in feeds
	IncludeLongForm bool

	// IncludeReplies includes replies (events with e-tags) in feeds
	IncludeReplies bool

	// DefaultAlgorithm is the default feed algorithm (chronological if not set)
	DefaultAlgorithm string
}

// DefaultConfig returns sensible defaults for feed configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:         true,
		DefaultLimit:    20,
		MaxLimit:        100,
		CacheTTL:        5 * time.Minute,
		IncludeLongForm: true,
		IncludeReplies:  false,
	}
}

// FeedItem represents a single item in an RSS/Atom feed
type FeedItem struct {
	// ID is the unique identifier (event ID)
	ID string

	// Title is the item title (first line or truncated content)
	Title string

	// Content is the full content of the item
	Content string

	// ContentHTML is the HTML-formatted content
	ContentHTML string

	// Link is the URL to view the item
	Link string

	// Author is the author name or npub
	Author string

	// AuthorPubkey is the hex pubkey of the author
	AuthorPubkey string

	// Published is the creation timestamp
	Published time.Time

	// Updated is the last update timestamp (same as Published for Nostr)
	Updated time.Time

	// Kind is the Nostr event kind
	Kind int

	// Tags contains any relevant tags (hashtags, etc.)
	Tags []string
}

// FeedMetadata holds feed-level metadata
type FeedMetadata struct {
	// Title is the feed title
	Title string

	// Description is the feed description
	Description string

	// Link is the feed's web URL
	Link string

	// FeedLink is the URL of the feed itself
	FeedLink string

	// Author is the feed author
	Author string

	// AuthorPubkey is the hex pubkey of the author
	AuthorPubkey string

	// Updated is when the feed was last updated
	Updated time.Time

	// Generator is the software that generated the feed
	Generator string

	// Language is the feed language (default: "en")
	Language string
}

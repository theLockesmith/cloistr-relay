package algo

import (
	"math"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

// AlgorithmName represents a named feed algorithm
type AlgorithmName string

const (
	// AlgoChronological is the default - no ranking, just chronological order
	AlgoChronological AlgorithmName = "chronological"

	// AlgoWoTRanked ranks events by Web of Trust level
	AlgoWoTRanked AlgorithmName = "wot-ranked"

	// AlgoEngagement ranks events by engagement (reactions, reposts, zaps)
	AlgoEngagement AlgorithmName = "engagement"

	// AlgoTrending combines recency with engagement for trending content
	AlgoTrending AlgorithmName = "trending"

	// AlgoPersonalized uses user preferences for custom ranking
	AlgoPersonalized AlgorithmName = "personalized"
)

// String returns the algorithm name as a string
func (a AlgorithmName) String() string {
	return string(a)
}

// IsValid checks if the algorithm name is valid
func (a AlgorithmName) IsValid() bool {
	switch a {
	case AlgoChronological, AlgoWoTRanked, AlgoEngagement, AlgoTrending, AlgoPersonalized:
		return true
	default:
		return false
	}
}

// ParseAlgorithm parses a string into an AlgorithmName
func ParseAlgorithm(s string) AlgorithmName {
	algo := AlgorithmName(s)
	if algo.IsValid() {
		return algo
	}
	return AlgoChronological // Default fallback
}

// ScoredEvent wraps an event with its calculated score
type ScoredEvent struct {
	Event *nostr.Event
	Score float64

	// Score components (for debugging/transparency)
	WoTScore        float64
	EngagementScore float64
	RecencyScore    float64
	PersonalScore   float64
}

// EngagementStats holds engagement metrics for an event
type EngagementStats struct {
	Reactions int64 // kind 7 events referencing this event
	Reposts   int64 // kind 6 events referencing this event
	Zaps      int64 // kind 9735 zap receipts referencing this event
	ZapAmount int64 // total sats zapped (in millisats)
	Replies   int64 // kind 1 events replying to this event
}

// TotalEngagement returns a weighted engagement score
func (e EngagementStats) TotalEngagement() float64 {
	// Weights: reactions=1, reposts=3, replies=2, zaps=5 + amount bonus
	score := float64(e.Reactions) +
		float64(e.Reposts)*3 +
		float64(e.Replies)*2 +
		float64(e.Zaps)*5

	// Add bonus for zap amounts (log scale to prevent huge zaps from dominating)
	if e.ZapAmount > 0 {
		// 1000 sats = +1, 10000 sats = +2, 100000 sats = +3, etc.
		score += logBase(float64(e.ZapAmount)/1000, 10)
	}

	return score
}

// UserPreferences stores user-defined feed preferences
type UserPreferences struct {
	// MutedPubkeys are pubkeys to filter out
	MutedPubkeys map[string]bool

	// MutedWords are words/phrases to filter out
	MutedWords []string

	// BoostedPubkeys are pubkeys to boost in rankings
	BoostedPubkeys map[string]float64 // pubkey -> boost multiplier

	// BoostedHashtags are hashtags to boost
	BoostedHashtags map[string]float64 // hashtag -> boost multiplier

	// MutedHashtags are hashtags to filter out
	MutedHashtags map[string]bool

	// PreferredLanguages for content (empty = all)
	PreferredLanguages []string

	// MinWoTLevel filters out events below this trust level
	MinWoTLevel int
}

// DefaultPreferences returns empty preferences (no filtering/boosting)
func DefaultPreferences() *UserPreferences {
	return &UserPreferences{
		MutedPubkeys:    make(map[string]bool),
		BoostedPubkeys:  make(map[string]float64),
		BoostedHashtags: make(map[string]float64),
		MutedHashtags:   make(map[string]bool),
	}
}

// Config holds algorithm engine configuration
type Config struct {
	// Enabled enables algorithmic feed support
	Enabled bool

	// DefaultAlgorithm is the default when none specified
	DefaultAlgorithm AlgorithmName

	// WoTWeight is the weight for WoT score in combined algorithms (0-1)
	WoTWeight float64

	// EngagementWeight is the weight for engagement score (0-1)
	EngagementWeight float64

	// RecencyWeight is the weight for recency score (0-1)
	RecencyWeight float64

	// RecencyHalfLife is the time for recency score to decay to 50%
	RecencyHalfLife time.Duration

	// EngagementCacheTTL is how long to cache engagement stats
	EngagementCacheTTL time.Duration

	// MaxEventsToScore is the maximum events to score per request
	MaxEventsToScore int
}

// DefaultConfig returns sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Enabled:            true,
		DefaultAlgorithm:   AlgoChronological,
		WoTWeight:          0.3,
		EngagementWeight:   0.4,
		RecencyWeight:      0.3,
		RecencyHalfLife:    24 * time.Hour,
		EngagementCacheTTL: 5 * time.Minute,
		MaxEventsToScore:   500,
	}
}

// logBase calculates log base b of x
func logBase(x, b float64) float64 {
	if x <= 0 || b <= 0 || b == 1 {
		return 0
	}
	return math.Log(x) / math.Log(b)
}

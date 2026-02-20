package algo

import (
	"context"
	"database/sql"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"

	"git.coldforge.xyz/coldforge/cloistr-relay/internal/wot"
)

// TrustLevelGetter is an interface for getting trust levels
type TrustLevelGetter interface {
	GetTrustContext(pubkey string) wot.TrustLevel
}

// Scorer scores and ranks events based on the selected algorithm
type Scorer struct {
	config          *Config
	db              *sql.DB
	trustGetter     TrustLevelGetter
	engagementCache sync.Map // eventID -> *cachedEngagement
}

type cachedEngagement struct {
	stats     EngagementStats
	cachedAt  time.Time
}

// NewScorer creates a new event scorer
func NewScorer(cfg *Config, db *sql.DB, trustGetter TrustLevelGetter) *Scorer {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Scorer{
		config:      cfg,
		db:          db,
		trustGetter: trustGetter,
	}
}

// ScoreEvents scores and sorts events according to the specified algorithm
func (s *Scorer) ScoreEvents(ctx context.Context, events []*nostr.Event, algo AlgorithmName, prefs *UserPreferences) []*ScoredEvent {
	if len(events) == 0 {
		return nil
	}

	// Limit events to score
	if len(events) > s.config.MaxEventsToScore {
		events = events[:s.config.MaxEventsToScore]
	}

	// Apply mute filters first if we have preferences
	if prefs != nil {
		events = s.applyMuteFilters(events, prefs)
	}

	// Score each event
	scored := make([]*ScoredEvent, 0, len(events))
	for _, event := range events {
		se := s.scoreEvent(ctx, event, algo, prefs)
		if se != nil {
			scored = append(scored, se)
		}
	}

	// Sort by score (descending)
	if algo != AlgoChronological {
		sort.Slice(scored, func(i, j int) bool {
			return scored[i].Score > scored[j].Score
		})
	}

	return scored
}

// scoreEvent calculates the score for a single event
func (s *Scorer) scoreEvent(ctx context.Context, event *nostr.Event, algo AlgorithmName, prefs *UserPreferences) *ScoredEvent {
	se := &ScoredEvent{
		Event: event,
		Score: 0,
	}

	switch algo {
	case AlgoChronological:
		// Use created_at as score (higher = more recent)
		se.Score = float64(event.CreatedAt)
		se.RecencyScore = se.Score

	case AlgoWoTRanked:
		se.WoTScore = s.calculateWoTScore(event.PubKey)
		se.RecencyScore = s.calculateRecencyScore(event.CreatedAt)
		// WoT-ranked: 70% WoT, 30% recency
		se.Score = se.WoTScore*0.7 + se.RecencyScore*0.3

	case AlgoEngagement:
		se.EngagementScore = s.calculateEngagementScore(ctx, event.ID)
		se.RecencyScore = s.calculateRecencyScore(event.CreatedAt)
		// Engagement: 70% engagement, 30% recency
		se.Score = se.EngagementScore*0.7 + se.RecencyScore*0.3

	case AlgoTrending:
		se.WoTScore = s.calculateWoTScore(event.PubKey)
		se.EngagementScore = s.calculateEngagementScore(ctx, event.ID)
		se.RecencyScore = s.calculateRecencyScore(event.CreatedAt)
		// Trending: configurable weights
		se.Score = se.WoTScore*s.config.WoTWeight +
			se.EngagementScore*s.config.EngagementWeight +
			se.RecencyScore*s.config.RecencyWeight

	case AlgoPersonalized:
		se.WoTScore = s.calculateWoTScore(event.PubKey)
		se.EngagementScore = s.calculateEngagementScore(ctx, event.ID)
		se.RecencyScore = s.calculateRecencyScore(event.CreatedAt)
		se.PersonalScore = s.calculatePersonalScore(event, prefs)
		// Personalized: blend all scores with personal boost
		baseScore := se.WoTScore*0.2 + se.EngagementScore*0.3 + se.RecencyScore*0.2
		se.Score = baseScore * (1 + se.PersonalScore*0.5)

	default:
		// Fallback to chronological
		se.Score = float64(event.CreatedAt)
	}

	return se
}

// calculateWoTScore converts trust level to a 0-1 score
func (s *Scorer) calculateWoTScore(pubkey string) float64 {
	if s.trustGetter == nil {
		return 0.5 // Neutral score if no WoT available
	}

	level := s.trustGetter.GetTrustContext(pubkey)

	switch level {
	case wot.TrustLevelOwner:
		return 1.0
	case wot.TrustLevelFollow:
		return 0.9
	case wot.TrustLevelFollowOfFollow:
		return 0.7
	default:
		return 0.3 // Unknown users get lower score
	}
}

// calculateRecencyScore converts timestamp to a 0-1 score with exponential decay
func (s *Scorer) calculateRecencyScore(createdAt nostr.Timestamp) float64 {
	age := time.Since(time.Unix(int64(createdAt), 0))

	// Exponential decay with configurable half-life
	halfLife := s.config.RecencyHalfLife
	if halfLife <= 0 {
		halfLife = 24 * time.Hour
	}

	// score = 0.5 ^ (age / halfLife)
	decayFactor := age.Seconds() / halfLife.Seconds()
	score := math.Pow(0.5, decayFactor)

	return score
}

// calculateEngagementScore fetches and calculates engagement score
func (s *Scorer) calculateEngagementScore(ctx context.Context, eventID string) float64 {
	stats := s.getEngagementStats(ctx, eventID)
	if stats == nil {
		return 0
	}

	// Normalize engagement to 0-1 range
	// Use log scale to handle viral content without overwhelming
	total := stats.TotalEngagement()
	if total <= 0 {
		return 0
	}

	// Log scale: 1 engagement = 0.1, 10 = 0.3, 100 = 0.5, 1000 = 0.7, 10000 = 0.9
	normalized := math.Log10(total+1) / 5.0
	if normalized > 1.0 {
		normalized = 1.0
	}

	return normalized
}

// calculatePersonalScore calculates boost based on user preferences
func (s *Scorer) calculatePersonalScore(event *nostr.Event, prefs *UserPreferences) float64 {
	if prefs == nil {
		return 0
	}

	score := 0.0

	// Check boosted pubkeys
	if boost, ok := prefs.BoostedPubkeys[event.PubKey]; ok {
		score += boost
	}

	// Check boosted hashtags
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "t" {
			hashtag := tag[1]
			if boost, ok := prefs.BoostedHashtags[hashtag]; ok {
				score += boost
			}
		}
	}

	// Cap score at 2.0 (100% boost)
	if score > 2.0 {
		score = 2.0
	}

	return score
}

// applyMuteFilters removes events that match mute filters
func (s *Scorer) applyMuteFilters(events []*nostr.Event, prefs *UserPreferences) []*nostr.Event {
	if prefs == nil {
		return events
	}

	filtered := make([]*nostr.Event, 0, len(events))
	for _, event := range events {
		// Check muted pubkeys
		if prefs.MutedPubkeys[event.PubKey] {
			continue
		}

		// Check muted hashtags
		muted := false
		for _, tag := range event.Tags {
			if len(tag) >= 2 && tag[0] == "t" {
				if prefs.MutedHashtags[tag[1]] {
					muted = true
					break
				}
			}
		}
		if muted {
			continue
		}

		// Check muted words (simple substring match)
		for _, word := range prefs.MutedWords {
			if containsIgnoreCase(event.Content, word) {
				muted = true
				break
			}
		}
		if muted {
			continue
		}

		// Check minimum WoT level
		if prefs.MinWoTLevel > 0 && s.trustGetter != nil {
			level := s.trustGetter.GetTrustContext(event.PubKey)
			if int(level) > prefs.MinWoTLevel {
				continue // Skip events from less trusted users
			}
		}

		filtered = append(filtered, event)
	}

	return filtered
}

// getEngagementStats fetches engagement stats for an event (with caching)
func (s *Scorer) getEngagementStats(ctx context.Context, eventID string) *EngagementStats {
	// Check cache first
	if cached, ok := s.engagementCache.Load(eventID); ok {
		ce := cached.(*cachedEngagement)
		if time.Since(ce.cachedAt) < s.config.EngagementCacheTTL {
			return &ce.stats
		}
	}

	// Query database for engagement
	stats := s.queryEngagementStats(ctx, eventID)
	if stats == nil {
		return nil
	}

	// Cache the result
	s.engagementCache.Store(eventID, &cachedEngagement{
		stats:    *stats,
		cachedAt: time.Now(),
	})

	return stats
}

// queryEngagementStats queries the database for engagement metrics
func (s *Scorer) queryEngagementStats(ctx context.Context, eventID string) *EngagementStats {
	if s.db == nil {
		return &EngagementStats{}
	}

	stats := &EngagementStats{}

	// Query reactions (kind 7)
	row := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM event
		WHERE kind = 7 AND tags @> $1::jsonb
	`, `[["e", "`+eventID+`"]]`)
	row.Scan(&stats.Reactions)

	// Query reposts (kind 6)
	row = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM event
		WHERE kind = 6 AND tags @> $1::jsonb
	`, `[["e", "`+eventID+`"]]`)
	row.Scan(&stats.Reposts)

	// Query replies (kind 1 with e-tag)
	row = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM event
		WHERE kind = 1 AND tags @> $1::jsonb
	`, `[["e", "`+eventID+`"]]`)
	row.Scan(&stats.Replies)

	// Query zaps (kind 9735) - count and sum amounts
	row = s.db.QueryRowContext(ctx, `
		SELECT COALESCE(COUNT(*), 0), COALESCE(SUM(
			CASE
				WHEN tags @> '[["amount"]]' THEN
					(SELECT (t->>1)::bigint FROM jsonb_array_elements(tags) t WHERE t->>0 = 'amount' LIMIT 1)
				ELSE 0
			END
		), 0)
		FROM event
		WHERE kind = 9735 AND tags @> $1::jsonb
	`, `[["e", "`+eventID+`"]]`)
	row.Scan(&stats.Zaps, &stats.ZapAmount)

	return stats
}

// containsIgnoreCase checks if s contains substr (case insensitive)
func containsIgnoreCase(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}

	// Simple case-insensitive contains
	sLower := toLower(s)
	substrLower := toLower(substr)

	for i := 0; i <= len(sLower)-len(substrLower); i++ {
		if sLower[i:i+len(substrLower)] == substrLower {
			return true
		}
	}
	return false
}

// toLower converts ASCII to lowercase
func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

// ExtractAlgorithmFromFilter checks if a filter has an #algo tag and returns the algorithm
func ExtractAlgorithmFromFilter(filter nostr.Filter) AlgorithmName {
	// Check for #algo tag
	if algoTags, ok := filter.Tags["#algo"]; ok && len(algoTags) > 0 {
		return ParseAlgorithm(algoTags[0])
	}

	// Check for algo tag (without #)
	if algoTags, ok := filter.Tags["algo"]; ok && len(algoTags) > 0 {
		return ParseAlgorithm(algoTags[0])
	}

	return AlgoChronological
}

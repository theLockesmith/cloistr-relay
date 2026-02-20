package algo

import (
	"context"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"

	"git.coldforge.xyz/coldforge/cloistr-relay/internal/wot"
)

// mockTrustGetter implements TrustLevelGetter for testing
type mockTrustGetter struct {
	levels map[string]wot.TrustLevel
}

func (m *mockTrustGetter) GetTrustContext(pubkey string) wot.TrustLevel {
	if level, ok := m.levels[pubkey]; ok {
		return level
	}
	return wot.TrustLevelUnknown
}

func TestAlgorithmNameParsing(t *testing.T) {
	tests := []struct {
		input    string
		expected AlgorithmName
	}{
		{"chronological", AlgoChronological},
		{"wot-ranked", AlgoWoTRanked},
		{"engagement", AlgoEngagement},
		{"trending", AlgoTrending},
		{"personalized", AlgoPersonalized},
		{"invalid", AlgoChronological}, // Default fallback
		{"", AlgoChronological},        // Empty string
	}

	for _, tc := range tests {
		result := ParseAlgorithm(tc.input)
		if result != tc.expected {
			t.Errorf("ParseAlgorithm(%q) = %q, expected %q", tc.input, result, tc.expected)
		}
	}
}

func TestAlgorithmNameValidity(t *testing.T) {
	tests := []struct {
		algo  AlgorithmName
		valid bool
	}{
		{AlgoChronological, true},
		{AlgoWoTRanked, true},
		{AlgoEngagement, true},
		{AlgoTrending, true},
		{AlgoPersonalized, true},
		{AlgorithmName("invalid"), false},
	}

	for _, tc := range tests {
		result := tc.algo.IsValid()
		if result != tc.valid {
			t.Errorf("%q.IsValid() = %v, expected %v", tc.algo, result, tc.valid)
		}
	}
}

func TestEngagementStatsTotalEngagement(t *testing.T) {
	tests := []struct {
		name     string
		stats    EngagementStats
		minScore float64
		maxScore float64
	}{
		{
			name:     "Empty stats",
			stats:    EngagementStats{},
			minScore: 0,
			maxScore: 0,
		},
		{
			name:     "Reactions only",
			stats:    EngagementStats{Reactions: 10},
			minScore: 10,
			maxScore: 10,
		},
		{
			name:     "Reposts weighted 3x",
			stats:    EngagementStats{Reposts: 5},
			minScore: 15,
			maxScore: 15,
		},
		{
			name:     "Zaps with amount",
			stats:    EngagementStats{Zaps: 3, ZapAmount: 10000}, // 10000 millisats = 10 sats
			minScore: 15, // 3*5 = 15
			maxScore: 20, // 15 + some bonus from amount
		},
		{
			name:     "Mixed engagement",
			stats:    EngagementStats{Reactions: 5, Reposts: 2, Replies: 3, Zaps: 1},
			minScore: 16, // 5*1 + 2*3 + 3*2 + 1*5 = 5 + 6 + 6 + 5 = 22
			maxScore: 25,
		},
	}

	for _, tc := range tests {
		total := tc.stats.TotalEngagement()
		if total < tc.minScore || total > tc.maxScore {
			t.Errorf("%s: TotalEngagement() = %v, expected between %v and %v",
				tc.name, total, tc.minScore, tc.maxScore)
		}
	}
}

func TestScorerWoTScore(t *testing.T) {
	trustGetter := &mockTrustGetter{
		levels: map[string]wot.TrustLevel{
			"owner":         wot.TrustLevelOwner,
			"direct_follow": wot.TrustLevelFollow,
			"fof":           wot.TrustLevelFollowOfFollow,
			"unknown":       wot.TrustLevelUnknown,
		},
	}

	scorer := NewScorer(DefaultConfig(), nil, trustGetter)

	tests := []struct {
		pubkey   string
		expected float64
	}{
		{"owner", 1.0},
		{"direct_follow", 0.9},
		{"fof", 0.7},
		{"unknown", 0.3},
		{"not_in_map", 0.3}, // Unknown defaults to 0.3
	}

	for _, tc := range tests {
		score := scorer.calculateWoTScore(tc.pubkey)
		if score != tc.expected {
			t.Errorf("calculateWoTScore(%q) = %v, expected %v", tc.pubkey, score, tc.expected)
		}
	}
}

func TestScorerRecencyScore(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RecencyHalfLife = 24 * time.Hour
	scorer := NewScorer(cfg, nil, nil)

	now := time.Now()

	tests := []struct {
		name     string
		age      time.Duration
		minScore float64
		maxScore float64
	}{
		{"Just now", 0, 0.95, 1.0},
		{"1 hour ago", time.Hour, 0.95, 1.0},
		{"12 hours ago", 12 * time.Hour, 0.6, 0.75},
		{"24 hours ago", 24 * time.Hour, 0.45, 0.55}, // Half-life
		{"48 hours ago", 48 * time.Hour, 0.2, 0.3},
		{"1 week ago", 7 * 24 * time.Hour, 0, 0.1},
	}

	for _, tc := range tests {
		timestamp := nostr.Timestamp(now.Add(-tc.age).Unix())
		score := scorer.calculateRecencyScore(timestamp)
		if score < tc.minScore || score > tc.maxScore {
			t.Errorf("%s: calculateRecencyScore() = %v, expected between %v and %v",
				tc.name, score, tc.minScore, tc.maxScore)
		}
	}
}

func TestScorerScoreEventsChronological(t *testing.T) {
	scorer := NewScorer(DefaultConfig(), nil, nil)

	now := time.Now()
	events := []*nostr.Event{
		{ID: "c", CreatedAt: nostr.Timestamp(now.Add(-2 * time.Hour).Unix())},
		{ID: "a", CreatedAt: nostr.Timestamp(now.Unix())},
		{ID: "b", CreatedAt: nostr.Timestamp(now.Add(-1 * time.Hour).Unix())},
	}

	scored := scorer.ScoreEvents(context.Background(), events, AlgoChronological, nil)

	// Chronological should preserve order (not re-sort)
	if len(scored) != 3 {
		t.Fatalf("Expected 3 scored events, got %d", len(scored))
	}

	// Scores should be based on timestamp
	if scored[0].Event.ID != "c" {
		t.Errorf("First event should be 'c' (chronological preserves order)")
	}
}

func TestScorerScoreEventsWoTRanked(t *testing.T) {
	trustGetter := &mockTrustGetter{
		levels: map[string]wot.TrustLevel{
			"owner":   wot.TrustLevelOwner,
			"follow":  wot.TrustLevelFollow,
			"unknown": wot.TrustLevelUnknown,
		},
	}

	scorer := NewScorer(DefaultConfig(), nil, trustGetter)

	now := time.Now()
	events := []*nostr.Event{
		{ID: "unknown_post", PubKey: "unknown", CreatedAt: nostr.Timestamp(now.Unix())},
		{ID: "owner_post", PubKey: "owner", CreatedAt: nostr.Timestamp(now.Add(-1 * time.Hour).Unix())},
		{ID: "follow_post", PubKey: "follow", CreatedAt: nostr.Timestamp(now.Add(-30 * time.Minute).Unix())},
	}

	scored := scorer.ScoreEvents(context.Background(), events, AlgoWoTRanked, nil)

	if len(scored) != 3 {
		t.Fatalf("Expected 3 scored events, got %d", len(scored))
	}

	// Owner should be first (highest WoT), then follow, then unknown
	if scored[0].Event.ID != "owner_post" {
		t.Errorf("First event should be owner_post, got %s", scored[0].Event.ID)
	}
	if scored[1].Event.ID != "follow_post" {
		t.Errorf("Second event should be follow_post, got %s", scored[1].Event.ID)
	}
	if scored[2].Event.ID != "unknown_post" {
		t.Errorf("Third event should be unknown_post, got %s", scored[2].Event.ID)
	}
}

func TestScorerMuteFilters(t *testing.T) {
	scorer := NewScorer(DefaultConfig(), nil, nil)

	events := []*nostr.Event{
		{ID: "keep1", PubKey: "good_user", Content: "Hello world"},
		{ID: "muted_user", PubKey: "bad_user", Content: "Hi there"},
		{ID: "muted_word", PubKey: "good_user", Content: "This contains badword in it"},
		{ID: "muted_hashtag", PubKey: "good_user", Content: "Check this", Tags: nostr.Tags{{"t", "spam"}}},
		{ID: "keep2", PubKey: "good_user", Content: "Another post", Tags: nostr.Tags{{"t", "nostr"}}},
	}

	prefs := &UserPreferences{
		MutedPubkeys:  map[string]bool{"bad_user": true},
		MutedWords:    []string{"badword"},
		MutedHashtags: map[string]bool{"spam": true},
	}

	scored := scorer.ScoreEvents(context.Background(), events, AlgoChronological, prefs)

	// Should only have keep1 and keep2
	if len(scored) != 2 {
		t.Fatalf("Expected 2 events after filtering, got %d", len(scored))
	}

	for _, se := range scored {
		if se.Event.ID == "muted_user" || se.Event.ID == "muted_word" || se.Event.ID == "muted_hashtag" {
			t.Errorf("Event %s should have been filtered out", se.Event.ID)
		}
	}
}

func TestExtractAlgorithmFromFilter(t *testing.T) {
	tests := []struct {
		name     string
		filter   nostr.Filter
		expected AlgorithmName
	}{
		{
			name:     "No algo tag",
			filter:   nostr.Filter{Kinds: []int{1}},
			expected: AlgoChronological,
		},
		{
			name: "With #algo tag",
			filter: nostr.Filter{
				Kinds: []int{1},
				Tags:  nostr.TagMap{"#algo": []string{"wot-ranked"}},
			},
			expected: AlgoWoTRanked,
		},
		{
			name: "With algo tag (no #)",
			filter: nostr.Filter{
				Kinds: []int{1},
				Tags:  nostr.TagMap{"algo": []string{"trending"}},
			},
			expected: AlgoTrending,
		},
		{
			name: "Invalid algo defaults to chronological",
			filter: nostr.Filter{
				Kinds: []int{1},
				Tags:  nostr.TagMap{"#algo": []string{"invalid"}},
			},
			expected: AlgoChronological,
		},
	}

	for _, tc := range tests {
		result := ExtractAlgorithmFromFilter(tc.filter)
		if result != tc.expected {
			t.Errorf("%s: ExtractAlgorithmFromFilter() = %q, expected %q", tc.name, result, tc.expected)
		}
	}
}

func TestContainsIgnoreCase(t *testing.T) {
	tests := []struct {
		s        string
		substr   string
		expected bool
	}{
		{"Hello World", "world", true},
		{"Hello World", "HELLO", true},
		{"Hello World", "foo", false},
		{"", "test", false},
		{"test", "", true},
		{"UPPERCASE", "case", true},
	}

	for _, tc := range tests {
		result := containsIgnoreCase(tc.s, tc.substr)
		if result != tc.expected {
			t.Errorf("containsIgnoreCase(%q, %q) = %v, expected %v", tc.s, tc.substr, result, tc.expected)
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.DefaultAlgorithm != AlgoChronological {
		t.Errorf("DefaultAlgorithm should be chronological")
	}

	// Weights should sum to approximately 1
	totalWeight := cfg.WoTWeight + cfg.EngagementWeight + cfg.RecencyWeight
	if totalWeight < 0.99 || totalWeight > 1.01 {
		t.Errorf("Weights should sum to 1, got %v", totalWeight)
	}

	if cfg.MaxEventsToScore <= 0 {
		t.Errorf("MaxEventsToScore should be positive")
	}
}

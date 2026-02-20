package algo

import (
	"context"
	"database/sql"
	"log"

	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"
)

// Handler provides algorithmic feed processing for the relay
type Handler struct {
	scorer *Scorer
	config *Config
}

// NewHandler creates a new algorithm handler
func NewHandler(cfg *Config, db *sql.DB, trustGetter TrustLevelGetter) *Handler {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	return &Handler{
		scorer: NewScorer(cfg, db, trustGetter),
		config: cfg,
	}
}

// WrapQueryEvents wraps a QueryEvents function to apply algorithmic ranking
func (h *Handler) WrapQueryEvents(originalQuery func(context.Context, nostr.Filter) (chan *nostr.Event, error)) func(context.Context, nostr.Filter) (chan *nostr.Event, error) {
	return func(ctx context.Context, filter nostr.Filter) (chan *nostr.Event, error) {
		// Extract algorithm from filter
		algo := ExtractAlgorithmFromFilter(filter)

		// If chronological (default), just pass through
		if algo == AlgoChronological {
			return originalQuery(ctx, filter)
		}

		// Remove the algo tag from filter before querying
		cleanFilter := removeAlgoTag(filter)

		// Execute original query
		sourceCh, err := originalQuery(ctx, cleanFilter)
		if err != nil {
			return nil, err
		}

		// Collect events for scoring
		var events []*nostr.Event
		for event := range sourceCh {
			events = append(events, event)
		}

		// Score and rank events
		// TODO: Get user preferences from context or session
		scored := h.scorer.ScoreEvents(ctx, events, algo, nil)

		// Create output channel with ranked events
		resultCh := make(chan *nostr.Event, len(scored))
		go func() {
			defer close(resultCh)
			for _, se := range scored {
				select {
				case resultCh <- se.Event:
				case <-ctx.Done():
					return
				}
			}
		}()

		return resultCh, nil
	}
}

// RegisterAlgorithmSupport registers algorithm support with a khatru relay
// This adds algorithm filtering capability to queries
func RegisterAlgorithmSupport(relay *khatru.Relay, cfg *Config, db *sql.DB, trustGetter TrustLevelGetter) *Handler {
	handler := NewHandler(cfg, db, trustGetter)

	if !cfg.Enabled {
		return handler
	}

	// Note: We can't easily wrap QueryEvents in khatru since it's a slice
	// Instead, we'll provide the handler for manual integration
	// The main.go should wrap its query handler with our WrapQueryEvents

	log.Printf("Algorithmic feeds enabled (default: %s)", cfg.DefaultAlgorithm)
	log.Printf("Algorithm weights: WoT=%.1f, Engagement=%.1f, Recency=%.1f",
		cfg.WoTWeight, cfg.EngagementWeight, cfg.RecencyWeight)

	return handler
}

// removeAlgoTag creates a copy of the filter without the algo tag
func removeAlgoTag(filter nostr.Filter) nostr.Filter {
	if filter.Tags == nil {
		return filter
	}

	// Create a new filter with the same values
	newFilter := nostr.Filter{
		IDs:     filter.IDs,
		Authors: filter.Authors,
		Kinds:   filter.Kinds,
		Since:   filter.Since,
		Until:   filter.Until,
		Limit:   filter.Limit,
		Search:  filter.Search,
	}

	// Copy tags without algo
	if len(filter.Tags) > 0 {
		newFilter.Tags = make(nostr.TagMap)
		for k, v := range filter.Tags {
			if k != "#algo" && k != "algo" {
				newFilter.Tags[k] = v
			}
		}
	}

	return newFilter
}

// GetScorer returns the underlying scorer for direct use
func (h *Handler) GetScorer() *Scorer {
	return h.scorer
}

// ScoreEventsForFeed is a convenience method for scoring events for RSS/Atom feeds
func (h *Handler) ScoreEventsForFeed(ctx context.Context, events []*nostr.Event, algo AlgorithmName) []*ScoredEvent {
	return h.scorer.ScoreEvents(ctx, events, algo, nil)
}

package wot

import (
	"context"
	"log"
	"math"
	"sync"
	"time"
)

const (
	// PageRank algorithm parameters
	dampingFactor   = 0.85
	maxIterations   = 100
	convergenceEps  = 1e-6
	minPageRankSize = 10 // Minimum nodes to run PageRank
)

// PageRankConfig holds configuration for PageRank computation
type PageRankConfig struct {
	// Enabled activates PageRank-based trust scoring
	Enabled bool
	// ComputeInterval is how often to recompute PageRank scores
	ComputeInterval time.Duration
	// OwnerBoost is an additional boost for the owner's PageRank
	OwnerBoost float64
	// TrustThresholds define score thresholds for trust levels
	TrustThresholds PageRankThresholds
}

// PageRankThresholds defines the score thresholds for trust levels
type PageRankThresholds struct {
	// FollowThreshold is the minimum PageRank to be considered "follow" level
	FollowThreshold float64
	// FollowOfFollowThreshold is the minimum for "follow-of-follow" level
	FollowOfFollowThreshold float64
}

// DefaultPageRankConfig returns sensible defaults
func DefaultPageRankConfig() *PageRankConfig {
	return &PageRankConfig{
		Enabled:         true,
		ComputeInterval: 1 * time.Hour,
		OwnerBoost:      2.0, // Owner gets 2x boost
		TrustThresholds: PageRankThresholds{
			FollowThreshold:         0.001,  // Top ~1% of scores
			FollowOfFollowThreshold: 0.0001, // Top ~10% of scores
		},
	}
}

// PageRankCalculator computes PageRank-based trust scores
type PageRankCalculator struct {
	store       *Store
	ownerPubkey string
	config      *PageRankConfig

	// In-memory PageRank scores (also cached externally)
	scores   map[string]float64
	scoresMu sync.RWMutex

	// Last computation time
	lastComputed time.Time

	// Stop channel for background computation
	stopCh chan struct{}
}

// NewPageRankCalculator creates a new PageRank calculator
func NewPageRankCalculator(store *Store, ownerPubkey string, cfg *PageRankConfig) *PageRankCalculator {
	if cfg == nil {
		cfg = DefaultPageRankConfig()
	}
	return &PageRankCalculator{
		store:       store,
		ownerPubkey: ownerPubkey,
		config:      cfg,
		scores:      make(map[string]float64),
		stopCh:      make(chan struct{}),
	}
}

// Start begins background PageRank computation
func (pr *PageRankCalculator) Start(ctx context.Context) {
	// Initial computation
	go pr.computePageRank(ctx)

	// Periodic recomputation
	go func() {
		ticker := time.NewTicker(pr.config.ComputeInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-pr.stopCh:
				return
			case <-ticker.C:
				pr.computePageRank(ctx)
			}
		}
	}()
}

// Stop halts background computation
func (pr *PageRankCalculator) Stop() {
	close(pr.stopCh)
}

// GetPageRank returns the PageRank score for a pubkey
func (pr *PageRankCalculator) GetPageRank(pubkey string) float64 {
	// Try external cache first via store
	if pr.store != nil && pr.store.extCache != nil {
		if score, ok := pr.store.extCache.GetPageRank(context.Background(), pubkey); ok {
			return score
		}
	}

	// Fall back to in-memory
	pr.scoresMu.RLock()
	score := pr.scores[pubkey]
	pr.scoresMu.RUnlock()

	return score
}

// GetTrustLevelFromPageRank converts a PageRank score to a trust level
func (pr *PageRankCalculator) GetTrustLevelFromPageRank(pubkey string) TrustLevel {
	// Owner is always level 0
	if pubkey == pr.ownerPubkey {
		return TrustLevelOwner
	}

	score := pr.GetPageRank(pubkey)

	// No PageRank data yet - fall back to unknown
	if score == 0 {
		return TrustLevelUnknown
	}

	// Apply thresholds
	if score >= pr.config.TrustThresholds.FollowThreshold {
		return TrustLevelFollow
	}
	if score >= pr.config.TrustThresholds.FollowOfFollowThreshold {
		return TrustLevelFollowOfFollow
	}

	return TrustLevelUnknown
}

// computePageRank runs the PageRank algorithm on the follow graph
func (pr *PageRankCalculator) computePageRank(ctx context.Context) {
	startTime := time.Now()

	// Get all follow relationships from the store
	graph, err := pr.store.GetAllFollows()
	if err != nil {
		log.Printf("PageRank: failed to get follow graph: %v", err)
		return
	}

	// Build the node set and adjacency lists
	nodes := make(map[string]bool)
	outLinks := make(map[string][]string) // follower -> list of followees
	inLinks := make(map[string][]string)  // followee -> list of followers

	for _, rel := range graph {
		nodes[rel.Follower] = true
		nodes[rel.Followee] = true
		outLinks[rel.Follower] = append(outLinks[rel.Follower], rel.Followee)
		inLinks[rel.Followee] = append(inLinks[rel.Followee], rel.Follower)
	}

	n := len(nodes)
	if n < minPageRankSize {
		log.Printf("PageRank: graph too small (%d nodes), skipping", n)
		return
	}

	// Initialize scores
	scores := make(map[string]float64)
	initialScore := 1.0 / float64(n)
	for node := range nodes {
		scores[node] = initialScore
	}

	// Give owner a boost in initial score
	if nodes[pr.ownerPubkey] {
		scores[pr.ownerPubkey] *= pr.config.OwnerBoost
	}

	// Iterate until convergence
	for iter := 0; iter < maxIterations; iter++ {
		newScores := make(map[string]float64)
		maxDiff := 0.0

		// Random jump probability
		randomJump := (1.0 - dampingFactor) / float64(n)

		for node := range nodes {
			// Sum contributions from all nodes linking to this node
			sum := 0.0
			for _, linker := range inLinks[node] {
				outDegree := len(outLinks[linker])
				if outDegree > 0 {
					sum += scores[linker] / float64(outDegree)
				}
			}

			newScores[node] = randomJump + dampingFactor*sum

			// Track convergence
			diff := math.Abs(newScores[node] - scores[node])
			if diff > maxDiff {
				maxDiff = diff
			}
		}

		scores = newScores

		// Check convergence
		if maxDiff < convergenceEps {
			log.Printf("PageRank: converged after %d iterations", iter+1)
			break
		}
	}

	// Normalize scores so max is 1.0 (for easier threshold comparison)
	maxScore := 0.0
	for _, score := range scores {
		if score > maxScore {
			maxScore = score
		}
	}
	if maxScore > 0 {
		for node := range scores {
			scores[node] /= maxScore
		}
	}

	// Update in-memory cache
	pr.scoresMu.Lock()
	pr.scores = scores
	pr.lastComputed = time.Now()
	pr.scoresMu.Unlock()

	// Update external cache
	if pr.store.extCache != nil {
		cacheTTL := pr.config.ComputeInterval * 2 // TTL is 2x compute interval
		if err := pr.store.extCache.SetPageRankBatch(ctx, scores, cacheTTL); err != nil {
			log.Printf("PageRank: failed to cache scores: %v", err)
		}
	}

	elapsed := time.Since(startTime)
	log.Printf("PageRank: computed %d scores in %v (max=%.6f)", len(scores), elapsed, maxScore)

	// Log some statistics
	pr.logStats(scores)
}

// logStats logs PageRank distribution statistics
func (pr *PageRankCalculator) logStats(scores map[string]float64) {
	// Count nodes by trust level
	levelCounts := make(map[TrustLevel]int)
	for pubkey := range scores {
		level := pr.GetTrustLevelFromPageRank(pubkey)
		levelCounts[level]++
	}

	log.Printf("PageRank distribution: owner=%d, follow=%d, follow2=%d, unknown=%d",
		levelCounts[TrustLevelOwner],
		levelCounts[TrustLevelFollow],
		levelCounts[TrustLevelFollowOfFollow],
		levelCounts[TrustLevelUnknown],
	)
}

// ForceRecompute triggers an immediate PageRank recomputation
func (pr *PageRankCalculator) ForceRecompute(ctx context.Context) {
	go pr.computePageRank(ctx)
}

// LastComputedAt returns when PageRank was last computed
func (pr *PageRankCalculator) LastComputedAt() time.Time {
	pr.scoresMu.RLock()
	defer pr.scoresMu.RUnlock()
	return pr.lastComputed
}

// ScoreCount returns the number of computed scores
func (pr *PageRankCalculator) ScoreCount() int {
	pr.scoresMu.RLock()
	defer pr.scoresMu.RUnlock()
	return len(pr.scores)
}

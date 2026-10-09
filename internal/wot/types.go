package wot

import "time"

// TrustLevel represents the trust level for a pubkey
type TrustLevel int

const (
	// TrustLevelPaid is an independent axis: the pubkey holds an active paid
	// membership and bypasses the follow graph entirely. On lapse the graph
	// takes over with no grace period (GetEffectiveTier returns Free the
	// moment tier_expires_at passes).
	TrustLevelPaid TrustLevel = -1

	// TrustLevelOwner is a trust root (trust level 0)
	TrustLevelOwner TrustLevel = 0
	// TrustLevelFollow is someone a trust root directly follows (trust level 1)
	TrustLevelFollow TrustLevel = 1
	// TrustLevelFollowOfFollow is a follow of a follow (trust level 2)
	TrustLevelFollowOfFollow TrustLevel = 2
	// TrustLevelUnknown is anyone else (trust level 3+)
	TrustLevelUnknown TrustLevel = 3
)

// String returns the human-readable name of the trust level
func (t TrustLevel) String() string {
	switch t {
	case TrustLevelPaid:
		return "paid"
	case TrustLevelOwner:
		return "owner"
	case TrustLevelFollow:
		return "follow"
	case TrustLevelFollowOfFollow:
		return "follow-of-follow"
	default:
		return "unknown"
	}
}

// TrustPolicy defines rate limits and requirements for a trust level
type TrustPolicy struct {
	// EventsPerSecond is the max events per second for this trust level
	EventsPerSecond int
	// RequirePoW if true, requires proof of work for this trust level
	RequirePoW bool
	// MinPoWDifficulty is the minimum PoW difficulty required (if RequirePoW is true)
	MinPoWDifficulty int
}

// FollowRelation represents a follow relationship from the database
type FollowRelation struct {
	Follower  string    `json:"follower"`
	Followee  string    `json:"followee"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CachedTrust stores a cached trust calculation for a pubkey
type CachedTrust struct {
	Pubkey     string     `json:"pubkey"`
	TrustLevel TrustLevel `json:"trust_level"`
	CachedAt   time.Time  `json:"cached_at"`
}

// Config holds WoT configuration
type Config struct {
	// Enabled turns on WoT filtering
	Enabled bool
	// OwnerPubkey is the relay owner's pubkey (trust level 0).
	// Kept for backward compatibility — if TrustRoots is empty, this pubkey
	// becomes the single trust root.
	OwnerPubkey string
	// TrustRoots is the set of pubkeys that anchor the trust graph (trust
	// level 0). For a single-owner relay this is one pubkey; for a hosted
	// multi-tenant deployment it includes the operator plus org admins.
	// When non-empty, OwnerPubkey is ignored.
	TrustRoots []string
	// Policies defines rate limits and requirements by trust level
	Policies map[TrustLevel]TrustPolicy
	// CacheTTL is how long to cache trust calculations
	CacheTTL time.Duration
	// MaxFollowDepth is the maximum depth for follow traversal (default 2)
	MaxFollowDepth int
	// AllowedPubkeys bypasses WoT requirements (treated as trusted)
	AllowedPubkeys []string

	// IsPaidMember checks whether a pubkey holds an active paid tier.
	// When it returns true the pubkey receives TrustLevelPaid, bypassing
	// the follow graph entirely (independent axis, not a floor within the
	// graph). The function should handle its own caching; it is called
	// once per trust computation (not on every event — results are cached
	// by the trust cache). Nil means no paid-membership integration.
	IsPaidMember func(pubkey string) bool

	// CollabKinds are live-collaboration kinds (CRDT sync updates and
	// presence heartbeats) that are rate-limited in their own per-pubkey
	// bucket instead of the general one. Collaborative editors publish one
	// event per keystroke, so sharing a bucket lets typing starve the
	// document's snapshot save.
	CollabKinds []int
	// CollabEventsPerSecond is the floor rate of the collab bucket at every
	// trust level: the bucket runs at max(level rate, this). 0 disables the
	// separate bucket and collab kinds share the general one.
	CollabEventsPerSecond int

	// PageRank settings (Tier 2)
	// UsePageRank enables PageRank-based trust scoring instead of simple follow distance
	UsePageRank bool
	// PageRankInterval is how often to recompute PageRank scores (default 1h)
	PageRankInterval time.Duration
}

// DefaultPolicies returns sensible default policies
func DefaultPolicies() map[TrustLevel]TrustPolicy {
	return map[TrustLevel]TrustPolicy{
		// Paid members bypass the graph — treat them like direct follows
		// (no PoW, generous rate limit). They are paying for the relay;
		// gating them on graph distance defeats the point.
		TrustLevelPaid: {
			EventsPerSecond:  50,
			RequirePoW:       false,
			MinPoWDifficulty: 0,
		},
		TrustLevelOwner: {
			EventsPerSecond:  100,
			RequirePoW:       false,
			MinPoWDifficulty: 0,
		},
		TrustLevelFollow: {
			EventsPerSecond:  50,
			RequirePoW:       false,
			MinPoWDifficulty: 0,
		},
		TrustLevelFollowOfFollow: {
			EventsPerSecond:  20,
			RequirePoW:       false,
			MinPoWDifficulty: 0,
		},
		// Default: no PoW. The override at cmd/relay/main.go sets RequirePoW=true
		// when WOT_UNKNOWN_POW_BITS > 0. Setting the env var to 0 (or omitting it)
		// means "no proof of work", not "keep the compiled-in default of 8". That
		// was the landmine: the > 0 guard skipped the override, leaving RequirePoW
		// true at difficulty 8, so "zero" meant the opposite of what it read.
		TrustLevelUnknown: {
			EventsPerSecond:  5,
			RequirePoW:       false,
			MinPoWDifficulty: 0,
		},
	}
}

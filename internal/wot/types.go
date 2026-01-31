package wot

import "time"

// TrustLevel represents the trust level for a pubkey
type TrustLevel int

const (
	// TrustLevelOwner is the relay owner (trust level 0)
	TrustLevelOwner TrustLevel = 0
	// TrustLevelFollow is someone the owner directly follows (trust level 1)
	TrustLevelFollow TrustLevel = 1
	// TrustLevelFollowOfFollow is a follow of a follow (trust level 2)
	TrustLevelFollowOfFollow TrustLevel = 2
	// TrustLevelUnknown is anyone else (trust level 3+)
	TrustLevelUnknown TrustLevel = 3
)

// String returns the human-readable name of the trust level
func (t TrustLevel) String() string {
	switch t {
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
	// OwnerPubkey is the relay owner's pubkey (trust level 0)
	OwnerPubkey string
	// Policies defines rate limits and requirements by trust level
	Policies map[TrustLevel]TrustPolicy
	// CacheTTL is how long to cache trust calculations
	CacheTTL time.Duration
	// MaxFollowDepth is the maximum depth for follow traversal (default 2)
	MaxFollowDepth int
}

// DefaultPolicies returns sensible default policies
func DefaultPolicies() map[TrustLevel]TrustPolicy {
	return map[TrustLevel]TrustPolicy{
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
		TrustLevelUnknown: {
			EventsPerSecond:  5,
			RequirePoW:       true,
			MinPoWDifficulty: 8,
		},
	}
}

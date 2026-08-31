// Package arbiterclaim implements exclusive task claims for the arbiter fleet.
//
// Multiple agent sessions may attempt the same task concurrently. A claim is a
// NIP-78 application-specific event (kind 30078) addressed by
// d="arbiter:claim:<task-id>", and the relay enforces that exactly one claimant
// wins.
//
// Two layers, deliberately:
//
//   - The CLIENT protocol (advisory lease, settle window, lowest-event-id
//     tie-break) is correct on its own and works against any NIP-01 relay. It is
//     what makes this a protocol rather than a property of one relay.
//   - The RELAY layer (RejectEvent plus a conditional INSERT) makes it atomic
//     rather than advisory, using Postgres as the serialization point that two
//     replicas already share.
//
// Exclusion alone is an efficiency measure, not a safety property. The winning
// claim's event id is a FENCING TOKEN, and the effect site must verify it before
// acting; otherwise a slow loser can still act after losing.
package arbiterclaim

import (
	"sort"
	"strings"

	"github.com/nbd-wtf/go-nostr"
)

const (
	// KindClaim is NIP-78 application-specific data. It is SHARED: WoT settings,
	// HAVEN settings, stash file metadata and third-party clients (Amethyst,
	// cloistr docs/sheets/slides) all publish this kind to this relay.
	//
	// That is why IsClaim must be cheap and must be the FIRST branch of the
	// handler. A claim handler in the write path of kind 30078 is in the write
	// path of stash; a bug here does not fail arbiter claims, it fails stash.
	KindClaim = 30078

	// DTagPrefix namespaces claims inside kind 30078. Verified against live
	// relay traffic: of 94 distinct d-tags observed on kind 30078, none begins
	// with "arbiter" and none even contains a colon.
	DTagPrefix = "arbiter:claim:"

	// TagMarker is a redundant marker tag. It is NOT the discriminator -- other
	// 30078 publishers already use "t" tags, and more importantly, discriminating
	// on it would let a claimant BYPASS enforcement simply by omitting the tag.
	// The d-tag prefix is the discriminator; a claim missing this marker is
	// malformed and rejected rather than waved through.
	TagMarker = "arbiter-claim"
)

// Claim is a parsed arbiter claim.
type Claim struct {
	TaskID   string // task being claimed
	EventID  string // the claim event's id -- this is the fencing token
	Claimant string // pubkey of the claiming role
	At       nostr.Timestamp
}

// IsClaim reports whether an event is in the arbiter claim namespace.
//
// This is the hot-path discriminator. It performs no I/O and must stay that way:
// every WoT settings write, HAVEN settings write and stash metadata write on this
// relay passes through it, and none of them may reach the database because of it.
func IsClaim(evt *nostr.Event) bool {
	if evt == nil || evt.Kind != KindClaim {
		return false
	}
	return strings.HasPrefix(tagValue(evt, "d"), DTagPrefix)
}

// TaskIDOf returns the task id a claim addresses, or "" if the event is not a
// well-formed claim for one.
func TaskIDOf(evt *nostr.Event) string {
	if !IsClaim(evt) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(tagValue(evt, "d"), DTagPrefix))
}

// Validate checks a claim's shape. It is only ever called for events IsClaim has
// already accepted, so a failure here means a malformed claim -- which is
// rejected, never ignored.
func Validate(evt *nostr.Event) (Claim, error) {
	taskID := TaskIDOf(evt)
	if taskID == "" {
		return Claim{}, ErrMalformed{Reason: "empty task id after " + DTagPrefix}
	}
	if !hasTag(evt, "t", TagMarker) {
		return Claim{}, ErrMalformed{Reason: "missing t=" + TagMarker + " marker tag"}
	}
	if evt.ID == "" {
		return Claim{}, ErrMalformed{Reason: "event has no id"}
	}
	return Claim{TaskID: taskID, EventID: evt.ID, Claimant: evt.PubKey, At: evt.CreatedAt}, nil
}

// ErrMalformed is returned for events in the claim namespace that are not
// well-formed claims.
type ErrMalformed struct{ Reason string }

func (e ErrMalformed) Error() string { return "malformed arbiter claim: " + e.Reason }

// Winner picks the winning claim among concurrent claims for one task.
//
// The tie-break is the LOWEST EVENT ID, never created_at. created_at is
// self-asserted and clocks skew, so a tie-break on created_at is a tie-break on
// whoever lies best. The event id is a SHA-256 over the serialized event:
// deterministic, and identical for every reader.
func Winner(claims []Claim) (Claim, bool) {
	if len(claims) == 0 {
		return Claim{}, false
	}
	sorted := make([]Claim, len(claims))
	copy(sorted, claims)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].EventID < sorted[j].EventID })
	return sorted[0], true
}

// BuildClaimEvent constructs an unsigned claim event for a task. The caller signs
// it with the ROLE key (see DeriveRoleKey) and publishes it to exactly ONE relay:
// fanning claims out across relays reintroduces the split-brain this prevents.
func BuildClaimEvent(taskID string, at nostr.Timestamp) *nostr.Event {
	return &nostr.Event{
		Kind:      KindClaim,
		CreatedAt: at,
		Tags: nostr.Tags{
			nostr.Tag{"d", DTagPrefix + taskID},
			nostr.Tag{"t", TagMarker},
		},
		Content: "",
	}
}

func tagValue(evt *nostr.Event, name string) string {
	for _, t := range evt.Tags {
		if len(t) >= 2 && t[0] == name {
			return t[1]
		}
	}
	return ""
}

func hasTag(evt *nostr.Event, name, value string) bool {
	for _, t := range evt.Tags {
		if len(t) >= 2 && t[0] == name && t[1] == value {
			return true
		}
	}
	return false
}

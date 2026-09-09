package groups

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"github.com/nbd-wtf/go-nostr"
)

// ownerPattern matches the Space group identifier format:
//
//	{slug}-{16-hex-pubkey-prefix}-{8-hex-random}
//
// The captured group is the 16-character hex prefix of the owner's pubkey.
// Groups created before this scheme have no pubkey in their d-tag and will
// not match; those are allowed through (see RejectExternalMetadata).
var ownerPattern = regexp.MustCompile(`^[a-z0-9-]+-([0-9a-f]{16})-[0-9a-f]{8}$`)

// EventQuerier is the subset of eventstore.Store needed by the ownership
// resolver. Accepting this interface instead of the full store keeps the
// groups package decoupled from PostgreSQL.
type EventQuerier interface {
	QueryEvents(context.Context, nostr.Filter) (chan *nostr.Event, error)
}

// maxTransferDepth bounds the transfer chain walk. Matches the Space client's
// MAX_TRANSFER_DEPTH (ownership.ts). A real chain will never approach this;
// the limit exists to cap adversarial event crafting.
const maxTransferDepth = 20

// memberWritePermissions are the admin-list permissions that grant a delegated
// admin the right to publish kind 39002 (member list) events.
var memberWritePermissions = map[string]bool{
	"add-user":    true,
	"remove-user": true,
}

// ResolveOwner determines the current owner of a group by:
//  1. Extracting the 16-hex pubkey prefix from the d-tag (Space identifier format).
//  2. Querying the store for kind 39000 events on that d-tag.
//  3. Walking the transfer chain: each owner's latest 39000 may carry a
//     ["transfer-to", successorPubkey] tag, which moves ownership forward.
//
// For new-format d-tags ({slug}-{16-hex-prefix}-{8-hex-random}), the genesis
// owner is identified by prefix match. For legacy d-tags (no embedded prefix),
// the genesis owner is the author of the earliest kind 39000 event in the store.
//
// Returns ("", nil) only when no ownership can be determined: legacy d-tags
// with no kind 39000 events in the store.
// Returns the resolved owner pubkey on success.
func ResolveOwner(ctx context.Context, q EventQuerier, dTag string) (string, error) {
	match := ownerPattern.FindStringSubmatch(dTag)

	// Fetch all kind 39000 events for this d-tag.
	events, err := queryGroupEvents(ctx, q, KindGroupMetadata, dTag)
	if err != nil {
		return "", err
	}

	if match != nil {
		// New-format d-tag: genesis owner prefix is embedded.
		prefix := match[1]

		// Find the genesis owner: the pubkey whose first 16 hex chars match
		// the prefix embedded in the d-tag.
		currentOwner := ""
		for _, evt := range events {
			if len(evt.PubKey) >= 16 && evt.PubKey[:16] == prefix {
				currentOwner = evt.PubKey
				break
			}
		}
		if currentOwner == "" {
			// No event from the genesis owner exists yet. The d-tag embeds a
			// prefix, so the only pubkey that can legitimately publish is one
			// matching that prefix (the genesis case, before any 39000 is stored).
			// Return the prefix as a partial match signal: callers compare it
			// against the event author's prefix.
			return "prefix:" + prefix, nil
		}

		return walkTransferChain(events, currentOwner), nil
	}

	// Legacy d-tag: no embedded pubkey prefix. Derive the genesis owner from
	// the earliest kind 39000 event in the store.
	if len(events) == 0 {
		return "", nil // no ownership data available
	}

	genesis := earliestEvent(events)
	return walkTransferChain(events, genesis.PubKey), nil
}

// walkTransferChain follows transfer-to tags from currentOwner through the
// event list, returning the final owner pubkey.
func walkTransferChain(events []*nostr.Event, currentOwner string) string {
	visited := map[string]bool{currentOwner: true}
	for depth := 0; depth < maxTransferDepth; depth++ {
		latest := latestEventFrom(events, currentOwner)
		if latest == nil {
			break
		}
		successor := getTagValue(latest, "transfer-to")
		if successor == "" {
			break
		}
		if visited[successor] {
			break // cycle
		}
		visited[successor] = true
		currentOwner = successor
	}
	return currentOwner
}

// earliestEvent returns the event with the lowest created_at timestamp.
// On ties, the lowest event ID wins (deterministic).
func earliestEvent(events []*nostr.Event) *nostr.Event {
	if len(events) == 0 {
		return nil
	}
	earliest := events[0]
	for _, evt := range events[1:] {
		if evt.CreatedAt < earliest.CreatedAt {
			earliest = evt
		} else if evt.CreatedAt == earliest.CreatedAt && evt.ID < earliest.ID {
			earliest = evt
		}
	}
	return earliest
}

// IsAuthorizedMetadataWriter checks whether pubkey is allowed to publish
// the given metadata kind for the group identified by dTag.
//
// Rules (matching Space's trustedWriters.ts):
//   - 39000 (group metadata), 39001 (admin list): current owner only.
//   - 39002 (member list): current owner, or a delegated admin whose
//     add-user or remove-user permission appears in the latest owner-signed 39001.
//   - 39003-39009: current owner only (conservative default).
func IsAuthorizedMetadataWriter(ctx context.Context, q EventQuerier, dTag string, kind int, pubkey string) (bool, error) {
	owner, err := ResolveOwner(ctx, q, dTag)
	if err != nil {
		return false, err
	}

	// No owner resolved. For legacy d-tags this means no kind 39000 events
	// exist in the store, so there is no ownership to verify against. Reject:
	// new groups should use the new d-tag format, and existing legacy groups
	// already have 39000 events that establish ownership.
	if owner == "" {
		return false, nil
	}

	// Genesis case: no 39000 in the store yet, but the d-tag embeds the
	// creator's prefix. Match on prefix alone.
	if strings.HasPrefix(owner, "prefix:") {
		prefix := owner[7:]
		return len(pubkey) >= 16 && pubkey[:16] == prefix, nil
	}

	// Direct owner match covers all metadata kinds.
	if pubkey == owner {
		return true, nil
	}

	// For kind 39002, check delegated admin permissions.
	if kind == KindGroupMembers {
		return isDelegatedMemberWriter(ctx, q, dTag, pubkey, owner)
	}

	return false, nil
}

// isDelegatedMemberWriter checks whether pubkey has add-user or remove-user
// permission in the latest owner-signed kind 39001 for this group.
func isDelegatedMemberWriter(ctx context.Context, q EventQuerier, dTag string, pubkey string, owner string) (bool, error) {
	events, err := queryGroupEvents(ctx, q, KindGroupAdmins, dTag)
	if err != nil {
		return false, err
	}

	// Find the latest 39001 signed by the current owner.
	adminEvent := latestEventFrom(events, owner)
	if adminEvent == nil {
		return false, nil // no admin list from the owner
	}

	// Parse the admin list. Two tag formats exist in the wild:
	//   NIP-29 spec: ["p", pubkey, role, permission1, permission2, ...]
	//   Space:       ["p", pubkey, permission1, permission2, ...]
	// Space's buildAdminTags (permissions.ts:172) omits the role label,
	// so permissions start at index 2. The scan below covers both formats
	// by checking every element from index 2 onward.
	for _, tag := range adminEvent.Tags {
		if len(tag) < 2 || tag[0] != "p" {
			continue
		}
		if tag[1] != pubkey {
			continue
		}
		for i := 2; i < len(tag); i++ {
			if memberWritePermissions[tag[i]] {
				return true, nil
			}
		}
	}

	return false, nil
}

// queryGroupEvents fetches all events of the given kind for a d-tag.
func queryGroupEvents(ctx context.Context, q EventQuerier, kind int, dTag string) ([]*nostr.Event, error) {
	filter := nostr.Filter{
		Kinds: []int{kind},
		Tags:  nostr.TagMap{"d": []string{dTag}},
		Limit: 100, // groups won't have more than this
	}
	ch, err := q.QueryEvents(ctx, filter)
	if err != nil {
		return nil, err
	}
	var events []*nostr.Event
	for evt := range ch {
		events = append(events, evt)
	}
	return events, nil
}

// latestEventFrom returns the most recent event signed by pubkey, using
// created_at descending and event ID as a deterministic tiebreak (matching
// Space's ownership.ts).
func latestEventFrom(events []*nostr.Event, pubkey string) *nostr.Event {
	var candidates []*nostr.Event
	for _, evt := range events {
		if evt.PubKey == pubkey {
			candidates = append(candidates, evt)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].CreatedAt != candidates[j].CreatedAt {
			return candidates[i].CreatedAt > candidates[j].CreatedAt
		}
		return candidates[i].ID > candidates[j].ID
	})
	return candidates[0]
}

// getTagValue returns the value of the first tag with the given name,
// or "" if not found.
func getTagValue(event *nostr.Event, tagName string) string {
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == tagName {
			return tag[1]
		}
	}
	return ""
}

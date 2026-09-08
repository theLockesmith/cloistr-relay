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
// Returns ("", nil) for legacy d-tags with no embedded prefix.
// Returns the resolved owner pubkey on success.
func ResolveOwner(ctx context.Context, q EventQuerier, dTag string) (string, error) {
	match := ownerPattern.FindStringSubmatch(dTag)
	if match == nil {
		return "", nil // legacy identifier, no derivable owner
	}
	prefix := match[1]

	// Fetch all kind 39000 events for this d-tag.
	events, err := queryGroupEvents(ctx, q, KindGroupMetadata, dTag)
	if err != nil {
		return "", err
	}

	// Find the genesis owner: the pubkey whose first 16 hex chars match the
	// prefix embedded in the d-tag.
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

	// Walk the transfer chain.
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

	return currentOwner, nil
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

	// Legacy d-tag: no derivable owner, allow through.
	if owner == "" {
		return true, nil
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

	// Parse the admin list. NIP-29 39001 uses "p" tags:
	//   ["p", pubkey, role, permission1, permission2, ...]
	for _, tag := range adminEvent.Tags {
		if len(tag) < 2 || tag[0] != "p" {
			continue
		}
		if tag[1] != pubkey {
			continue
		}
		// Check permissions starting at index 3 (index 2 is the role label).
		for i := 3; i < len(tag); i++ {
			if memberWritePermissions[tag[i]] {
				return true, nil
			}
		}
		// Also check index 2 in case the role label is omitted and permissions
		// start immediately (defensive).
		if len(tag) >= 3 && memberWritePermissions[tag[2]] {
			return true, nil
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

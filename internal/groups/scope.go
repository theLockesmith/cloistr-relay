package groups

import (
	"context"

	"github.com/fiatjaf/khatru"
	"github.com/fiatjaf/relay29"
	"github.com/nbd-wtf/go-nostr"
)

// WireConfig carries the Cloistr policy hooks relay29 is attached with.
type WireConfig struct {
	AdminPubkeys        []string
	AllowPublicCreation bool
}

// Wire attaches relay29's NIP-29 handlers to a SHARED relay.
//
// relay29 is written for a groups-only relay: appended as-is, its rules refuse
// every event without an h tag and every REQ without h/e/a/ids, and one of its
// save hooks dereferences a nil group on any event outside a group. Wire
// scopes every relay29 handler to NIP-29 traffic and adds the privacy checks
// relay29 only performs on group-shaped REQs, so a private group's events
// cannot come back through a plain REQ, a COUNT, or a live subscription.
//
// NIP-29 traffic is a NIP-29 kind (9000-9022 moderation/join/leave, 39000-39003
// metadata) or an event whose h tag names a group relay29 knows. An h tag alone
// is not enough: gift-wrap thread buckets use h tags too.
//
// Call Wire after every other QueryEvents and CountEvents handler is
// registered: it wraps the ones already on r, and a handler appended later
// would serve private group events unfiltered.
//
// st must already have Relay and GetAuthed set.
//
// Known limitation (relay29 v0.5.1): group membership and the private flag
// are read without relay29's unexported per-group lock, the same way
// relay29's own query handlers read them.
func Wire(r *khatru.Relay, st *relay29.State, cfg WireConfig) {
	authed := st.GetAuthed

	base := r.QueryEvents
	r.QueryEvents = make([]func(context.Context, nostr.Filter) (chan *nostr.Event, error), 0, len(base)+5)
	for _, q := range base {
		r.QueryEvents = append(r.QueryEvents, ordinaryQuery(st, authed, q))
	}
	for _, q := range []func(context.Context, nostr.Filter) (chan *nostr.Event, error){
		st.NormalEventQuery,
		st.MetadataQueryHandler,
		st.AdminsQueryHandler,
		st.MembersQueryHandler,
		st.RolesQueryHandler,
	} {
		r.QueryEvents = append(r.QueryEvents, groupQuery(st, q))
	}

	baseCount := r.CountEvents
	r.CountEvents = make([]func(context.Context, nostr.Filter) (int64, error), 0, len(baseCount))
	for _, c := range baseCount {
		r.CountEvents = append(r.CountEvents, privateAwareCount(st, authed, c))
	}

	r.RejectFilter = append(r.RejectFilter,
		groupFilterOnly(st, st.RequireKindAndSingleGroupIDOrSpecificEventReference),
	)

	r.RejectEvent = append(r.RejectEvent,
		groupEventOnly(st, st.RequireHTagForExistingGroup),
		groupEventOnly(st, st.RequireModerationEventsToBeRecent),
		groupEventOnly(st, st.RestrictWritesBasedOnGroupRules),
		groupEventOnly(st, GroupCreationGate(cfg.AdminPubkeys, cfg.AllowPublicCreation)),
		groupEventOnly(st, st.RestrictInvalidModerationActions),
		groupEventOnly(st, st.PreventWritingOfEventsJustDeleted),
		groupEventOnly(st, st.CheckPreviousTag),
	)

	r.OnEventSaved = append(r.OnEventSaved,
		savedGroupEventOnly(st, st.ApplyModerationAction),
		savedGroupEventOnly(st, st.ReactToJoinRequest),
		savedGroupEventOnly(st, st.ReactToLeaveRequest),
		// last50 tracking needs the group to exist; skip events outside one.
		func(ctx context.Context, evt *nostr.Event) {
			if knownGroupEvent(st, evt) {
				st.AddToPreviousChecking(ctx, evt)
			}
		},
	)

	r.PreventBroadcast = append(r.PreventBroadcast, func(ws *khatru.WebSocket, evt *nostr.Event) bool {
		return !visible(st, evt, ws.AuthedPublicKey)
	})
}

func isGroupKind(kind int) bool {
	return (kind >= 9000 && kind <= 9022) || isMetadataKind(kind)
}

func isMetadataKind(kind int) bool {
	return kind >= nostr.KindSimpleGroupMetadata && kind <= nostr.KindSimpleGroupRoles
}

func knownGroup(st *relay29.State, id string) *relay29.Group {
	g, _ := st.Groups.Load(id)
	return g
}

func knownGroupEvent(st *relay29.State, evt *nostr.Event) bool {
	for _, tag := range evt.Tags {
		if len(tag) >= 2 && tag[0] == "h" && knownGroup(st, tag[1]) != nil {
			return true
		}
	}
	return false
}

func isGroupEvent(st *relay29.State, evt *nostr.Event) bool {
	return isGroupKind(evt.Kind) || knownGroupEvent(st, evt)
}

// isGroupFilter reports whether a REQ is addressed to NIP-29: it asks for
// group metadata kinds, or its #h names a group relay29 knows.
func isGroupFilter(st *relay29.State, f nostr.Filter) bool {
	for _, k := range f.Kinds {
		if isMetadataKind(k) {
			return true
		}
	}
	for _, id := range f.Tags["h"] {
		if knownGroup(st, id) != nil {
			return true
		}
	}
	return false
}

func canRead(st *relay29.State, groupID, pubkey string) bool {
	g := knownGroup(st, groupID)
	if g == nil || !g.Private {
		return true
	}
	if pubkey == "" {
		return false
	}
	_, member := g.Members[pubkey]
	return member
}

// visible reports whether pubkey may see evt: false only when one of its h
// tags names a private group pubkey is not a member of.
func visible(st *relay29.State, evt *nostr.Event, pubkey string) bool {
	for _, tag := range evt.Tags {
		if len(tag) >= 2 && tag[0] == "h" && !canRead(st, tag[1], pubkey) {
			return false
		}
	}
	return true
}

// hiddenGroups lists the private groups pubkey cannot read.
func hiddenGroups(st *relay29.State, pubkey string) []string {
	var ids []string
	for id, g := range st.Groups.Range {
		if !g.Private {
			continue
		}
		if pubkey != "" {
			if _, member := g.Members[pubkey]; member {
				continue
			}
		}
		ids = append(ids, id)
	}
	return ids
}

func groupEventOnly(st *relay29.State, fn func(context.Context, *nostr.Event) (bool, string)) func(context.Context, *nostr.Event) (bool, string) {
	return func(ctx context.Context, evt *nostr.Event) (bool, string) {
		if !isGroupEvent(st, evt) {
			return false, ""
		}
		return fn(ctx, evt)
	}
}

func savedGroupEventOnly(st *relay29.State, fn func(context.Context, *nostr.Event)) func(context.Context, *nostr.Event) {
	return func(ctx context.Context, evt *nostr.Event) {
		if isGroupEvent(st, evt) {
			fn(ctx, evt)
		}
	}
}

func groupFilterOnly(st *relay29.State, fn func(context.Context, nostr.Filter) (bool, string)) func(context.Context, nostr.Filter) (bool, string) {
	return func(ctx context.Context, f nostr.Filter) (bool, string) {
		if !isGroupFilter(st, f) {
			return false, ""
		}
		return fn(ctx, f)
	}
}

// closedChan is the "no results" answer. A nil channel is not safe: khatru's
// own write path (adding.go, the deletion check) reads one value from every
// QueryEvents handler without a nil check, and a nil channel blocks forever.
func closedChan() chan *nostr.Event {
	ch := make(chan *nostr.Event)
	close(ch)
	return ch
}

// groupQuery serves only NIP-29 REQs. Everything else is left to the
// ordinary store, so no event comes back twice.
func groupQuery(st *relay29.State, q func(context.Context, nostr.Filter) (chan *nostr.Event, error)) func(context.Context, nostr.Filter) (chan *nostr.Event, error) {
	return func(ctx context.Context, f nostr.Filter) (chan *nostr.Event, error) {
		if !isGroupFilter(st, f) {
			return closedChan(), nil
		}
		return q(ctx, f)
	}
}

// ordinaryQuery serves every non-NIP-29 REQ from the store and drops events
// that belong to a private group the reader is not in.
func ordinaryQuery(st *relay29.State, authed func(context.Context) string, q func(context.Context, nostr.Filter) (chan *nostr.Event, error)) func(context.Context, nostr.Filter) (chan *nostr.Event, error) {
	return func(ctx context.Context, f nostr.Filter) (chan *nostr.Event, error) {
		if isGroupFilter(st, f) {
			return closedChan(), nil
		}
		src, err := q(ctx, f)
		if err != nil || src == nil {
			return src, err
		}
		reader := authed(ctx)
		out := make(chan *nostr.Event)
		go func() {
			defer close(out)
			for evt := range src {
				if !visible(st, evt, reader) {
					continue
				}
				select {
				case out <- evt:
				case <-ctx.Done():
					for range src {
					}
					return
				}
			}
		}()
		return out, nil
	}
}

// privateAwareCount subtracts the private-group events the reader cannot see
// from an ordinary COUNT, and returns 0 for a group COUNT the reader may not
// read.
func privateAwareCount(st *relay29.State, authed func(context.Context) string, c func(context.Context, nostr.Filter) (int64, error)) func(context.Context, nostr.Filter) (int64, error) {
	return func(ctx context.Context, f nostr.Filter) (int64, error) {
		reader := authed(ctx)
		if hs := f.Tags["h"]; len(hs) > 0 {
			for _, id := range hs {
				if !canRead(st, id, reader) {
					return 0, nil
				}
			}
			return c(ctx, f)
		}

		n, err := c(ctx, f)
		if err != nil {
			return n, err
		}
		hidden := hiddenGroups(st, reader)
		if len(hidden) == 0 {
			return n, nil
		}
		masked := f.Clone()
		if masked.Tags == nil {
			masked.Tags = nostr.TagMap{}
		}
		masked.Tags["h"] = hidden
		m, err := c(ctx, masked)
		if err != nil {
			return n, err
		}
		if m > n {
			return 0, nil
		}
		return n - m, nil
	}
}

// Package groups provides NIP-29 group authorization logic.
//
// The relay's NIP-29 support is powered by the relay29 library. This package
// holds the authorization policy that relay29 delegates to via its AllowAction
// callback, and the group-creation gate that restricts who can create groups.
package groups

import (
	"context"

	"github.com/fiatjaf/relay29"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip29"
)

// Roles used by the relay. Pointer identity is the comparison contract:
// relay29 compares role pointers when checking membership, so these must be the
// same objects passed to relay29.Options.DefaultRoles and
// GroupCreatorDefaultRole. Never construct a &nip29.Role{Name: "admin"}
// elsewhere; use these package-level variables.
var (
	AdminRole     = &nip29.Role{Name: "admin", Description: "group administrator with full control"}
	ModeratorRole = &nip29.Role{Name: "moderator", Description: "can delete events and remove users"}

	// Roles is the set of roles every group advertises (kind 39003).
	Roles = []*nip29.Role{AdminRole, ModeratorRole}
)

// allowActionCalledKey is a context key used to detect whether relay29's
// RestrictInvalidModerationActions actually invoked AllowAction. When a member
// has no roles, relay29's for-range loop is empty and AllowAction is never
// called — see PermitRolelessPutUser.
type allowActionCalledKey struct{}

// AllowAction is the relay29 AllowAction callback. It answers: "given this
// member's role in this group, may they perform this moderation action?"
//
// relay29 calls this once per role in the member's role slice. Members with no
// roles (nil/empty slice) never reach this function; PermitRolelessPutUser
// handles the PutUser case for them.
//
// Policy:
//   - Admin: everything (including group deletion).
//   - Moderator: delete events, remove users, invite new users.
func AllowAction(_ context.Context, _ nip29.Group, role *nip29.Role, action relay29.Action) bool {
	if role == AdminRole {
		return true
	}
	if role == ModeratorRole {
		switch action.(type) {
		case relay29.DeleteEvent:
			return true
		case relay29.RemoveUser:
			return true
		case relay29.PutUser:
			return true
		}
	}
	return false
}

// TrackingAllowAction wraps AllowAction to record in the context whether it
// was invoked. Assign this to groupsState.AllowAction (not AllowAction
// directly) when using PermitRolelessPutUser.
func TrackingAllowAction(ctx context.Context, group nip29.Group, role *nip29.Role, action relay29.Action) bool {
	if marker, ok := ctx.Value(allowActionCalledKey{}).(*bool); ok {
		*marker = true
	}
	return AllowAction(ctx, group, role, action)
}

// PermitRolelessPutUser wraps relay29's RestrictInvalidModerationActions to
// allow roleless group members to perform PutUser (invite other users).
//
// The problem: relay29 calls AllowAction once per role in the member's role
// slice. Members with no roles skip the loop entirely and are rejected with
// "insufficient permissions" without AllowAction ever seeing them. khatru's
// RejectEvent chain is "reject if any handler says yes" — a separate handler
// running before RestrictInvalidModerationActions cannot un-reject what it
// will reject later. So this function replaces RestrictInvalidModerationActions
// in the handler chain, delegates to it, and overrides the result only when
// AllowAction was never called and the action is PutUser.
//
// groupsState.AllowAction must be set to TrackingAllowAction for the detection
// to work. Non-members never reach this point: RestrictWritesBasedOnGroupRules
// runs earlier and rejects them.
func PermitRolelessPutUser(state *relay29.State) func(context.Context, *nostr.Event) (bool, string) {
	return func(ctx context.Context, event *nostr.Event) (bool, string) {
		var called bool
		markerCtx := context.WithValue(ctx, allowActionCalledKey{}, &called)

		reject, msg := state.RestrictInvalidModerationActions(markerCtx, event)
		if !reject {
			return false, ""
		}

		// AllowAction ran and explicitly denied — respect the rejection.
		if called {
			return true, msg
		}

		// AllowAction was never called. The member has no roles, so relay29's
		// for-range loop had nothing to iterate. Allow PutUser for roleless
		// members; deny everything else.
		if event.Kind == nostr.KindSimpleGroupPutUser {
			return false, ""
		}

		return true, msg
	}
}

// GroupCreationGate returns a khatru RejectEvent handler that restricts group
// creation (kind 9007) to the listed admin pubkeys. When allowPublic is true,
// any authenticated user can create groups and the admin list is ignored.
func GroupCreationGate(adminPubkeys []string, allowPublic bool) func(context.Context, *nostr.Event) (bool, string) {
	admins := make(map[string]bool, len(adminPubkeys))
	for _, pk := range adminPubkeys {
		admins[pk] = true
	}

	return func(_ context.Context, event *nostr.Event) (bool, string) {
		if event.Kind != nostr.KindSimpleGroupCreateGroup {
			return false, ""
		}
		if allowPublic {
			return false, ""
		}
		if admins[event.PubKey] {
			return false, ""
		}
		return true, "restricted: only designated admins can create groups"
	}
}

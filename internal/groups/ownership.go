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

// Roles used by the relay. Pointer identity matters: relay29 compares role
// pointers when checking membership, so these must be the same objects passed
// to relay29.Options.DefaultRoles and GroupCreatorDefaultRole.
var (
	AdminRole     = &nip29.Role{Name: "admin", Description: "group administrator with full control"}
	ModeratorRole = &nip29.Role{Name: "moderator", Description: "can delete events and remove users"}

	// Roles is the set of roles every group advertises (kind 39003).
	Roles = []*nip29.Role{AdminRole, ModeratorRole}
)

// AllowAction is the relay29 AllowAction callback. It answers: "given this
// member's role in this group, may they perform this moderation action?"
//
// Policy:
//   - Admin: everything.
//   - Moderator: delete events, remove users.
//   - Any member (including no role): invite new users (PutUser).
//   - Everything else: denied.
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
		}
	}
	// Any member can invite new users via PutUser.
	if _, ok := action.(relay29.PutUser); ok {
		return true
	}
	return false
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

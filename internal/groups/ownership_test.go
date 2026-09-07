package groups

import (
	"context"
	"testing"

	"github.com/fiatjaf/relay29"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip29"
)

var testGroup = nip29.Group{
	Address: nip29.GroupAddress{Relay: "wss://relay.test", ID: "test-group"},
}

func TestAllowAction_AdminCanDoEverything(t *testing.T) {
	actions := []relay29.Action{
		relay29.PutUser{Targets: []relay29.PubKeyRoles{{PubKey: "abc"}}},
		relay29.RemoveUser{Targets: []string{"abc"}},
		relay29.DeleteEvent{Targets: []string{"abc"}},
		relay29.EditMetadata{NameValue: ptrStr("new name")},
		relay29.DeleteGroup{},
	}

	for _, action := range actions {
		if !AllowAction(context.Background(), testGroup, AdminRole, action) {
			t.Errorf("admin should be allowed to perform %T, but was denied", action)
		}
	}
}

func TestAllowAction_ModeratorPermissions(t *testing.T) {
	allowed := []relay29.Action{
		relay29.DeleteEvent{Targets: []string{"abc"}},
		relay29.RemoveUser{Targets: []string{"abc"}},
		relay29.PutUser{Targets: []relay29.PubKeyRoles{{PubKey: "abc"}}},
	}
	for _, action := range allowed {
		if !AllowAction(context.Background(), testGroup, ModeratorRole, action) {
			t.Errorf("moderator should be allowed to perform %T, but was denied", action)
		}
	}

	denied := []relay29.Action{
		relay29.EditMetadata{NameValue: ptrStr("hijack")},
		relay29.DeleteGroup{},
	}
	for _, action := range denied {
		if AllowAction(context.Background(), testGroup, ModeratorRole, action) {
			t.Errorf("moderator should NOT be allowed to perform %T, but was allowed", action)
		}
	}
}

func TestAllowAction_RolelessMemberDeniedEverything(t *testing.T) {
	// relay29 never calls AllowAction for roleless members (nil/empty role
	// slice means the range loop is skipped). PermitRolelessPutUser handles
	// PutUser for them. AllowAction itself must deny nil-role callers so
	// there is no accidental permission escalation if the call path changes.
	actions := []relay29.Action{
		relay29.PutUser{Targets: []relay29.PubKeyRoles{{PubKey: "newuser"}}},
		relay29.RemoveUser{Targets: []string{"abc"}},
		relay29.DeleteEvent{Targets: []string{"abc"}},
		relay29.EditMetadata{NameValue: ptrStr("hijack")},
		relay29.DeleteGroup{},
	}
	for _, action := range actions {
		if AllowAction(context.Background(), testGroup, nil, action) {
			t.Errorf("roleless member (nil role) should NOT be allowed to perform %T via AllowAction, but was allowed", action)
		}
	}
}

func TestAllowAction_UnknownRoleDenied(t *testing.T) {
	strangerRole := &nip29.Role{Name: "stranger", Description: "not a real role"}
	actions := []relay29.Action{
		relay29.EditMetadata{NameValue: ptrStr("nope")},
		relay29.DeleteGroup{},
		relay29.PutUser{Targets: []relay29.PubKeyRoles{{PubKey: "abc"}}},
	}
	for _, action := range actions {
		if AllowAction(context.Background(), testGroup, strangerRole, action) {
			t.Errorf("unknown role should be denied %T, but was allowed", action)
		}
	}
}

func TestTrackingAllowAction_SetsContextFlag(t *testing.T) {
	var called bool
	ctx := context.WithValue(context.Background(), allowActionCalledKey{}, &called)

	// Call with AdminRole — should return true AND set the flag.
	result := TrackingAllowAction(ctx, testGroup, AdminRole, relay29.PutUser{Targets: []relay29.PubKeyRoles{{PubKey: "abc"}}})
	if !result {
		t.Error("TrackingAllowAction should return true for admin PutUser")
	}
	if !called {
		t.Error("TrackingAllowAction should set context flag when invoked")
	}
}

func TestTrackingAllowAction_FlagNotSetWithoutContextKey(t *testing.T) {
	// Without the context key, TrackingAllowAction should still work as AllowAction
	// and not panic.
	result := TrackingAllowAction(context.Background(), testGroup, AdminRole, relay29.DeleteGroup{})
	if !result {
		t.Error("TrackingAllowAction should return true for admin DeleteGroup even without context key")
	}
}

func TestGroupCreationGate_PublicCreation(t *testing.T) {
	gate := GroupCreationGate(nil, true)

	evt := &nostr.Event{Kind: nostr.KindSimpleGroupCreateGroup, PubKey: "random-user"}
	reject, _ := gate(context.Background(), evt)
	if reject {
		t.Error("public creation is enabled but event was rejected")
	}
}

func TestGroupCreationGate_AdminOnly(t *testing.T) {
	gate := GroupCreationGate([]string{"admin1", "admin2"}, false)

	evt := &nostr.Event{Kind: nostr.KindSimpleGroupCreateGroup, PubKey: "admin1"}
	reject, _ := gate(context.Background(), evt)
	if reject {
		t.Error("admin1 should be allowed to create groups, but was rejected")
	}

	evt = &nostr.Event{Kind: nostr.KindSimpleGroupCreateGroup, PubKey: "random-user"}
	reject, msg := gate(context.Background(), evt)
	if !reject {
		t.Error("random user should be rejected for group creation, but was allowed")
	}
	if msg != "restricted: only designated admins can create groups" {
		t.Errorf("unexpected rejection message: %q", msg)
	}
}

func TestGroupCreationGate_PassthroughNonCreateEvents(t *testing.T) {
	gate := GroupCreationGate(nil, false)

	// Use a text note (kind 1) to verify the gate ignores non-create events.
	evt := &nostr.Event{Kind: nostr.KindTextNote, PubKey: "anyone"}
	reject, _ := gate(context.Background(), evt)
	if reject {
		t.Error("non-create event should pass through the creation gate")
	}
}

func TestGroupCreationGate_EmptyAdminsDenyAll(t *testing.T) {
	gate := GroupCreationGate(nil, false)

	evt := &nostr.Event{Kind: nostr.KindSimpleGroupCreateGroup, PubKey: "anyone"}
	reject, _ := gate(context.Background(), evt)
	if !reject {
		t.Error("with no admin pubkeys and public creation off, all creation should be denied")
	}
}

func ptrStr(s string) *string { return &s }

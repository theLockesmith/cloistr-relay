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
	}

	for _, action := range actions {
		if !AllowAction(context.Background(), testGroup, AdminRole, action) {
			t.Errorf("admin should be allowed to perform %T, but was denied", action)
		}
	}
}

func TestAllowAction_ModeratorCanDeleteAndRemove(t *testing.T) {
	allowed := []relay29.Action{
		relay29.DeleteEvent{Targets: []string{"abc"}},
		relay29.RemoveUser{Targets: []string{"abc"}},
	}
	for _, action := range allowed {
		if !AllowAction(context.Background(), testGroup, ModeratorRole, action) {
			t.Errorf("moderator should be allowed to perform %T, but was denied", action)
		}
	}

	denied := []relay29.Action{
		relay29.EditMetadata{NameValue: ptrStr("hijack")},
	}
	for _, action := range denied {
		if AllowAction(context.Background(), testGroup, ModeratorRole, action) {
			t.Errorf("moderator should NOT be allowed to perform %T, but was allowed", action)
		}
	}
}

func TestAllowAction_MemberCanInviteOnly(t *testing.T) {
	invite := relay29.PutUser{Targets: []relay29.PubKeyRoles{{PubKey: "newuser"}}}
	if !AllowAction(context.Background(), testGroup, nil, invite) {
		t.Error("regular member should be allowed to invite (PutUser), but was denied")
	}

	denied := []relay29.Action{
		relay29.RemoveUser{Targets: []string{"abc"}},
		relay29.DeleteEvent{Targets: []string{"abc"}},
		relay29.EditMetadata{NameValue: ptrStr("hijack")},
	}
	for _, action := range denied {
		if AllowAction(context.Background(), testGroup, nil, action) {
			t.Errorf("regular member should NOT be allowed to perform %T, but was allowed", action)
		}
	}
}

func TestAllowAction_UnknownRoleDenied(t *testing.T) {
	strangerRole := &nip29.Role{Name: "stranger", Description: "not a real role"}
	edit := relay29.EditMetadata{NameValue: ptrStr("nope")}
	if AllowAction(context.Background(), testGroup, strangerRole, edit) {
		t.Error("unknown role should be denied EditMetadata, but was allowed")
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

	evt := &nostr.Event{Kind: 9, PubKey: "anyone"}
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

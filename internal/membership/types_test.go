package membership

import (
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

func TestKindConstants(t *testing.T) {
	tests := []struct {
		name  string
		kind  int
		value int
	}{
		{"MembershipList", KindMembershipList, 13534},
		{"AddMember", KindAddMember, 8000},
		{"RemoveMember", KindRemoveMember, 8001},
		{"JoinRequest", KindJoinRequest, 28934},
		{"InviteResponse", KindInviteResponse, 28935},
		{"LeaveRequest", KindLeaveRequest, 28936},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.kind != tt.value {
				t.Errorf("Kind%s = %d, want %d", tt.name, tt.kind, tt.value)
			}
		})
	}
}

func TestIsMembershipKind(t *testing.T) {
	membershipKinds := []int{
		KindMembershipList,
		KindAddMember,
		KindRemoveMember,
		KindJoinRequest,
		KindInviteResponse,
		KindLeaveRequest,
	}

	for _, kind := range membershipKinds {
		if !IsMembershipKind(kind) {
			t.Errorf("IsMembershipKind(%d) = false, want true", kind)
		}
	}

	nonMembershipKinds := []int{0, 1, 3, 4, 7, 1985, 10002}
	for _, kind := range nonMembershipKinds {
		if IsMembershipKind(kind) {
			t.Errorf("IsMembershipKind(%d) = true, want false", kind)
		}
	}
}

func TestGenerateInviteCode(t *testing.T) {
	code1, err := GenerateInviteCode()
	if err != nil {
		t.Fatalf("GenerateInviteCode() error: %v", err)
	}

	if len(code1) != 32 {
		t.Errorf("Invite code length = %d, want 32", len(code1))
	}

	// Generate another to verify randomness
	code2, err := GenerateInviteCode()
	if err != nil {
		t.Fatalf("GenerateInviteCode() error: %v", err)
	}

	if code1 == code2 {
		t.Error("Generated codes should be unique")
	}
}

func TestInviteIsValid(t *testing.T) {
	tests := []struct {
		name     string
		invite   Invite
		expected bool
	}{
		{
			name: "valid invite",
			invite: Invite{
				Code:      "test",
				ExpiresAt: time.Now().Add(time.Hour),
				MaxUses:   10,
				Uses:      0,
			},
			expected: true,
		},
		{
			name: "expired invite",
			invite: Invite{
				Code:      "test",
				ExpiresAt: time.Now().Add(-time.Hour),
				MaxUses:   10,
				Uses:      0,
			},
			expected: false,
		},
		{
			name: "used up invite",
			invite: Invite{
				Code:    "test",
				MaxUses: 5,
				Uses:    5,
			},
			expected: false,
		},
		{
			name: "unlimited uses",
			invite: Invite{
				Code:    "test",
				MaxUses: 0,
				Uses:    1000,
			},
			expected: true,
		},
		{
			name: "no expiry",
			invite: Invite{
				Code:    "test",
				MaxUses: 10,
				Uses:    5,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.invite.IsValidInvite(); got != tt.expected {
				t.Errorf("IsValidInvite() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestCreateMembershipListEvent(t *testing.T) {
	members := []Member{
		{Pubkey: "pubkey1"},
		{Pubkey: "pubkey2"},
		{Pubkey: "pubkey3"},
	}

	event := CreateMembershipListEvent("relaypubkey", members)

	if event.Kind != KindMembershipList {
		t.Errorf("Kind = %d, want %d", event.Kind, KindMembershipList)
	}

	if event.PubKey != "relaypubkey" {
		t.Errorf("PubKey = %s, want relaypubkey", event.PubKey)
	}

	// Check for protected tag
	if !HasProtectedTag(event) {
		t.Error("Event should have protected tag")
	}

	// Check for member tags
	memberCount := 0
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "member" {
			memberCount++
		}
	}

	if memberCount != 3 {
		t.Errorf("Member tag count = %d, want 3", memberCount)
	}
}

func TestCreateAddMemberEvent(t *testing.T) {
	event := CreateAddMemberEvent("relaypubkey", "memberpubkey")

	if event.Kind != KindAddMember {
		t.Errorf("Kind = %d, want %d", event.Kind, KindAddMember)
	}

	if !HasProtectedTag(event) {
		t.Error("Event should have protected tag")
	}

	// Check for p tag
	found := false
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "p" && tag[1] == "memberpubkey" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Event should have p tag with member pubkey")
	}
}

func TestCreateRemoveMemberEvent(t *testing.T) {
	event := CreateRemoveMemberEvent("relaypubkey", "memberpubkey")

	if event.Kind != KindRemoveMember {
		t.Errorf("Kind = %d, want %d", event.Kind, KindRemoveMember)
	}

	if !HasProtectedTag(event) {
		t.Error("Event should have protected tag")
	}
}

func TestCreateJoinRequestEvent(t *testing.T) {
	// Without invite code
	event1 := CreateJoinRequestEvent("userpubkey", "")
	if event1.Kind != KindJoinRequest {
		t.Errorf("Kind = %d, want %d", event1.Kind, KindJoinRequest)
	}

	if !HasProtectedTag(event1) {
		t.Error("Event should have protected tag")
	}

	// With invite code
	event2 := CreateJoinRequestEvent("userpubkey", "invitecode123")

	claimFound := false
	for _, tag := range event2.Tags {
		if len(tag) >= 2 && tag[0] == "claim" && tag[1] == "invitecode123" {
			claimFound = true
			break
		}
	}

	if !claimFound {
		t.Error("Event should have claim tag with invite code")
	}
}

func TestCreateInviteResponseEvent(t *testing.T) {
	event := CreateInviteResponseEvent("relaypubkey", "invitecode", "requesterpubkey")

	if event.Kind != KindInviteResponse {
		t.Errorf("Kind = %d, want %d", event.Kind, KindInviteResponse)
	}

	if !HasProtectedTag(event) {
		t.Error("Event should have protected tag")
	}

	// Check for claim tag
	claimFound := false
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "claim" && tag[1] == "invitecode" {
			claimFound = true
			break
		}
	}

	if !claimFound {
		t.Error("Event should have claim tag with invite code")
	}
}

func TestCreateLeaveRequestEvent(t *testing.T) {
	event := CreateLeaveRequestEvent("userpubkey")

	if event.Kind != KindLeaveRequest {
		t.Errorf("Kind = %d, want %d", event.Kind, KindLeaveRequest)
	}

	if !HasProtectedTag(event) {
		t.Error("Event should have protected tag")
	}
}

func TestParseJoinRequest(t *testing.T) {
	// With invite code
	event1 := &nostr.Event{
		Kind:   KindJoinRequest,
		PubKey: "userpubkey123",
		Tags: nostr.Tags{
			{"-"},
			{"p", "userpubkey123"},
			{"claim", "myinvitecode"},
		},
	}

	pubkey, inviteCode := ParseJoinRequest(event1)
	if pubkey != "userpubkey123" {
		t.Errorf("Pubkey = %s, want userpubkey123", pubkey)
	}
	if inviteCode != "myinvitecode" {
		t.Errorf("InviteCode = %s, want myinvitecode", inviteCode)
	}

	// Without invite code
	event2 := &nostr.Event{
		Kind:   KindJoinRequest,
		PubKey: "userpubkey456",
		Tags: nostr.Tags{
			{"-"},
			{"p", "userpubkey456"},
		},
	}

	pubkey2, inviteCode2 := ParseJoinRequest(event2)
	if pubkey2 != "userpubkey456" {
		t.Errorf("Pubkey = %s, want userpubkey456", pubkey2)
	}
	if inviteCode2 != "" {
		t.Errorf("InviteCode = %s, want empty", inviteCode2)
	}
}

func TestHasProtectedTag(t *testing.T) {
	tests := []struct {
		name     string
		tags     nostr.Tags
		expected bool
	}{
		{
			name:     "has protected tag",
			tags:     nostr.Tags{{"-"}, {"p", "pubkey"}},
			expected: true,
		},
		{
			name:     "no protected tag",
			tags:     nostr.Tags{{"p", "pubkey"}, {"e", "eventid"}},
			expected: false,
		},
		{
			name:     "empty tags",
			tags:     nostr.Tags{},
			expected: false,
		},
		{
			name:     "protected tag with extra values",
			tags:     nostr.Tags{{"-", "extra"}, {"p", "pubkey"}},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &nostr.Event{Tags: tt.tags}
			if got := HasProtectedTag(event); got != tt.expected {
				t.Errorf("HasProtectedTag() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Enabled {
		t.Error("Default should be disabled")
	}

	if cfg.RequireMembership {
		t.Error("Default should not require membership")
	}

	if !cfg.AllowJoinRequests {
		t.Error("Default should allow join requests")
	}

	if cfg.PublishMembershipList {
		t.Error("Default should not publish membership list")
	}

	if cfg.DefaultInviteExpiry != 7*24*time.Hour {
		t.Errorf("DefaultInviteExpiry = %v, want 1 week", cfg.DefaultInviteExpiry)
	}

	if cfg.DefaultInviteMaxUses != 1 {
		t.Errorf("DefaultInviteMaxUses = %d, want 1", cfg.DefaultInviteMaxUses)
	}
}

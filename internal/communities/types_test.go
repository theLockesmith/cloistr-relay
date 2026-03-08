package communities

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

func TestKindConstants(t *testing.T) {
	tests := []struct {
		name  string
		kind  int
		value int
	}{
		{"CommunityDefinition", KindCommunityDefinition, 34550},
		{"CommunityPost", KindCommunityPost, 1111},
		{"Approval", KindApproval, 4550},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.kind != tt.value {
				t.Errorf("Kind%s = %d, want %d", tt.name, tt.kind, tt.value)
			}
		})
	}
}

func TestIsCommunityKind(t *testing.T) {
	communityKinds := []int{
		KindCommunityDefinition,
		KindCommunityPost,
		KindApproval,
	}

	for _, kind := range communityKinds {
		if !IsCommunityKind(kind) {
			t.Errorf("IsCommunityKind(%d) = false, want true", kind)
		}
	}

	nonCommunityKinds := []int{0, 1, 3, 4, 7, 1985, 10002}
	for _, kind := range nonCommunityKinds {
		if IsCommunityKind(kind) {
			t.Errorf("IsCommunityKind(%d) = true, want false", kind)
		}
	}
}

func TestParseCommunityDefinition(t *testing.T) {
	event := &nostr.Event{
		Kind:   KindCommunityDefinition,
		PubKey: "ownerpubkey123",
		Tags: nostr.Tags{
			{"d", "my-community"},
			{"name", "My Community"},
			{"description", "A test community"},
			{"image", "https://example.com/img.png", "800x600"},
			{"rules", "Be nice"},
			{"relay", "wss://relay1.example.com"},
			{"relay", "wss://relay2.example.com"},
			{"p", "mod1pubkey", "wss://relay.example.com", "moderator"},
			{"p", "mod2pubkey", "", "moderator", "admin"},
		},
	}

	community := ParseCommunityDefinition(event)

	if community == nil {
		t.Fatal("ParseCommunityDefinition returned nil")
	}

	if community.ID != "my-community" {
		t.Errorf("ID = %s, want my-community", community.ID)
	}

	if community.OwnerPubkey != "ownerpubkey123" {
		t.Errorf("OwnerPubkey = %s, want ownerpubkey123", community.OwnerPubkey)
	}

	if community.Name != "My Community" {
		t.Errorf("Name = %s, want 'My Community'", community.Name)
	}

	if community.Description != "A test community" {
		t.Errorf("Description = %s, want 'A test community'", community.Description)
	}

	if community.Image != "https://example.com/img.png" {
		t.Errorf("Image = %s, want 'https://example.com/img.png'", community.Image)
	}

	if community.ImageDimensions != "800x600" {
		t.Errorf("ImageDimensions = %s, want '800x600'", community.ImageDimensions)
	}

	if community.Rules != "Be nice" {
		t.Errorf("Rules = %s, want 'Be nice'", community.Rules)
	}

	if len(community.RelayURLs) != 2 {
		t.Errorf("RelayURLs count = %d, want 2", len(community.RelayURLs))
	}

	if len(community.Moderators) != 2 {
		t.Errorf("Moderators count = %d, want 2", len(community.Moderators))
	}

	// Check first moderator
	if community.Moderators[0].Pubkey != "mod1pubkey" {
		t.Errorf("Moderator[0].Pubkey = %s, want mod1pubkey", community.Moderators[0].Pubkey)
	}
	if community.Moderators[0].RelayHint != "wss://relay.example.com" {
		t.Errorf("Moderator[0].RelayHint = %s, want wss://relay.example.com", community.Moderators[0].RelayHint)
	}

	// Check second moderator
	if community.Moderators[1].Pubkey != "mod2pubkey" {
		t.Errorf("Moderator[1].Pubkey = %s, want mod2pubkey", community.Moderators[1].Pubkey)
	}
	if community.Moderators[1].Role != "admin" {
		t.Errorf("Moderator[1].Role = %s, want admin", community.Moderators[1].Role)
	}
}

func TestParseCommunityDefinitionWrongKind(t *testing.T) {
	event := &nostr.Event{
		Kind: 1, // Wrong kind
	}

	community := ParseCommunityDefinition(event)
	if community != nil {
		t.Error("Should return nil for wrong kind")
	}
}

func TestCreateCommunityDefinitionEvent(t *testing.T) {
	community := &Community{
		ID:              "test-community",
		OwnerPubkey:     "ownerpubkey",
		Name:            "Test Community",
		Description:     "A test",
		Image:           "https://example.com/img.png",
		ImageDimensions: "800x600",
		Rules:           "Be nice",
		RelayURLs:       []string{"wss://relay1.example.com"},
		Moderators: []Moderator{
			{Pubkey: "modpubkey", RelayHint: "wss://relay.example.com", Role: "admin"},
		},
	}

	event := CreateCommunityDefinitionEvent(community)

	if event.Kind != KindCommunityDefinition {
		t.Errorf("Kind = %d, want %d", event.Kind, KindCommunityDefinition)
	}

	if event.PubKey != "ownerpubkey" {
		t.Errorf("PubKey = %s, want ownerpubkey", event.PubKey)
	}

	// Parse it back to verify
	parsed := ParseCommunityDefinition(event)
	if parsed.ID != community.ID {
		t.Errorf("Parsed ID = %s, want %s", parsed.ID, community.ID)
	}
	if parsed.Name != community.Name {
		t.Errorf("Parsed Name = %s, want %s", parsed.Name, community.Name)
	}
}

func TestIsCommunityPost(t *testing.T) {
	// Valid community post
	validPost := &nostr.Event{
		Kind: KindCommunityPost,
		Tags: nostr.Tags{
			{"A", "34550:ownerpubkey:community-id"},
			{"P", "ownerpubkey"},
		},
	}

	if !IsCommunityPost(validPost) {
		t.Error("Should recognize valid community post")
	}

	// Missing A tag
	invalidPost := &nostr.Event{
		Kind: KindCommunityPost,
		Tags: nostr.Tags{
			{"P", "ownerpubkey"},
		},
	}

	if IsCommunityPost(invalidPost) {
		t.Error("Should reject post without A tag")
	}

	// Wrong kind
	wrongKind := &nostr.Event{
		Kind: 1,
		Tags: nostr.Tags{
			{"A", "34550:ownerpubkey:community-id"},
		},
	}

	if IsCommunityPost(wrongKind) {
		t.Error("Should reject wrong kind")
	}
}

func TestGetCommunityRef(t *testing.T) {
	event := &nostr.Event{
		Kind: KindCommunityPost,
		Tags: nostr.Tags{
			{"A", "34550:ownerpubkey:community-id"},
			{"P", "ownerpubkey"},
		},
	}

	ref := GetCommunityRef(event)
	if ref != "34550:ownerpubkey:community-id" {
		t.Errorf("GetCommunityRef() = %s, want '34550:ownerpubkey:community-id'", ref)
	}

	// No A tag
	event2 := &nostr.Event{
		Tags: nostr.Tags{
			{"P", "ownerpubkey"},
		},
	}

	ref2 := GetCommunityRef(event2)
	if ref2 != "" {
		t.Errorf("GetCommunityRef() = %s, want empty", ref2)
	}
}

func TestParseApproval(t *testing.T) {
	event := &nostr.Event{
		Kind:    KindApproval,
		Content: `{"id":"postid","kind":1111}`,
		Tags: nostr.Tags{
			{"a", "34550:ownerpubkey:community-id"},
			{"e", "postid123"},
			{"p", "authorpubkey"},
			{"k", "1111"},
		},
	}

	approval := ParseApproval(event)

	if approval == nil {
		t.Fatal("ParseApproval returned nil")
	}

	if approval.CommunityRef != "34550:ownerpubkey:community-id" {
		t.Errorf("CommunityRef = %s, want '34550:ownerpubkey:community-id'", approval.CommunityRef)
	}

	if approval.PostID != "postid123" {
		t.Errorf("PostID = %s, want 'postid123'", approval.PostID)
	}

	if approval.AuthorPubkey != "authorpubkey" {
		t.Errorf("AuthorPubkey = %s, want 'authorpubkey'", approval.AuthorPubkey)
	}
}

func TestParseApprovalWrongKind(t *testing.T) {
	event := &nostr.Event{
		Kind: 1, // Wrong kind
	}

	approval := ParseApproval(event)
	if approval != nil {
		t.Error("Should return nil for wrong kind")
	}
}

func TestCommunityIsModerator(t *testing.T) {
	community := &Community{
		OwnerPubkey: "ownerpubkey",
		Moderators: []Moderator{
			{Pubkey: "mod1pubkey"},
			{Pubkey: "mod2pubkey"},
		},
	}

	// Owner is always moderator
	if !community.IsModerator("ownerpubkey") {
		t.Error("Owner should be moderator")
	}

	// Listed moderators
	if !community.IsModerator("mod1pubkey") {
		t.Error("mod1 should be moderator")
	}
	if !community.IsModerator("mod2pubkey") {
		t.Error("mod2 should be moderator")
	}

	// Random pubkey
	if community.IsModerator("randompubkey") {
		t.Error("Random pubkey should not be moderator")
	}
}

func TestCreateApprovalEvent(t *testing.T) {
	approval := &Approval{
		CommunityRef: "34550:ownerpubkey:community-id",
		PostID:       "postid123",
		AuthorPubkey: "authorpubkey",
		PostKind:     1111,
	}

	postEvent := &nostr.Event{
		ID:      "postid123",
		Kind:    1111,
		Content: "Hello community!",
	}

	event := CreateApprovalEvent("moderatorpubkey", approval, postEvent)

	if event.Kind != KindApproval {
		t.Errorf("Kind = %d, want %d", event.Kind, KindApproval)
	}

	if event.PubKey != "moderatorpubkey" {
		t.Errorf("PubKey = %s, want moderatorpubkey", event.PubKey)
	}

	// Content should contain the post JSON
	if event.Content == "" {
		t.Error("Content should contain post JSON")
	}

	// Check for community a-tag
	foundCommunityTag := false
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "a" && tag[1] == "34550:ownerpubkey:community-id" {
			foundCommunityTag = true
			break
		}
	}

	if !foundCommunityTag {
		t.Error("Should have community a-tag")
	}
}

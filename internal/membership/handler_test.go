package membership

import (
	"context"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

func TestNewJoinHandler_MissingKey(t *testing.T) {
	_, err := NewJoinHandler(JoinHandlerConfig{
		Store:     nil,
		SecretKey: "",
		AddEvent:  func(context.Context, *nostr.Event) (bool, error) { return false, nil },
	})
	if err == nil {
		t.Fatal("expected error for empty secret key")
	}
}

func TestNewJoinHandler_MissingAddEvent(t *testing.T) {
	_, err := NewJoinHandler(JoinHandlerConfig{
		Store:     nil,
		SecretKey: nostr.GeneratePrivateKey(),
		AddEvent:  nil,
	})
	if err == nil {
		t.Fatal("expected error for nil AddEvent")
	}
}

func TestNewJoinHandler_InvalidKey(t *testing.T) {
	_, err := NewJoinHandler(JoinHandlerConfig{
		Store:     nil,
		SecretKey: "not-a-valid-hex-key",
		AddEvent:  func(context.Context, *nostr.Event) (bool, error) { return false, nil },
	})
	if err == nil {
		t.Fatal("expected error for invalid secret key")
	}
}

func TestNewJoinHandler_ValidConfig(t *testing.T) {
	sk := nostr.GeneratePrivateKey()
	pub, _ := nostr.GetPublicKey(sk)

	h, err := NewJoinHandler(JoinHandlerConfig{
		Store:     nil,
		SecretKey: sk,
		AddEvent:  func(context.Context, *nostr.Event) (bool, error) { return false, nil },
	})
	if err != nil {
		t.Fatalf("NewJoinHandler: %v", err)
	}
	if h.pubkey != pub {
		t.Errorf("pubkey = %q, want %q", h.pubkey, pub)
	}
}

func TestCreateAddMemberEvent_Structure(t *testing.T) {
	relayPub := "aabbccdd"
	memberPub := "11223344"

	evt := CreateAddMemberEvent(relayPub, memberPub)

	if evt.Kind != KindAddMember {
		t.Errorf("kind = %d, want %d", evt.Kind, KindAddMember)
	}
	if evt.PubKey != relayPub {
		t.Errorf("pubkey = %q, want %q", evt.PubKey, relayPub)
	}

	// Check NIP-70 protected tag
	hasProtected := false
	hasPTag := false
	for _, tag := range evt.Tags {
		if len(tag) >= 1 && tag[0] == "-" {
			hasProtected = true
		}
		if len(tag) >= 2 && tag[0] == "p" && tag[1] == memberPub {
			hasPTag = true
		}
	}
	if !hasProtected {
		t.Error("missing NIP-70 protected tag")
	}
	if !hasPTag {
		t.Error("missing p tag for member pubkey")
	}
}

func TestParseJoinRequest_WithInvite(t *testing.T) {
	userPub := "aabb1122"
	evt := CreateJoinRequestEvent(userPub, "invite-code-123")

	pubkey, invite := ParseJoinRequest(evt)
	if pubkey != userPub {
		t.Errorf("pubkey = %q, want %q", pubkey, userPub)
	}
	if invite != "invite-code-123" {
		t.Errorf("invite = %q, want %q", invite, "invite-code-123")
	}
}

func TestParseJoinRequest_NoInvite(t *testing.T) {
	userPub := "aabb1122"
	evt := CreateJoinRequestEvent(userPub, "")

	pubkey, invite := ParseJoinRequest(evt)
	if pubkey != userPub {
		t.Errorf("pubkey = %q, want %q", pubkey, userPub)
	}
	if invite != "" {
		t.Errorf("invite = %q, want empty", invite)
	}
}

func TestInviteValidity(t *testing.T) {
	t.Run("valid invite", func(t *testing.T) {
		inv := &Invite{
			Code:      "abc",
			MaxUses:   5,
			Uses:      2,
			ExpiresAt: time.Now().Add(time.Hour),
		}
		if !inv.IsValidInvite() {
			t.Error("expected valid invite")
		}
	})

	t.Run("expired invite", func(t *testing.T) {
		inv := &Invite{
			Code:      "abc",
			MaxUses:   5,
			Uses:      0,
			ExpiresAt: time.Now().Add(-time.Hour),
		}
		if inv.IsValidInvite() {
			t.Error("expected invalid (expired)")
		}
	})

	t.Run("fully used invite", func(t *testing.T) {
		inv := &Invite{
			Code:    "abc",
			MaxUses: 1,
			Uses:    1,
		}
		if inv.IsValidInvite() {
			t.Error("expected invalid (fully used)")
		}
	})

	t.Run("unlimited uses", func(t *testing.T) {
		inv := &Invite{
			Code:    "abc",
			MaxUses: 0,
			Uses:    999,
		}
		if !inv.IsValidInvite() {
			t.Error("expected valid (unlimited)")
		}
	})

	t.Run("no expiry", func(t *testing.T) {
		inv := &Invite{
			Code:    "abc",
			MaxUses: 1,
			Uses:    0,
		}
		if !inv.IsValidInvite() {
			t.Error("expected valid (no expiry)")
		}
	})
}

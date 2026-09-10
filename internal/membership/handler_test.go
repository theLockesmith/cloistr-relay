package membership

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

// mockStore provides an in-memory membership store for testing.
type mockStore struct {
	mu      sync.Mutex
	members map[string]*Member
	invites map[string]*Invite
}

func newMockStore() *mockStore {
	return &mockStore{
		members: make(map[string]*Member),
		invites: make(map[string]*Invite),
	}
}

func (m *mockStore) isMember(pubkey string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.members[pubkey]
	return ok
}

func (m *mockStore) getMember(pubkey string) *Member {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.members[pubkey]
}

// testJoinHandler builds a JoinHandler backed by a mock store for testing.
// It returns the handler and a slice that captures published events.
func testJoinHandler(t *testing.T, requireInvite bool) (*JoinHandler, *mockStore, *[]*nostr.Event) {
	t.Helper()

	sk := nostr.GeneratePrivateKey()
	ms := newMockStore()

	var published []*nostr.Event
	var pubMu sync.Mutex

	// We can't use the real Store (needs a DB), so we build a JoinHandler
	// with a real Store set to nil and override its behavior via the
	// methods the handler calls. Instead, build a thin Store wrapper.
	//
	// Actually, since JoinHandler uses *Store methods and Store requires
	// *sql.DB, we build a real JoinHandler but redirect its store calls
	// through a test double. The cleanest way: create a JoinHandler
	// manually and swap the store field with a shim.
	//
	// For unit tests we test the handler logic directly by constructing
	// events and calling the returned funcs.

	// Build a real store (nil DB) for type satisfaction, then we'll test
	// differently — we test RejectJoinRequest and OnJoinRequestSaved
	// logic by exercising them against a real DB-backed store in
	// integration tests. For pure logic tests here, we test the event
	// creation helpers and the handler config validation.

	_ = ms // used below in integration-style tests if DB available

	addEvent := func(_ context.Context, evt *nostr.Event) (bool, error) {
		pubMu.Lock()
		published = append(published, evt)
		pubMu.Unlock()
		return false, nil
	}

	handler, err := NewJoinHandler(JoinHandlerConfig{
		Store:         nil, // will panic if Store methods are called; see below
		SecretKey:     sk,
		RequireInvite: requireInvite,
		AddEvent:      addEvent,
	})
	if err != nil {
		t.Fatalf("NewJoinHandler: %v", err)
	}

	return handler, ms, &published
}

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

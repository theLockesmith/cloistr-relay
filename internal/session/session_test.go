package session

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.Enabled {
		t.Error("Expected Enabled to be true by default")
	}
	if cfg.SessionTTL != 30*time.Minute {
		t.Errorf("Expected SessionTTL to be 30m, got %v", cfg.SessionTTL)
	}
	if cfg.KeyPrefix != "session:" {
		t.Errorf("Expected KeyPrefix to be 'session:', got %s", cfg.KeyPrefix)
	}
}

func TestStore_New(t *testing.T) {
	store := New(nil, nil)
	if store == nil {
		t.Error("Expected non-nil store")
	}
	if store.config == nil {
		t.Error("Expected non-nil config")
	}
	if store.config.SessionTTL != 30*time.Minute {
		t.Errorf("Expected default SessionTTL, got %v", store.config.SessionTTL)
	}
}

func TestStore_Key(t *testing.T) {
	store := New(nil, nil)

	key := store.key("abc123")
	expected := "session:abc123"
	if key != expected {
		t.Errorf("Expected %s, got %s", expected, key)
	}
}

func TestStore_CreateWithoutRedis(t *testing.T) {
	store := New(nil, DefaultConfig())

	state, err := store.Create(nil, "test-session", "192.168.1.1")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if state == nil {
		t.Fatal("Expected non-nil state")
	}
	if state.SessionID != "test-session" {
		t.Errorf("Expected SessionID 'test-session', got %s", state.SessionID)
	}
	if state.RemoteIP != "192.168.1.1" {
		t.Errorf("Expected RemoteIP '192.168.1.1', got %s", state.RemoteIP)
	}
	if state.ConnectedAt == 0 {
		t.Error("Expected ConnectedAt to be set")
	}
}

func TestStore_CreateDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false
	store := New(nil, cfg)

	state, err := store.Create(nil, "test-session", "192.168.1.1")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if state == nil {
		t.Fatal("Expected non-nil state")
	}
	if state.SessionID != "test-session" {
		t.Errorf("Expected SessionID 'test-session', got %s", state.SessionID)
	}
}

func TestStore_GetWithoutRedis(t *testing.T) {
	store := New(nil, DefaultConfig())

	state, err := store.Get(nil, "nonexistent")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if state != nil {
		t.Error("Expected nil state without Redis")
	}
}

func TestStore_SetAuthPubkeyWithoutRedis(t *testing.T) {
	store := New(nil, DefaultConfig())

	// Should not error without Redis (fail open)
	err := store.SetAuthPubkey(nil, "test-session", "abc123pubkey")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestStore_GetAuthPubkeyWithoutRedis(t *testing.T) {
	store := New(nil, DefaultConfig())

	pubkey := store.GetAuthPubkey(nil, "nonexistent")
	if pubkey != "" {
		t.Errorf("Expected empty pubkey without Redis, got %s", pubkey)
	}
}

func TestStore_AddSubscriptionWithoutRedis(t *testing.T) {
	store := New(nil, DefaultConfig())

	err := store.AddSubscription(nil, "test-session", "sub-1")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestStore_RemoveSubscriptionWithoutRedis(t *testing.T) {
	store := New(nil, DefaultConfig())

	err := store.RemoveSubscription(nil, "test-session", "sub-1")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestStore_DeleteWithoutRedis(t *testing.T) {
	store := New(nil, DefaultConfig())

	err := store.Delete(nil, "test-session")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestStore_TouchWithoutRedis(t *testing.T) {
	store := New(nil, DefaultConfig())

	err := store.Touch(nil, "test-session")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestStore_ListSessionsWithoutRedis(t *testing.T) {
	store := New(nil, DefaultConfig())

	sessions, err := store.ListSessions(nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if sessions != nil {
		t.Error("Expected nil sessions without Redis")
	}
}

func TestStore_CountSessionsWithoutRedis(t *testing.T) {
	store := New(nil, DefaultConfig())

	count, err := store.CountSessions(nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 sessions without Redis, got %d", count)
	}
}

func TestStore_IsAuthenticatedWithoutRedis(t *testing.T) {
	store := New(nil, DefaultConfig())

	auth := store.IsAuthenticated(nil, "nonexistent")
	if auth {
		t.Error("Expected false for IsAuthenticated without Redis")
	}
}

func TestStore_GetSessionsByPubkeyWithoutRedis(t *testing.T) {
	store := New(nil, DefaultConfig())

	sessions, err := store.GetSessionsByPubkey(nil, "somepubkey")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if sessions != nil {
		t.Error("Expected nil sessions without Redis")
	}
}

func TestState_Fields(t *testing.T) {
	state := &State{
		SessionID:     "test-123",
		AuthPubkey:    "pubkey-abc",
		AuthedAt:      1234567890,
		ConnectedAt:   1234567800,
		RemoteIP:      "10.0.0.1",
		Subscriptions: []string{"sub-1", "sub-2"},
		LastActivity:  1234567895,
	}

	if state.SessionID != "test-123" {
		t.Errorf("SessionID mismatch: %s", state.SessionID)
	}
	if state.AuthPubkey != "pubkey-abc" {
		t.Errorf("AuthPubkey mismatch: %s", state.AuthPubkey)
	}
	if state.AuthedAt != 1234567890 {
		t.Errorf("AuthedAt mismatch: %d", state.AuthedAt)
	}
	if state.ConnectedAt != 1234567800 {
		t.Errorf("ConnectedAt mismatch: %d", state.ConnectedAt)
	}
	if state.RemoteIP != "10.0.0.1" {
		t.Errorf("RemoteIP mismatch: %s", state.RemoteIP)
	}
	if len(state.Subscriptions) != 2 {
		t.Errorf("Subscriptions count mismatch: %d", len(state.Subscriptions))
	}
	if state.LastActivity != 1234567895 {
		t.Errorf("LastActivity mismatch: %d", state.LastActivity)
	}
}

// Integration tests would require a real Redis/Dragonfly connection
// These are left as structural tests that validate the API without Redis

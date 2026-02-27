package management

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

func TestValidateNIP98Auth(t *testing.T) {
	// Generate a test key pair
	sk := nostr.GeneratePrivateKey()
	pk, _ := nostr.GetPublicKey(sk)

	adminPubkeys := []string{pk}

	t.Run("missing auth header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "http://localhost:3334/management", nil)
		_, err := ValidateNIP98Auth(req, adminPubkeys)
		if err != ErrMissingAuth {
			t.Errorf("expected ErrMissingAuth, got %v", err)
		}
	})

	t.Run("invalid auth format", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "http://localhost:3334/management", nil)
		req.Header.Set("Authorization", "Bearer invalid")
		_, err := ValidateNIP98Auth(req, adminPubkeys)
		if err != ErrInvalidAuth {
			t.Errorf("expected ErrInvalidAuth, got %v", err)
		}
	})

	t.Run("invalid base64", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "http://localhost:3334/management", nil)
		req.Header.Set("Authorization", "Nostr not-valid-base64!")
		_, err := ValidateNIP98Auth(req, adminPubkeys)
		if err != ErrInvalidAuth {
			t.Errorf("expected ErrInvalidAuth, got %v", err)
		}
	})

	t.Run("wrong event kind", func(t *testing.T) {
		event := nostr.Event{
			Kind:      1, // Wrong kind, should be 27235
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			PubKey:    pk,
			Tags: nostr.Tags{
				{"u", "http://localhost:3334/management"},
				{"method", "POST"},
			},
		}
		_ = event.Sign(sk)

		eventJSON, _ := json.Marshal(event)
		encoded := base64.StdEncoding.EncodeToString(eventJSON)

		req := httptest.NewRequest(http.MethodPost, "http://localhost:3334/management", nil)
		req.Header.Set("Authorization", "Nostr "+encoded)
		_, err := ValidateNIP98Auth(req, adminPubkeys)
		if err != ErrInvalidEvent {
			t.Errorf("expected ErrInvalidEvent, got %v", err)
		}
	})

	t.Run("expired auth event", func(t *testing.T) {
		event := nostr.Event{
			Kind:      AuthEventKind,
			CreatedAt: nostr.Timestamp(time.Now().Add(-2 * time.Minute).Unix()), // Too old
			PubKey:    pk,
			Tags: nostr.Tags{
				{"u", "http://localhost:3334/management"},
				{"method", "POST"},
			},
		}
		_ = event.Sign(sk)

		eventJSON, _ := json.Marshal(event)
		encoded := base64.StdEncoding.EncodeToString(eventJSON)

		req := httptest.NewRequest(http.MethodPost, "http://localhost:3334/management", nil)
		req.Header.Set("Authorization", "Nostr "+encoded)
		_, err := ValidateNIP98Auth(req, adminPubkeys)
		if err != ErrExpiredAuth {
			t.Errorf("expected ErrExpiredAuth, got %v", err)
		}
	})

	t.Run("missing u tag", func(t *testing.T) {
		event := nostr.Event{
			Kind:      AuthEventKind,
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			PubKey:    pk,
			Tags: nostr.Tags{
				{"method", "POST"},
			},
		}
		_ = event.Sign(sk)

		eventJSON, _ := json.Marshal(event)
		encoded := base64.StdEncoding.EncodeToString(eventJSON)

		req := httptest.NewRequest(http.MethodPost, "http://localhost:3334/management", nil)
		req.Header.Set("Authorization", "Nostr "+encoded)
		_, err := ValidateNIP98Auth(req, adminPubkeys)
		if err != ErrInvalidURL {
			t.Errorf("expected ErrInvalidURL, got %v", err)
		}
	})

	t.Run("url mismatch", func(t *testing.T) {
		event := nostr.Event{
			Kind:      AuthEventKind,
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			PubKey:    pk,
			Tags: nostr.Tags{
				{"u", "http://other-host:3334/management"},
				{"method", "POST"},
			},
		}
		_ = event.Sign(sk)

		eventJSON, _ := json.Marshal(event)
		encoded := base64.StdEncoding.EncodeToString(eventJSON)

		req := httptest.NewRequest(http.MethodPost, "http://localhost:3334/management", nil)
		req.Header.Set("Authorization", "Nostr "+encoded)
		_, err := ValidateNIP98Auth(req, adminPubkeys)
		if err != ErrInvalidURL {
			t.Errorf("expected ErrInvalidURL, got %v", err)
		}
	})

	t.Run("method mismatch", func(t *testing.T) {
		event := nostr.Event{
			Kind:      AuthEventKind,
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			PubKey:    pk,
			Tags: nostr.Tags{
				{"u", "http://localhost:3334/management"},
				{"method", "GET"}, // Should be POST
			},
		}
		_ = event.Sign(sk)

		eventJSON, _ := json.Marshal(event)
		encoded := base64.StdEncoding.EncodeToString(eventJSON)

		req := httptest.NewRequest(http.MethodPost, "http://localhost:3334/management", nil)
		req.Header.Set("Authorization", "Nostr "+encoded)
		_, err := ValidateNIP98Auth(req, adminPubkeys)
		if err != ErrInvalidMethod {
			t.Errorf("expected ErrInvalidMethod, got %v", err)
		}
	})

	t.Run("non-admin pubkey", func(t *testing.T) {
		otherSk := nostr.GeneratePrivateKey()
		otherPk, _ := nostr.GetPublicKey(otherSk)

		event := nostr.Event{
			Kind:      AuthEventKind,
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			PubKey:    otherPk,
			Tags: nostr.Tags{
				{"u", "http://localhost:3334/management"},
				{"method", "POST"},
			},
		}
		_ = event.Sign(otherSk)

		eventJSON, _ := json.Marshal(event)
		encoded := base64.StdEncoding.EncodeToString(eventJSON)

		req := httptest.NewRequest(http.MethodPost, "http://localhost:3334/management", nil)
		req.Header.Set("Authorization", "Nostr "+encoded)
		_, err := ValidateNIP98Auth(req, adminPubkeys)
		if err != ErrNotAdmin {
			t.Errorf("expected ErrNotAdmin, got %v", err)
		}
	})

	t.Run("valid auth", func(t *testing.T) {
		event := nostr.Event{
			Kind:      AuthEventKind,
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			PubKey:    pk,
			Tags: nostr.Tags{
				{"u", "http://localhost:3334/management"},
				{"method", "POST"},
			},
		}
		_ = event.Sign(sk)

		eventJSON, _ := json.Marshal(event)
		encoded := base64.StdEncoding.EncodeToString(eventJSON)

		req := httptest.NewRequest(http.MethodPost, "http://localhost:3334/management", nil)
		req.Header.Set("Authorization", "Nostr "+encoded)
		pubkey, err := ValidateNIP98Auth(req, adminPubkeys)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if pubkey != pk {
			t.Errorf("expected pubkey %s, got %s", pk, pubkey)
		}
	})
}

func TestGetRequestURL(t *testing.T) {
	t.Run("basic http request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "http://localhost:3334/management", nil)
		url := getRequestURL(req)
		if url != "http://localhost:3334/management" {
			t.Errorf("expected http://localhost:3334/management, got %s", url)
		}
	})

	t.Run("with X-Forwarded-Proto", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "http://localhost:3334/management", nil)
		req.Header.Set("X-Forwarded-Proto", "https")
		url := getRequestURL(req)
		if url != "https://localhost:3334/management" {
			t.Errorf("expected https://localhost:3334/management, got %s", url)
		}
	})

	t.Run("with X-Forwarded-Host", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "http://localhost:3334/management", nil)
		req.Header.Set("X-Forwarded-Host", "relay.example.com")
		url := getRequestURL(req)
		if url != "http://relay.example.com/management" {
			t.Errorf("expected http://relay.example.com/management, got %s", url)
		}
	})
}

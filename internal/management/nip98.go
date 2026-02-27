package management

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

const (
	// NIP-98 auth event kind
	AuthEventKind = 27235

	// Maximum age for auth events (60 seconds as per NIP-98)
	MaxAuthEventAge = 60 * time.Second
)

var (
	ErrMissingAuth     = errors.New("missing Nostr authorization header")
	ErrInvalidAuth     = errors.New("invalid authorization format")
	ErrInvalidEvent    = errors.New("invalid auth event")
	ErrExpiredAuth     = errors.New("auth event expired")
	ErrInvalidURL      = errors.New("auth event URL mismatch")
	ErrInvalidMethod   = errors.New("auth event method mismatch")
	ErrInvalidSig      = errors.New("auth event signature invalid")
	ErrNotAdmin        = errors.New("pubkey not authorized as admin")
)

// ValidateNIP98Auth validates a NIP-98 HTTP Auth event from the Authorization header
// Returns the authenticated pubkey or an error
func ValidateNIP98Auth(r *http.Request, adminPubkeys []string) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", ErrMissingAuth
	}

	if !strings.HasPrefix(authHeader, "Nostr ") {
		return "", ErrInvalidAuth
	}

	// Decode base64 event
	eventB64 := strings.TrimPrefix(authHeader, "Nostr ")
	eventJSON, err := base64.StdEncoding.DecodeString(eventB64)
	if err != nil {
		return "", ErrInvalidAuth
	}

	// Parse event
	var event nostr.Event
	if err := json.Unmarshal(eventJSON, &event); err != nil {
		return "", ErrInvalidEvent
	}

	// Validate event kind (must be 27235)
	if event.Kind != AuthEventKind {
		return "", ErrInvalidEvent
	}

	// Validate timestamp (must be within 60 seconds)
	eventTime := event.CreatedAt.Time()
	now := time.Now()
	if now.Sub(eventTime) > MaxAuthEventAge || eventTime.Sub(now) > MaxAuthEventAge {
		return "", ErrExpiredAuth
	}

	// Validate "u" tag (URL must match)
	uTag := event.Tags.Find("u")
	if len(uTag) < 2 {
		return "", ErrInvalidURL
	}
	requestURL := getRequestURL(r)
	if uTag[1] != requestURL {
		return "", ErrInvalidURL
	}

	// Validate "method" tag (HTTP method must match)
	methodTag := event.Tags.Find("method")
	if len(methodTag) < 2 {
		return "", ErrInvalidMethod
	}
	if strings.ToUpper(methodTag[1]) != r.Method {
		return "", ErrInvalidMethod
	}

	// Validate signature
	valid, err := event.CheckSignature()
	if err != nil || !valid {
		return "", ErrInvalidSig
	}

	// Validate pubkey is in admin list
	if !slices.Contains(adminPubkeys, event.PubKey) {
		return "", ErrNotAdmin
	}

	return event.PubKey, nil
}

// getRequestURL reconstructs the full request URL for validation
func getRequestURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	// Check for forwarded protocol (behind reverse proxy)
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}

	host := r.Host
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		host = fwdHost
	}

	return scheme + "://" + host + r.URL.Path
}

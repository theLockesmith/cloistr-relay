package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/config"
)

// TestRejectInvalidEvents_ValidEvent tests that valid events are accepted
func TestRejectInvalidEvents_ValidEvent(t *testing.T) {
	ctx := context.Background()
	event := createValidEvent(t)

	reject, msg := rejectInvalidEvents(ctx, event)
	if reject {
		t.Errorf("Valid event was rejected: %s", msg)
	}
	if msg != "" {
		t.Errorf("Valid event returned message: %s", msg)
	}
}

// TestRejectInvalidEvents_InvalidSignature tests rejection of events with invalid signatures
func TestRejectInvalidEvents_InvalidSignature(t *testing.T) {
	ctx := context.Background()
	event := createValidEvent(t)

	// Corrupt the signature by changing one byte
	event.Sig = corruptHex(event.Sig)

	reject, msg := rejectInvalidEvents(ctx, event)
	if !reject {
		t.Error("Event with invalid signature was not rejected")
	}
	if msg != "invalid: signature verification failed" {
		t.Errorf("Wrong rejection message: got %s, want 'invalid: signature verification failed'", msg)
	}
}

// TestRejectInvalidEvents_MismatchedID tests rejection of events with mismatched IDs
func TestRejectInvalidEvents_MismatchedID(t *testing.T) {
	ctx := context.Background()
	event := createValidEvent(t)

	// Corrupt the ID
	event.ID = corruptHex(event.ID)

	reject, msg := rejectInvalidEvents(ctx, event)
	if !reject {
		t.Error("Event with mismatched ID was not rejected")
	}
	if msg != "invalid: event id mismatch" {
		t.Errorf("Wrong rejection message: got %s, want 'invalid: event id mismatch'", msg)
	}
}

// TestRejectTimestampOutOfRange_FutureTimestamp tests rejection of events far in the future via NIP-22
func TestRejectTimestampOutOfRange_FutureTimestamp(t *testing.T) {
	ctx := context.Background()
	event := createValidEvent(t)

	// Set timestamp far in the future (6 minutes ahead, limit is 5 minutes / 300 seconds)
	event.CreatedAt = nostr.Timestamp(time.Now().Unix() + 360)
	_ = event.Sign(testPrivateKey)

	cfg := &config.Config{
		MaxCreatedAtFuture: 300, // 5 minutes
		MaxCreatedAtPast:   0,   // unlimited
	}

	handler := rejectTimestampOutOfRange(cfg)
	reject, msg := handler(ctx, event)
	if !reject {
		t.Error("Event with future timestamp was not rejected")
	}
	if msg == "" {
		t.Error("Wrong rejection message: got empty string")
	}
}

// TestRejectTimestampOutOfRange_RecentFutureTimestamp tests that events within tolerance are accepted
func TestRejectTimestampOutOfRange_RecentFutureTimestamp(t *testing.T) {
	ctx := context.Background()
	event := createValidEvent(t)

	// Set timestamp slightly in the future (2 minutes, within 5 minute tolerance)
	event.CreatedAt = nostr.Timestamp(time.Now().Unix() + 120)
	_ = event.Sign(testPrivateKey)

	cfg := &config.Config{
		MaxCreatedAtFuture: 300, // 5 minutes
		MaxCreatedAtPast:   0,   // unlimited
	}

	handler := rejectTimestampOutOfRange(cfg)
	reject, msg := handler(ctx, event)
	if reject {
		t.Errorf("Event within timestamp tolerance was rejected: %s", msg)
	}
}

// TestRejectTimestampOutOfRange_PastTimestamp tests that past events within limit are accepted
func TestRejectTimestampOutOfRange_PastTimestamp(t *testing.T) {
	ctx := context.Background()
	event := createValidEvent(t)

	// Set timestamp in the past (1 hour)
	event.CreatedAt = nostr.Timestamp(time.Now().Unix() - 3600)
	_ = event.Sign(testPrivateKey)

	cfg := &config.Config{
		MaxCreatedAtFuture: 300, // 5 minutes
		MaxCreatedAtPast:   0,   // unlimited
	}

	handler := rejectTimestampOutOfRange(cfg)
	reject, msg := handler(ctx, event)
	if reject {
		t.Errorf("Past event was rejected: %s", msg)
	}
}

// TestRejectTimestampOutOfRange_PastTimestampWithLimit tests rejection of old events when limit is set
func TestRejectTimestampOutOfRange_PastTimestampWithLimit(t *testing.T) {
	ctx := context.Background()
	event := createValidEvent(t)

	// Set timestamp too far in the past (2 hours, limit is 1 hour)
	event.CreatedAt = nostr.Timestamp(time.Now().Unix() - 7200)
	_ = event.Sign(testPrivateKey)

	cfg := &config.Config{
		MaxCreatedAtFuture: 300,  // 5 minutes
		MaxCreatedAtPast:   3600, // 1 hour
	}

	handler := rejectTimestampOutOfRange(cfg)
	reject, msg := handler(ctx, event)
	if !reject {
		t.Error("Old event should have been rejected")
	}
	if msg == "" {
		t.Error("Wrong rejection message: got empty string")
	}
}

// TestRejectComplexFilters_ValidFilter tests that reasonable filters are accepted
func TestRejectComplexFilters_ValidFilter(t *testing.T) {
	ctx := context.Background()
	filter := nostr.Filter{
		Authors: []string{"pubkey1", "pubkey2", "pubkey3"},
		IDs:     []string{"id1", "id2", "id3"},
		Kinds:   []int{1, 2, 3},
	}

	reject, msg := rejectComplexFilters(ctx, filter)
	if reject {
		t.Errorf("Valid filter was rejected: %s", msg)
	}
}

// TestRejectComplexFilters_EmptyFilter tests that empty filters are accepted
func TestRejectComplexFilters_EmptyFilter(t *testing.T) {
	ctx := context.Background()
	filter := nostr.Filter{}

	reject, msg := rejectComplexFilters(ctx, filter)
	if reject {
		t.Errorf("Empty filter was rejected: %s", msg)
	}
}

// TestRejectComplexFilters_TooManyAuthors tests rejection of filters with too many authors
func TestRejectComplexFilters_TooManyAuthors(t *testing.T) {
	ctx := context.Background()

	// Create filter with 101 authors (limit is 100)
	authors := make([]string, 101)
	for i := 0; i < 101; i++ {
		authors[i] = generateRandomHex(64)
	}

	filter := nostr.Filter{Authors: authors}

	reject, msg := rejectComplexFilters(ctx, filter)
	if !reject {
		t.Error("Filter with too many authors was not rejected")
	}
	if msg != "error: too many authors in filter" {
		t.Errorf("Wrong rejection message: got %s, want 'error: too many authors in filter'", msg)
	}
}

// TestRejectComplexFilters_ExactlyMaxAuthors tests filter with exactly max authors
func TestRejectComplexFilters_ExactlyMaxAuthors(t *testing.T) {
	ctx := context.Background()

	// Create filter with exactly 100 authors (at the limit)
	authors := make([]string, 100)
	for i := 0; i < 100; i++ {
		authors[i] = generateRandomHex(64)
	}

	filter := nostr.Filter{Authors: authors}

	reject, msg := rejectComplexFilters(ctx, filter)
	if reject {
		t.Errorf("Filter with exactly max authors was rejected: %s", msg)
	}
}

// TestRejectComplexFilters_TooManyIDs tests rejection of filters with too many IDs
func TestRejectComplexFilters_TooManyIDs(t *testing.T) {
	ctx := context.Background()

	// Create filter with 501 IDs (limit is 500)
	ids := make([]string, 501)
	for i := 0; i < 501; i++ {
		ids[i] = generateRandomHex(64)
	}

	filter := nostr.Filter{IDs: ids}

	reject, msg := rejectComplexFilters(ctx, filter)
	if !reject {
		t.Error("Filter with too many IDs was not rejected")
	}
	if msg != "error: too many ids in filter" {
		t.Errorf("Wrong rejection message: got %s, want 'error: too many ids in filter'", msg)
	}
}

// TestRejectComplexFilters_ExactlyMaxIDs tests filter with exactly max IDs
func TestRejectComplexFilters_ExactlyMaxIDs(t *testing.T) {
	ctx := context.Background()

	// Create filter with exactly 500 IDs (at the limit)
	ids := make([]string, 500)
	for i := 0; i < 500; i++ {
		ids[i] = generateRandomHex(64)
	}

	filter := nostr.Filter{IDs: ids}

	reject, msg := rejectComplexFilters(ctx, filter)
	if reject {
		t.Errorf("Filter with exactly max IDs was rejected: %s", msg)
	}
}

// TestRejectComplexFilters_TooManyKinds tests rejection of filters with too many kinds
func TestRejectComplexFilters_TooManyKinds(t *testing.T) {
	ctx := context.Background()

	// Create filter with 21 kinds (limit is 20)
	kinds := make([]int, 21)
	for i := 0; i < 21; i++ {
		kinds[i] = i
	}

	filter := nostr.Filter{Kinds: kinds}

	reject, msg := rejectComplexFilters(ctx, filter)
	if !reject {
		t.Error("Filter with too many kinds was not rejected")
	}
	if msg != "error: too many kinds in filter" {
		t.Errorf("Wrong rejection message: got %s, want 'error: too many kinds in filter'", msg)
	}
}

// TestRejectComplexFilters_ExactlyMaxKinds tests filter with exactly max kinds
func TestRejectComplexFilters_ExactlyMaxKinds(t *testing.T) {
	ctx := context.Background()

	// Create filter with exactly 20 kinds (at the limit)
	kinds := make([]int, 20)
	for i := 0; i < 20; i++ {
		kinds[i] = i
	}

	filter := nostr.Filter{Kinds: kinds}

	reject, msg := rejectComplexFilters(ctx, filter)
	if reject {
		t.Errorf("Filter with exactly max kinds was rejected: %s", msg)
	}
}

// TestRejectComplexFilters_MultipleViolations tests filter with multiple violations
func TestRejectComplexFilters_MultipleViolations(t *testing.T) {
	ctx := context.Background()

	// Create filter with violations in all categories
	authors := make([]string, 101)
	for i := 0; i < 101; i++ {
		authors[i] = generateRandomHex(64)
	}

	ids := make([]string, 501)
	for i := 0; i < 501; i++ {
		ids[i] = generateRandomHex(64)
	}

	kinds := make([]int, 21)
	for i := 0; i < 21; i++ {
		kinds[i] = i
	}

	filter := nostr.Filter{
		Authors: authors,
		IDs:     ids,
		Kinds:   kinds,
	}

	reject, msg := rejectComplexFilters(ctx, filter)
	if !reject {
		t.Error("Filter with multiple violations was not rejected")
	}
	// Should fail on first check (authors)
	if msg != "error: too many authors in filter" {
		t.Errorf("Wrong rejection message: got %s", msg)
	}
}

// Test private key for creating valid test events
var testPrivateKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// createValidEvent creates a properly signed valid Nostr event for testing
func createValidEvent(t *testing.T) *nostr.Event {
	t.Helper()

	event := &nostr.Event{
		PubKey:    mustGetPublicKey(t, testPrivateKey),
		CreatedAt: nostr.Now(),
		Kind:      1,
		Tags:      nostr.Tags{},
		Content:   "Test event content",
	}

	err := event.Sign(testPrivateKey)
	if err != nil {
		t.Fatalf("Failed to sign test event: %v", err)
	}

	return event
}

// mustGetPublicKey gets the public key from a private key or fails the test
func mustGetPublicKey(t *testing.T, privateKey string) string {
	t.Helper()

	pubkey, err := nostr.GetPublicKey(privateKey)
	if err != nil {
		t.Fatalf("Failed to get public key: %v", err)
	}

	return pubkey
}

// corruptHex changes one character in a hex string to corrupt it
func corruptHex(hexStr string) string {
	if len(hexStr) == 0 {
		return hexStr
	}

	// Change the first character to a different valid hex digit
	firstChar := hexStr[0]
	var newChar byte
	if firstChar == 'a' {
		newChar = 'b'
	} else {
		newChar = 'a'
	}

	return string(newChar) + hexStr[1:]
}

// generateRandomHex generates a random hex string of specified length
func generateRandomHex(length int) string {
	bytes := make([]byte, length/2)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

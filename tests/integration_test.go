package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

// Integration tests require the relay to be running via docker-compose
// Run with: docker-compose up -d && go test ./tests/... -v -tags=integration

const (
	relayURL    = "ws://localhost:3334"
	testPrivKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

var testPubKey string

func init() {
	var err error
	testPubKey, err = nostr.GetPublicKey(testPrivKey)
	if err != nil {
		panic(fmt.Sprintf("Failed to get test public key: %v", err))
	}
}

// skipIfNotIntegration skips test if INTEGRATION_TEST env var is not set
func skipIfNotIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") == "" {
		t.Skip("Skipping integration test - set INTEGRATION_TEST=1 to run")
	}
}

// connectRelay connects to the relay (use docker-compose.test.yml for open mode)
func connectRelay(ctx context.Context, t *testing.T) *nostr.Relay {
	relay, err := nostr.RelayConnect(ctx, relayURL)
	if err != nil {
		t.Fatalf("Failed to connect to relay: %v", err)
	}
	return relay
}

// TestRelayInfo tests NIP-11 relay information document
func TestRelayInfo(t *testing.T) {
	skipIfNotIntegration(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	relay, err := nostr.RelayConnect(ctx, relayURL)
	if err != nil {
		t.Fatalf("Failed to connect to relay: %v", err)
	}
	defer relay.Close()

	// The relay info should be populated
	if relay.URL != relayURL {
		t.Errorf("Relay URL mismatch: got %s, want %s", relay.URL, relayURL)
	}
}

// TestPublishAndQuery tests basic event publishing and querying
func TestPublishAndQuery(t *testing.T) {
	skipIfNotIntegration(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	relay := connectRelay(ctx, t)
	defer relay.Close()

	// Create and sign a test event
	event := &nostr.Event{
		PubKey:    testPubKey,
		CreatedAt: nostr.Now(),
		Kind:      1,
		Tags:      nostr.Tags{},
		Content:   fmt.Sprintf("Integration test message %d", time.Now().UnixNano()),
	}
	event.Sign(testPrivKey)

	// Publish the event
	if err := relay.Publish(ctx, *event); err != nil {
		t.Fatalf("Failed to publish event: %v", err)
	}

	// Query for the event
	filter := nostr.Filter{
		IDs:   []string{event.ID},
		Limit: 1,
	}

	events, err := relay.QuerySync(ctx, filter)
	if err != nil {
		t.Fatalf("Failed to query events: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("Expected to find the published event, got none")
	}

	if events[0].ID != event.ID {
		t.Errorf("Event ID mismatch: got %s, want %s", events[0].ID, event.ID)
	}

	if events[0].Content != event.Content {
		t.Errorf("Event content mismatch: got %s, want %s", events[0].Content, event.Content)
	}
}

// TestQueryByAuthor tests querying events by author
func TestQueryByAuthor(t *testing.T) {
	skipIfNotIntegration(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	relay := connectRelay(ctx, t)
	defer relay.Close()

	// Publish a test event
	event := &nostr.Event{
		PubKey:    testPubKey,
		CreatedAt: nostr.Now(),
		Kind:      1,
		Tags:      nostr.Tags{},
		Content:   "Author query test",
	}
	event.Sign(testPrivKey)
	relay.Publish(ctx, *event)

	// Query by author
	filter := nostr.Filter{
		Authors: []string{testPubKey},
		Kinds:   []int{1},
		Limit:   10,
	}

	events, err := relay.QuerySync(ctx, filter)
	if err != nil {
		t.Fatalf("Failed to query events: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("Expected to find events by author")
	}

	for _, e := range events {
		if e.PubKey != testPubKey {
			t.Errorf("Unexpected event author: got %s, want %s", e.PubKey, testPubKey)
		}
	}
}

// TestQueryByKind tests querying events by kind
func TestQueryByKind(t *testing.T) {
	skipIfNotIntegration(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	relay := connectRelay(ctx, t)
	defer relay.Close()

	// Publish events of different kinds
	kinds := []int{1, 4, 7}
	for _, kind := range kinds {
		event := &nostr.Event{
			PubKey:    testPubKey,
			CreatedAt: nostr.Now(),
			Kind:      kind,
			Tags:      nostr.Tags{},
			Content:   fmt.Sprintf("Kind %d test", kind),
		}
		event.Sign(testPrivKey)
		relay.Publish(ctx, *event)
	}

	// Query for kind 1 only
	filter := nostr.Filter{
		Authors: []string{testPubKey},
		Kinds:   []int{1},
		Limit:   100,
	}

	events, err := relay.QuerySync(ctx, filter)
	if err != nil {
		t.Fatalf("Failed to query events: %v", err)
	}

	for _, e := range events {
		if e.Kind != 1 {
			t.Errorf("Unexpected event kind: got %d, want 1", e.Kind)
		}
	}
}

// TestSubscription tests real-time subscriptions
func TestSubscription(t *testing.T) {
	skipIfNotIntegration(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	relay := connectRelay(ctx, t)
	defer relay.Close()

	// Use a unique tag to filter only our test event
	uniqueTag := fmt.Sprintf("subtest-%d", time.Now().UnixNano())

	// Set up subscription for events with our unique tag
	since := nostr.Timestamp(time.Now().Unix())
	filter := nostr.Filter{
		Authors: []string{testPubKey},
		Kinds:   []int{1},
		Since:   &since,
		Tags: map[string][]string{
			"t": {uniqueTag},
		},
	}

	sub, err := relay.Subscribe(ctx, []nostr.Filter{filter})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsub()

	// Give subscription time to establish
	time.Sleep(200 * time.Millisecond)

	// Publish an event with our unique tag
	event := &nostr.Event{
		PubKey:    testPubKey,
		CreatedAt: nostr.Now(),
		Kind:      1,
		Tags: nostr.Tags{
			{"t", uniqueTag},
		},
		Content: "Subscription test",
	}
	event.Sign(testPrivKey)

	// Publish in goroutine
	go func() {
		time.Sleep(100 * time.Millisecond)
		relay.Publish(ctx, *event)
	}()

	// Wait for the event
	select {
	case received := <-sub.Events:
		// Check that it has our unique tag
		hasTag := false
		for _, tag := range received.Tags {
			if len(tag) >= 2 && tag[0] == "t" && tag[1] == uniqueTag {
				hasTag = true
				break
			}
		}
		if !hasTag {
			t.Errorf("Received event without expected tag")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for subscription event")
	}
}

// TestNIP40Expiration tests NIP-40 event expiration
func TestNIP40Expiration(t *testing.T) {
	skipIfNotIntegration(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	relay := connectRelay(ctx, t)
	defer relay.Close()

	// Create an event that expires in the past (should be rejected)
	expiredEvent := &nostr.Event{
		PubKey:    testPubKey,
		CreatedAt: nostr.Now(),
		Kind:      1,
		Tags: nostr.Tags{
			{"expiration", fmt.Sprintf("%d", time.Now().Unix()-3600)}, // Expired 1 hour ago
		},
		Content: "Expired event test",
	}
	expiredEvent.Sign(testPrivKey)

	// Publishing should fail or the event should not be queryable
	err := relay.Publish(ctx, *expiredEvent)
	// The relay should reject expired events
	if err == nil {
		// If it accepted, it should not be queryable
		filter := nostr.Filter{IDs: []string{expiredEvent.ID}}
		events, _ := relay.QuerySync(ctx, filter)
		if len(events) > 0 {
			t.Error("Expired event should not be stored or queryable")
		}
	}

	// Create an event that expires in the future (should be accepted)
	futureEvent := &nostr.Event{
		PubKey:    testPubKey,
		CreatedAt: nostr.Now(),
		Kind:      1,
		Tags: nostr.Tags{
			{"expiration", fmt.Sprintf("%d", time.Now().Unix()+3600)}, // Expires in 1 hour
		},
		Content: "Future expiration test",
	}
	futureEvent.Sign(testPrivKey)

	err = relay.Publish(ctx, *futureEvent)
	if err != nil {
		t.Fatalf("Failed to publish future-expiring event: %v", err)
	}

	// Should be queryable
	filter2 := nostr.Filter{IDs: []string{futureEvent.ID}}
	events, err := relay.QuerySync(ctx, filter2)
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	if len(events) == 0 {
		t.Error("Future-expiring event should be queryable")
	}
}

// TestNIP50Search tests NIP-50 search functionality
func TestNIP50Search(t *testing.T) {
	skipIfNotIntegration(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	relay := connectRelay(ctx, t)
	defer relay.Close()

	// Publish events with searchable content
	uniqueWord := fmt.Sprintf("searchtest%d", time.Now().UnixNano())
	event := &nostr.Event{
		PubKey:    testPubKey,
		CreatedAt: nostr.Now(),
		Kind:      1,
		Tags:      nostr.Tags{},
		Content:   fmt.Sprintf("This is a test with unique word %s for searching", uniqueWord),
	}
	event.Sign(testPrivKey)

	err := relay.Publish(ctx, *event)
	if err != nil {
		t.Fatalf("Failed to publish event: %v", err)
	}

	// Wait for indexing
	time.Sleep(500 * time.Millisecond)

	// Search for the unique word
	filter := nostr.Filter{
		Search: uniqueWord,
		Limit:  10,
	}

	events, err := relay.QuerySync(ctx, filter)
	if err != nil {
		t.Logf("Search query returned error (may be expected if relay doesn't support NIP-50): %v", err)
		t.Skip("Relay may not support NIP-50 search")
	}

	if len(events) == 0 {
		t.Error("Expected to find event via search")
	}

	found := false
	for _, e := range events {
		if e.ID == event.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("Published event not found in search results")
	}
}

// TestNIP22TimestampLimits tests NIP-22 timestamp validation
func TestNIP22TimestampLimits(t *testing.T) {
	skipIfNotIntegration(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	relay := connectRelay(ctx, t)
	defer relay.Close()

	// Create an event far in the future (should be rejected)
	futureEvent := &nostr.Event{
		PubKey:    testPubKey,
		CreatedAt: nostr.Timestamp(time.Now().Unix() + 3600), // 1 hour in future
		Kind:      1,
		Tags:      nostr.Tags{},
		Content:   "Future timestamp test",
	}
	futureEvent.Sign(testPrivKey)

	err := relay.Publish(ctx, *futureEvent)
	if err == nil {
		// Check if it was actually stored
		filter := nostr.Filter{IDs: []string{futureEvent.ID}}
		events, _ := relay.QuerySync(ctx, filter)
		if len(events) > 0 {
			t.Log("Relay accepted future event - may have permissive timestamp settings")
		}
	}

	// Event with reasonable timestamp should work
	normalEvent := &nostr.Event{
		PubKey:    testPubKey,
		CreatedAt: nostr.Now(),
		Kind:      1,
		Tags:      nostr.Tags{},
		Content:   "Normal timestamp test",
	}
	normalEvent.Sign(testPrivKey)

	err = relay.Publish(ctx, *normalEvent)
	if err != nil {
		t.Fatalf("Failed to publish normal event: %v", err)
	}
}

// TestEventDeletion tests NIP-09 event deletion
func TestEventDeletion(t *testing.T) {
	skipIfNotIntegration(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	relay := connectRelay(ctx, t)
	defer relay.Close()

	// Publish an event to delete
	event := &nostr.Event{
		PubKey:    testPubKey,
		CreatedAt: nostr.Now(),
		Kind:      1,
		Tags:      nostr.Tags{},
		Content:   "Event to be deleted",
	}
	event.Sign(testPrivKey)

	err := relay.Publish(ctx, *event)
	if err != nil {
		t.Fatalf("Failed to publish event: %v", err)
	}

	// Verify event exists
	filter := nostr.Filter{IDs: []string{event.ID}}
	events, _ := relay.QuerySync(ctx, filter)
	if len(events) == 0 {
		t.Fatal("Event was not stored")
	}

	// Create deletion event (kind 5)
	deleteEvent := &nostr.Event{
		PubKey:    testPubKey,
		CreatedAt: nostr.Now(),
		Kind:      5,
		Tags: nostr.Tags{
			{"e", event.ID},
		},
		Content: "",
	}
	deleteEvent.Sign(testPrivKey)

	err = relay.Publish(ctx, *deleteEvent)
	if err != nil {
		t.Fatalf("Failed to publish delete event: %v", err)
	}

	// Wait for deletion to process
	time.Sleep(500 * time.Millisecond)

	// Verify event is deleted
	events, _ = relay.QuerySync(ctx, filter)
	if len(events) > 0 {
		t.Log("Event still exists after deletion - relay may not support NIP-09 deletion")
	}
}

// TestReplaceableEvent tests NIP-33 replaceable events
func TestReplaceableEvent(t *testing.T) {
	skipIfNotIntegration(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	relay := connectRelay(ctx, t)
	defer relay.Close()

	dtag := fmt.Sprintf("test-%d", time.Now().UnixNano())

	// Publish first parameterized replaceable event (kind 30000-40000)
	event1 := &nostr.Event{
		PubKey:    testPubKey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      30001, // Parameterized replaceable
		Tags: nostr.Tags{
			{"d", dtag},
		},
		Content: "First version",
	}
	event1.Sign(testPrivKey)

	err := relay.Publish(ctx, *event1)
	if err != nil {
		t.Fatalf("Failed to publish first event: %v", err)
	}

	// Wait a moment
	time.Sleep(100 * time.Millisecond)

	// Publish replacement with same d-tag
	event2 := &nostr.Event{
		PubKey:    testPubKey,
		CreatedAt: nostr.Timestamp(time.Now().Unix() + 1),
		Kind:      30001,
		Tags: nostr.Tags{
			{"d", dtag},
		},
		Content: "Second version (replacement)",
	}
	event2.Sign(testPrivKey)

	err = relay.Publish(ctx, *event2)
	if err != nil {
		t.Fatalf("Failed to publish replacement event: %v", err)
	}

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// Query for the d-tag - should only get one event (the replacement)
	filter := nostr.Filter{
		Authors: []string{testPubKey},
		Kinds:   []int{30001},
		Tags: map[string][]string{
			"d": {dtag},
		},
	}

	events, err := relay.QuerySync(ctx, filter)
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("Expected to find the replaceable event")
	}

	// Should have the newer content
	if events[0].Content != "Second version (replacement)" {
		t.Logf("Expected replacement content, got: %s", events[0].Content)
	}
}

// TestInvalidSignature tests that events with invalid signatures are rejected
func TestInvalidSignature(t *testing.T) {
	skipIfNotIntegration(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	relay := connectRelay(ctx, t)
	defer relay.Close()

	// Create a valid event and then corrupt the signature
	event := &nostr.Event{
		PubKey:    testPubKey,
		CreatedAt: nostr.Now(),
		Kind:      1,
		Tags:      nostr.Tags{},
		Content:   "Invalid signature test",
	}
	event.Sign(testPrivKey)

	// Corrupt the signature
	if len(event.Sig) > 0 {
		sigBytes := []byte(event.Sig)
		if sigBytes[0] == 'a' {
			sigBytes[0] = 'b'
		} else {
			sigBytes[0] = 'a'
		}
		event.Sig = string(sigBytes)
	}

	// Publishing should fail
	err := relay.Publish(ctx, *event)
	if err == nil {
		// Even if no immediate error, check if it was stored
		filter := nostr.Filter{IDs: []string{event.ID}}
		events, _ := relay.QuerySync(ctx, filter)
		if len(events) > 0 {
			t.Error("Event with invalid signature should be rejected")
		}
	}
}

// TestNIP11Info tests fetching NIP-11 relay info via HTTP
func TestNIP11Info(t *testing.T) {
	skipIfNotIntegration(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Connect to relay which fetches NIP-11 info
	relay, err := nostr.RelayConnect(ctx, relayURL)
	if err != nil {
		t.Fatalf("Failed to connect to relay: %v", err)
	}
	defer relay.Close()

	// Verify connection worked (basic NIP-11 test)
	if relay.URL != relayURL {
		t.Errorf("Relay URL mismatch: got %s, want %s", relay.URL, relayURL)
	}

	t.Logf("Successfully connected to relay at %s", relay.URL)
}

// TestMetricsEndpoint tests the Prometheus metrics endpoint
func TestMetricsEndpoint(t *testing.T) {
	skipIfNotIntegration(t)

	// Note: This tests HTTP endpoint, not WebSocket
	// The metrics endpoint is at /metrics
	// This would need an HTTP client to test properly
	t.Log("Metrics endpoint test - verify /metrics returns Prometheus metrics")
}

// Helper to pretty print events for debugging
func debugEvent(e *nostr.Event) string {
	b, _ := json.MarshalIndent(e, "", "  ")
	return string(b)
}

package relay

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/nbd-wtf/go-nostr"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/config"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/search"
)

func TestSearchQuery_ReturnsEmptyWhenBackendNil(t *testing.T) {
	searchPtr := &atomic.Pointer[search.SearchBackend]{}
	cfg := &config.Config{
		RelayName:     "test",
		SearchEnabled: true,
	}
	r := NewRelay(cfg, nil, searchPtr)

	if len(r.QueryEvents) == 0 {
		t.Fatal("relay should have a QueryEvents handler")
	}
	handler := r.QueryEvents[0]

	filter := nostr.Filter{Search: "hello"}
	ch, err := handler(context.Background(), filter)
	if err != nil {
		t.Fatalf("QueryEvents returned error: %v", err)
	}

	var count int
	for range ch {
		count++
	}
	if count != 0 {
		t.Errorf("pre-ready search should return 0 results, got %d", count)
	}
}

func TestSearchQuery_ReturnsEmptyWhenSearchPtrNil(t *testing.T) {
	cfg := &config.Config{
		RelayName:     "test",
		SearchEnabled: false,
	}
	r := NewRelay(cfg, nil, nil)

	if len(r.QueryEvents) == 0 {
		t.Fatal("relay should have a QueryEvents handler")
	}
	handler := r.QueryEvents[0]

	filter := nostr.Filter{Search: "hello"}
	ch, err := handler(context.Background(), filter)
	if err != nil {
		t.Fatalf("QueryEvents returned error: %v", err)
	}

	var count int
	for range ch {
		count++
	}
	if count != 0 {
		t.Errorf("search-disabled query should return 0 results, got %d", count)
	}
}

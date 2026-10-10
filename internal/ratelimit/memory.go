package ratelimit

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/clientip"
	"github.com/nbd-wtf/go-nostr"
)

// In-memory per-IP limiters. These replace khatru's policies.EventIPRateLimiter,
// FilterIPRateLimiter and ConnectionRateLimiter, which key on khatru.GetIP —
// the leftmost public X-Forwarded-For entry, which any client can forge. Same
// bucket semantics as khatru's: each key may spend maxTokens, refilled by
// tokensPerInterval every interval. The key comes from clientip instead.

func newMemoryBuckets(tokensPerInterval int, interval time.Duration, maxTokens int) func(key string) (limited bool) {
	var buckets sync.Map // key -> *atomic.Int32 (tokens spent)
	go func() {
		for {
			time.Sleep(interval)
			buckets.Range(func(k, v any) bool {
				if v.(*atomic.Int32).Add(int32(-tokensPerInterval)) <= 0 {
					buckets.Delete(k)
				}
				return true
			})
		}
	}()
	max := int32(maxTokens)
	return func(key string) bool {
		v, _ := buckets.LoadOrStore(key, &atomic.Int32{})
		spent := v.(*atomic.Int32)
		if spent.Load() < max {
			spent.Add(1)
			return false
		}
		return true
	}
}

// EventIPRateLimiter rate limits events per client IP.
func EventIPRateLimiter(tokensPerInterval int, interval time.Duration, maxTokens int) func(context.Context, *nostr.Event) (bool, string) {
	limited := newMemoryBuckets(tokensPerInterval, interval, maxTokens)
	return func(ctx context.Context, _ *nostr.Event) (bool, string) {
		ip := clientip.FromContext(ctx)
		if ip == "" || !limited(ip) {
			return false, ""
		}
		return true, "rate-limited: slow down, please"
	}
}

// FilterIPRateLimiter rate limits filter queries per client IP.
func FilterIPRateLimiter(tokensPerInterval int, interval time.Duration, maxTokens int) func(context.Context, nostr.Filter) (bool, string) {
	limited := newMemoryBuckets(tokensPerInterval, interval, maxTokens)
	return func(ctx context.Context, _ nostr.Filter) (bool, string) {
		ip := clientip.FromContext(ctx)
		if ip == "" || !limited(ip) {
			return false, ""
		}
		return true, "rate-limited: there is a bug in the client, no one should be making so many requests"
	}
}

// ConnectionRateLimiter rate limits new connections per client IP.
func ConnectionRateLimiter(tokensPerInterval int, interval time.Duration, maxTokens int) func(*http.Request) bool {
	limited := newMemoryBuckets(tokensPerInterval, interval, maxTokens)
	return func(r *http.Request) bool {
		ip := clientip.FromRequest(r)
		return ip != "" && limited(ip)
	}
}

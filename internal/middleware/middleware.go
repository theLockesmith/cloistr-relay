package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"
	"git.coldforge.xyz/coldforge/cloistr-relay/internal/logging"
	"git.coldforge.xyz/coldforge/cloistr-relay/internal/tracing"
)

var connectionCounter uint64

// GenerateConnectionID creates a unique connection ID
func GenerateConnectionID() string {
	// Incrementing counter + random suffix for uniqueness across restarts
	count := atomic.AddUint64(&connectionCounter, 1)
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString([]byte{
		byte(count >> 24), byte(count >> 16), byte(count >> 8), byte(count),
	}) + hex.EncodeToString(b)
}

// RegisterObservability registers logging and tracing hooks with the relay
func RegisterObservability(relay *khatru.Relay) {
	logger := logging.Default().WithComponent("relay")

	// Connection logging
	relay.OnConnect = append(relay.OnConnect, func(ctx context.Context) {
		connID := GenerateConnectionID()
		ip := getClientIP(ctx)

		// Store connection ID in context for later use
		// Note: khatru manages its own context, so we log here
		logger.Info(ctx, "client connected", map[string]interface{}{
			"connection_id": connID,
			"remote_ip":     ip,
		})
	})

	relay.OnDisconnect = append(relay.OnDisconnect, func(ctx context.Context) {
		ip := getClientIP(ctx)
		logger.Info(ctx, "client disconnected", map[string]interface{}{
			"remote_ip": ip,
		})
	})

	// Event processing observability
	originalRejectEvent := relay.RejectEvent
	relay.RejectEvent = nil

	relay.RejectEvent = append(relay.RejectEvent, func(ctx context.Context, event *nostr.Event) (bool, string) {
		ctx, span := tracing.StartSpan(ctx, "event.process")
		defer span.End()

		span.SetAttribute("event.id", event.ID[:16]+"...")
		span.SetAttribute("event.kind", event.Kind)
		span.SetAttribute("event.pubkey", event.PubKey[:16]+"...")

		start := time.Now()

		// Run original handlers
		for _, handler := range originalRejectEvent {
			if reject, msg := handler(ctx, event); reject {
				span.SetStatus("rejected")
				span.SetAttribute("reject.reason", msg)

				logger.Info(ctx, "event rejected", map[string]interface{}{
					"event_id":    event.ID[:16],
					"kind":        event.Kind,
					"reason":      msg,
					"duration_ms": float64(time.Since(start).Microseconds()) / 1000.0,
				})

				return reject, msg
			}
		}

		return false, ""
	})

	// Event saved logging
	relay.OnEventSaved = append(relay.OnEventSaved, func(ctx context.Context, event *nostr.Event) {
		logger.Debug(ctx, "event saved", map[string]interface{}{
			"event_id": event.ID[:16],
			"kind":     event.Kind,
			"pubkey":   event.PubKey[:16],
		})
	})

	// Query observability
	originalRejectFilter := relay.RejectFilter
	relay.RejectFilter = nil

	relay.RejectFilter = append(relay.RejectFilter, func(ctx context.Context, filter nostr.Filter) (bool, string) {
		ctx, span := tracing.StartSpan(ctx, "filter.process")
		defer span.End()

		span.SetAttribute("filter.kinds", filter.Kinds)
		span.SetAttribute("filter.authors_count", len(filter.Authors))
		span.SetAttribute("filter.limit", filter.Limit)

		start := time.Now()

		// Run original handlers
		for _, handler := range originalRejectFilter {
			if reject, msg := handler(ctx, filter); reject {
				span.SetStatus("rejected")
				span.SetAttribute("reject.reason", msg)

				logger.Info(ctx, "filter rejected", map[string]interface{}{
					"kinds":       filter.Kinds,
					"reason":      msg,
					"duration_ms": float64(time.Since(start).Microseconds()) / 1000.0,
				})

				return reject, msg
			}
		}

		return false, ""
	})

	// Auth observability
	originalRejectConnection := relay.RejectConnection
	relay.RejectConnection = nil

	relay.RejectConnection = append(relay.RejectConnection, func(r *http.Request) bool {
		ip := extractIP(r)
		userAgent := r.Header.Get("User-Agent")

		for _, handler := range originalRejectConnection {
			if handler(r) {
				logger.Warn(context.Background(), "connection rejected", map[string]interface{}{
					"remote_ip":  ip,
					"user_agent": userAgent,
				})
				return true
			}
		}

		return false
	})
}

// getClientIP extracts client IP from khatru context
func getClientIP(ctx context.Context) string {
	// Try to get from khatru's context
	if ip := khatru.GetIP(ctx); ip != "" {
		return ip
	}
	return "unknown"
}

// extractIP extracts the real client IP from request headers
func extractIP(r *http.Request) string {
	// Check X-Forwarded-For header (from reverse proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Check CF-Connecting-IP (Cloudflare)
	if cfip := r.Header.Get("CF-Connecting-IP"); cfip != "" {
		return cfip
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

package metrics

import (
	"context"
	"net/http"
	"time"

	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// Connection metrics
	connectionsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nostr_relay_connections_total",
		Help: "Total number of WebSocket connections",
	})

	connectionsActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nostr_relay_connections_active",
		Help: "Number of active WebSocket connections",
	})

	// Event metrics
	eventsReceived = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nostr_relay_events_received_total",
		Help: "Total number of events received",
	}, []string{"kind"})

	eventsStored = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nostr_relay_events_stored_total",
		Help: "Total number of events successfully stored",
	}, []string{"kind"})

	eventsRejected = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nostr_relay_events_rejected_total",
		Help: "Total number of events rejected",
	}, []string{"reason"})

	// Query metrics
	queriesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nostr_relay_queries_total",
		Help: "Total number of filter queries",
	})

	queriesRejected = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nostr_relay_queries_rejected_total",
		Help: "Total number of queries rejected",
	}, []string{"reason"})

	// Authentication metrics
	authAttempts = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nostr_relay_auth_attempts_total",
		Help: "Total number of authentication attempts",
	})

	authSuccesses = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nostr_relay_auth_successes_total",
		Help: "Total number of successful authentications",
	})

	// NIP-46 metrics
	nip46MessagesRelayed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nostr_relay_nip46_messages_relayed_total",
		Help: "Total number of NIP-46 (Nostr Connect) messages relayed",
	})

	// Latency metrics
	eventProcessingDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "nostr_relay_event_processing_seconds",
		Help:    "Time spent processing events",
		Buckets: prometheus.DefBuckets,
	})

	queryProcessingDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "nostr_relay_query_processing_seconds",
		Help:    "Time spent processing queries",
		Buckets: prometheus.DefBuckets,
	})

	// Relay info
	relayInfo = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nostr_relay_info",
		Help: "Relay information",
	}, []string{"name", "version"})
)

// Handler returns the Prometheus metrics HTTP handler
func Handler() http.Handler {
	return promhttp.Handler()
}

// RegisterRelayMetrics registers metric collection hooks with the relay
func RegisterRelayMetrics(relay *khatru.Relay) {
	// Set relay info
	relayInfo.WithLabelValues(relay.Info.Name, "khatru").Set(1)

	// Track connections
	relay.OnConnect = append(relay.OnConnect, func(ctx context.Context) {
		connectionsTotal.Inc()
		connectionsActive.Inc()
	})

	relay.OnDisconnect = append(relay.OnDisconnect, func(ctx context.Context) {
		connectionsActive.Dec()
	})

	// Track event processing (wrap existing handlers)
	// We add these at the beginning to measure timing
	originalRejectEvent := relay.RejectEvent
	relay.RejectEvent = nil

	// Add timing wrapper
	relay.RejectEvent = append(relay.RejectEvent, func(ctx context.Context, event *nostr.Event) (bool, string) {
		start := time.Now()
		defer func() {
			eventProcessingDuration.Observe(time.Since(start).Seconds())
		}()

		kindStr := kindToString(event.Kind)
		eventsReceived.WithLabelValues(kindStr).Inc()

		// Run original reject handlers
		for _, handler := range originalRejectEvent {
			if reject, msg := handler(ctx, event); reject {
				eventsRejected.WithLabelValues(categorizeRejection(msg)).Inc()
				return reject, msg
			}
		}
		return false, ""
	})

	// Track successful storage
	relay.OnEventSaved = append(relay.OnEventSaved, func(ctx context.Context, event *nostr.Event) {
		kindStr := kindToString(event.Kind)
		eventsStored.WithLabelValues(kindStr).Inc()

		// Track auth events
		if event.Kind == 22242 {
			authAttempts.Inc()
			authSuccesses.Inc()
		}
	})

	// Track queries
	originalRejectFilter := relay.RejectFilter
	relay.RejectFilter = nil

	relay.RejectFilter = append(relay.RejectFilter, func(ctx context.Context, filter nostr.Filter) (bool, string) {
		start := time.Now()
		defer func() {
			queryProcessingDuration.Observe(time.Since(start).Seconds())
		}()

		queriesTotal.Inc()

		// Run original reject handlers
		for _, handler := range originalRejectFilter {
			if reject, msg := handler(ctx, filter); reject {
				queriesRejected.WithLabelValues(categorizeRejection(msg)).Inc()
				return reject, msg
			}
		}
		return false, ""
	})
}

// kindToString converts event kind to a string label
func kindToString(kind int) string {
	switch kind {
	case 0:
		return "metadata"
	case 1:
		return "text_note"
	case 3:
		return "contacts"
	case 4:
		return "dm"
	case 5:
		return "deletion"
	case 6:
		return "repost"
	case 7:
		return "reaction"
	case 22242:
		return "auth"
	default:
		if kind >= 10000 && kind < 20000 {
			return "replaceable"
		}
		if kind >= 20000 && kind < 30000 {
			return "ephemeral"
		}
		if kind >= 30000 && kind < 40000 {
			return "parameterized_replaceable"
		}
		return "other"
	}
}

// RecordNIP46Message increments the NIP-46 message counter
func RecordNIP46Message() {
	nip46MessagesRelayed.Inc()
}

// categorizeRejection extracts a category from rejection message
func categorizeRejection(msg string) string {
	if len(msg) > 20 {
		// Extract prefix before colon
		for i, c := range msg {
			if c == ':' {
				return msg[:i]
			}
		}
	}
	if len(msg) > 15 {
		return msg[:15]
	}
	return msg
}

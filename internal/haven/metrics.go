package haven

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// HAVEN Prometheus metrics
var (
	// Box routing metrics
	havenEventsRouted = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nostr_relay_haven_events_routed_total",
		Help: "Total number of events routed to HAVEN boxes",
	}, []string{"box"})

	havenEventsRejected = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nostr_relay_haven_events_rejected_total",
		Help: "Total number of events rejected by HAVEN box routing",
	}, []string{"box", "reason"})

	havenFiltersRouted = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nostr_relay_haven_filters_routed_total",
		Help: "Total number of filters routed to HAVEN boxes",
	}, []string{"box"})

	havenFiltersRejected = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nostr_relay_haven_filters_rejected_total",
		Help: "Total number of filters rejected by HAVEN access policies",
	}, []string{"box", "reason"})

	// Access metrics
	havenAccessAttempts = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nostr_relay_haven_access_attempts_total",
		Help: "Total number of box access attempts",
	}, []string{"box", "operation"})

	havenAccessDenied = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nostr_relay_haven_access_denied_total",
		Help: "Total number of box access denials",
	}, []string{"box", "operation", "reason"})

	// Blastr metrics
	havenBlastrEventsBroadcast = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nostr_relay_haven_blastr_events_broadcast_total",
		Help: "Total number of events successfully broadcast by Blastr",
	})

	havenBlastrEventsFailed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nostr_relay_haven_blastr_events_failed_total",
		Help: "Total number of events that failed to broadcast",
	})

	havenBlastrEventsQueued = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nostr_relay_haven_blastr_events_queued_total",
		Help: "Total number of events queued for broadcast",
	})

	havenBlastrEventsDropped = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nostr_relay_haven_blastr_events_dropped_total",
		Help: "Total number of events dropped due to full queue",
	})

	havenBlastrRelaysConnected = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nostr_relay_haven_blastr_relays_connected",
		Help: "Number of relays currently connected for broadcasting",
	})

	havenBlastrQueueSize = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nostr_relay_haven_blastr_queue_size",
		Help: "Current number of events in the broadcast queue",
	})

	havenBlastrRelayPublishTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nostr_relay_haven_blastr_relay_publish_total",
		Help: "Total publish attempts per relay",
	}, []string{"relay", "status"})

	// Blastr retry metrics
	havenBlastrRetryQueued = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nostr_relay_haven_blastr_retry_queued_total",
		Help: "Total number of failed broadcasts queued for retry",
	})

	havenBlastrRetrySuccess = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nostr_relay_haven_blastr_retry_success_total",
		Help: "Total number of successful retries",
	})

	havenBlastrRetryExhausted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nostr_relay_haven_blastr_retry_exhausted_total",
		Help: "Total number of retries that exhausted max attempts",
	})

	havenBlastrRetryQueueSize = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nostr_relay_haven_blastr_retry_queue_size",
		Help: "Current number of events in the retry queue",
	})

	// Importer metrics
	havenImporterEventsImported = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nostr_relay_haven_importer_events_imported_total",
		Help: "Total number of events imported from other relays",
	})

	havenImporterEventsSkipped = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nostr_relay_haven_importer_events_skipped_total",
		Help: "Total number of events skipped (duplicates or filtered)",
	})

	havenImporterFetchErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nostr_relay_haven_importer_fetch_errors_total",
		Help: "Total number of fetch errors from remote relays",
	})

	havenImporterRelaysPolled = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nostr_relay_haven_importer_relays_polled",
		Help: "Number of relays being polled for imports",
	})

	havenImporterLastPollTimestamp = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nostr_relay_haven_importer_last_poll_timestamp",
		Help: "Unix timestamp of the last poll cycle",
	})

	havenImporterRelayFetchTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nostr_relay_haven_importer_relay_fetch_total",
		Help: "Total fetch attempts per relay",
	}, []string{"relay", "status"})

	// Box event counts (gauges for current state, updated periodically)
	havenBoxEventsStored = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nostr_relay_haven_box_events_stored",
		Help: "Number of events stored in each HAVEN box (approximate)",
	}, []string{"box"})

	// HAVEN system status
	havenEnabled = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nostr_relay_haven_enabled",
		Help: "Whether HAVEN box routing is enabled (1=enabled, 0=disabled)",
	})

	havenBlastrEnabled = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nostr_relay_haven_blastr_enabled",
		Help: "Whether Blastr is enabled (1=enabled, 0=disabled)",
	})

	havenImporterEnabled = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nostr_relay_haven_importer_enabled",
		Help: "Whether Importer is enabled (1=enabled, 0=disabled)",
	})
)

// Metrics provides methods for recording HAVEN metrics
type Metrics struct{}

// NewMetrics creates a new Metrics instance
func NewMetrics() *Metrics {
	return &Metrics{}
}

// RecordEventRouted records an event being routed to a box
func (m *Metrics) RecordEventRouted(box Box) {
	havenEventsRouted.WithLabelValues(box.String()).Inc()
}

// RecordEventRejected records an event being rejected
func (m *Metrics) RecordEventRejected(box Box, reason string) {
	havenEventsRejected.WithLabelValues(box.String(), categorizeReason(reason)).Inc()
}

// RecordFilterRouted records a filter being routed to a box
func (m *Metrics) RecordFilterRouted(box Box) {
	havenFiltersRouted.WithLabelValues(box.String()).Inc()
}

// RecordFilterRejected records a filter being rejected
func (m *Metrics) RecordFilterRejected(box Box, reason string) {
	havenFiltersRejected.WithLabelValues(box.String(), categorizeReason(reason)).Inc()
}

// RecordAccessAttempt records a box access attempt
func (m *Metrics) RecordAccessAttempt(box Box, operation string) {
	havenAccessAttempts.WithLabelValues(box.String(), operation).Inc()
}

// RecordAccessDenied records a denied box access
func (m *Metrics) RecordAccessDenied(box Box, operation string, reason string) {
	havenAccessDenied.WithLabelValues(box.String(), operation, categorizeReason(reason)).Inc()
}

// RecordBlastrBroadcast records a successful broadcast
func (m *Metrics) RecordBlastrBroadcast() {
	havenBlastrEventsBroadcast.Inc()
}

// RecordBlastrFailed records a failed broadcast
func (m *Metrics) RecordBlastrFailed() {
	havenBlastrEventsFailed.Inc()
}

// RecordBlastrQueued records an event being queued
func (m *Metrics) RecordBlastrQueued() {
	havenBlastrEventsQueued.Inc()
}

// RecordBlastrDropped records an event being dropped from queue
func (m *Metrics) RecordBlastrDropped() {
	havenBlastrEventsDropped.Inc()
}

// SetBlastrRelaysConnected sets the number of connected relays
func (m *Metrics) SetBlastrRelaysConnected(count int) {
	havenBlastrRelaysConnected.Set(float64(count))
}

// SetBlastrQueueSize sets the current queue size
func (m *Metrics) SetBlastrQueueSize(size int) {
	havenBlastrQueueSize.Set(float64(size))
}

// RecordBlastrRelayPublish records a publish attempt to a specific relay
func (m *Metrics) RecordBlastrRelayPublish(relay string, success bool) {
	status := "success"
	if !success {
		status = "failed"
	}
	havenBlastrRelayPublishTotal.WithLabelValues(truncateRelay(relay), status).Inc()
}

// RecordBlastrRetryQueued records an event being queued for retry
func (m *Metrics) RecordBlastrRetryQueued() {
	havenBlastrRetryQueued.Inc()
}

// RecordBlastrRetrySuccess records a successful retry
func (m *Metrics) RecordBlastrRetrySuccess() {
	havenBlastrRetrySuccess.Inc()
}

// RecordBlastrRetryExhausted records retries exhausted for an event
func (m *Metrics) RecordBlastrRetryExhausted() {
	havenBlastrRetryExhausted.Inc()
}

// SetBlastrRetryQueueSize sets the retry queue size
func (m *Metrics) SetBlastrRetryQueueSize(size int) {
	havenBlastrRetryQueueSize.Set(float64(size))
}

// RecordImporterImported records an imported event
func (m *Metrics) RecordImporterImported() {
	havenImporterEventsImported.Inc()
}

// RecordImporterSkipped records a skipped event
func (m *Metrics) RecordImporterSkipped() {
	havenImporterEventsSkipped.Inc()
}

// RecordImporterFetchError records a fetch error
func (m *Metrics) RecordImporterFetchError() {
	havenImporterFetchErrors.Inc()
}

// SetImporterRelaysPolled sets the number of relays being polled
func (m *Metrics) SetImporterRelaysPolled(count int) {
	havenImporterRelaysPolled.Set(float64(count))
}

// SetImporterLastPollTimestamp sets the last poll timestamp
func (m *Metrics) SetImporterLastPollTimestamp(ts float64) {
	havenImporterLastPollTimestamp.Set(ts)
}

// RecordImporterRelayFetch records a fetch attempt from a specific relay
func (m *Metrics) RecordImporterRelayFetch(relay string, success bool) {
	status := "success"
	if !success {
		status = "failed"
	}
	havenImporterRelayFetchTotal.WithLabelValues(truncateRelay(relay), status).Inc()
}

// SetBoxEventsStored sets the event count for a box
func (m *Metrics) SetBoxEventsStored(box Box, count int64) {
	havenBoxEventsStored.WithLabelValues(box.String()).Set(float64(count))
}

// SetHavenEnabled sets the HAVEN enabled status
func (m *Metrics) SetHavenEnabled(enabled bool) {
	if enabled {
		havenEnabled.Set(1)
	} else {
		havenEnabled.Set(0)
	}
}

// SetBlastrEnabled sets the Blastr enabled status
func (m *Metrics) SetBlastrEnabled(enabled bool) {
	if enabled {
		havenBlastrEnabled.Set(1)
	} else {
		havenBlastrEnabled.Set(0)
	}
}

// SetImporterEnabled sets the Importer enabled status
func (m *Metrics) SetImporterEnabled(enabled bool) {
	if enabled {
		havenImporterEnabled.Set(1)
	} else {
		havenImporterEnabled.Set(0)
	}
}

// categorizeReason extracts a short category from a rejection reason
// It looks for a colon prefix (e.g., "auth-required: message" -> "auth-required")
// to keep Prometheus label cardinality low
func categorizeReason(reason string) string {
	// First, try to extract prefix before colon (works for any length)
	for i, c := range reason {
		if c == ':' {
			return reason[:i]
		}
	}
	// No colon found, truncate if too long
	if len(reason) > 30 {
		return reason[:30]
	}
	return reason
}

// truncateRelay shortens relay URLs for metric labels
func truncateRelay(url string) string {
	// Remove wss:// or ws:// prefix
	url = strings.TrimPrefix(url, "wss://")
	url = strings.TrimPrefix(url, "ws://")
	// Truncate long URLs
	if len(url) > 40 {
		return url[:40]
	}
	return url
}

// Global metrics instance
var globalMetrics = NewMetrics()

// GetMetrics returns the global metrics instance
func GetMetrics() *Metrics {
	return globalMetrics
}

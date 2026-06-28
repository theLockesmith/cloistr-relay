package haven

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetrics_RecordEventRouted(t *testing.T) {
	m := NewMetrics()

	// Record events routed to different boxes
	m.RecordEventRouted(BoxPrivate)
	m.RecordEventRouted(BoxChat)
	m.RecordEventRouted(BoxInbox)
	m.RecordEventRouted(BoxOutbox)
	m.RecordEventRouted(BoxOutbox)

	// Verify counter values
	if count := testutil.ToFloat64(havenEventsRouted.WithLabelValues("private")); count < 1 {
		t.Errorf("Expected private events routed >= 1, got %f", count)
	}
	if count := testutil.ToFloat64(havenEventsRouted.WithLabelValues("outbox")); count < 2 {
		t.Errorf("Expected outbox events routed >= 2, got %f", count)
	}
}

func TestMetrics_RecordEventRejected(t *testing.T) {
	m := NewMetrics()

	m.RecordEventRejected(BoxPrivate, "auth_required")
	m.RecordEventRejected(BoxOutbox, "not_owner")

	// Verify counter values
	if count := testutil.ToFloat64(havenEventsRejected.WithLabelValues("private", "auth_required")); count < 1 {
		t.Errorf("Expected private auth_required rejections >= 1, got %f", count)
	}
}

func TestMetrics_BlastrMetrics(t *testing.T) {
	m := NewMetrics()

	m.RecordBlastrQueued()
	m.RecordBlastrQueued()
	m.RecordBlastrBroadcast()
	m.RecordBlastrFailed()
	m.RecordBlastrDropped()
	m.SetBlastrRelaysConnected(3)
	m.SetBlastrQueueSize(5)

	// Verify gauges
	if count := testutil.ToFloat64(havenBlastrRelaysConnected); count != 3 {
		t.Errorf("Expected relays connected = 3, got %f", count)
	}
	if count := testutil.ToFloat64(havenBlastrQueueSize); count != 5 {
		t.Errorf("Expected queue size = 5, got %f", count)
	}
}

func TestMetrics_ImporterMetrics(t *testing.T) {
	m := NewMetrics()

	m.RecordImporterImported()
	m.RecordImporterImported()
	m.RecordImporterSkipped()
	m.RecordImporterFetchError()
	m.SetImporterRelaysPolled(5)
	m.SetImporterLastPollTimestamp(1234567890.0)

	// Verify gauges
	if count := testutil.ToFloat64(havenImporterRelaysPolled); count != 5 {
		t.Errorf("Expected relays polled = 5, got %f", count)
	}
	if ts := testutil.ToFloat64(havenImporterLastPollTimestamp); ts != 1234567890.0 {
		t.Errorf("Expected last poll timestamp = 1234567890, got %f", ts)
	}
}

func TestMetrics_HavenEnabled(t *testing.T) {
	m := NewMetrics()

	m.SetHavenEnabled(true)
	if v := testutil.ToFloat64(havenEnabled); v != 1 {
		t.Errorf("Expected haven enabled = 1, got %f", v)
	}

	m.SetHavenEnabled(false)
	if v := testutil.ToFloat64(havenEnabled); v != 0 {
		t.Errorf("Expected haven enabled = 0, got %f", v)
	}
}

func TestMetrics_BlastrEnabled(t *testing.T) {
	m := NewMetrics()

	m.SetBlastrEnabled(true)
	if v := testutil.ToFloat64(havenBlastrEnabled); v != 1 {
		t.Errorf("Expected blastr enabled = 1, got %f", v)
	}

	m.SetBlastrEnabled(false)
	if v := testutil.ToFloat64(havenBlastrEnabled); v != 0 {
		t.Errorf("Expected blastr enabled = 0, got %f", v)
	}
}

func TestMetrics_ImporterEnabled(t *testing.T) {
	m := NewMetrics()

	m.SetImporterEnabled(true)
	if v := testutil.ToFloat64(havenImporterEnabled); v != 1 {
		t.Errorf("Expected importer enabled = 1, got %f", v)
	}

	m.SetImporterEnabled(false)
	if v := testutil.ToFloat64(havenImporterEnabled); v != 0 {
		t.Errorf("Expected importer enabled = 0, got %f", v)
	}
}

func TestMetrics_CategorizeReason(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"auth_required", "auth_required"},
		{"not_owner", "not_owner"},
		{"restricted: only owner can write to private box", "restricted"},
		{"auth: short", "auth"},                                             // Short string with colon
		{"this is a very long reason that should be truncated", "this is a very long reason tha"},
	}

	for _, tc := range tests {
		got := categorizeReason(tc.input)
		if got != tc.expected {
			t.Errorf("categorizeReason(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestMetrics_TruncateRelay(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"wss://relay.example.com", "relay.example.com"},
		{"ws://relay.example.com", "relay.example.com"},
		{"relay.example.com", "relay.example.com"},
		{"wss://this-is-a-very-long-relay-url-that-should-be-truncated.example.com/path", "this-is-a-very-long-relay-url-that-shoul"},
	}

	for _, tc := range tests {
		got := truncateRelay(tc.input)
		if got != tc.expected {
			t.Errorf("truncateRelay(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestMetrics_GetMetrics(t *testing.T) {
	m := GetMetrics()
	if m == nil {
		t.Error("GetMetrics() returned nil")
	}

	// Should return the same instance
	m2 := GetMetrics()
	if m != m2 {
		t.Error("GetMetrics() should return the same instance")
	}
}

func TestMetrics_AccessMetrics(t *testing.T) {
	m := NewMetrics()

	m.RecordAccessAttempt(BoxPrivate, "read")
	m.RecordAccessAttempt(BoxPrivate, "write")
	m.RecordAccessDenied(BoxPrivate, "read", "auth_required")

	// Verify counters are recording
	if count := testutil.ToFloat64(havenAccessAttempts.WithLabelValues("private", "read")); count < 1 {
		t.Errorf("Expected access attempts >= 1, got %f", count)
	}
	if count := testutil.ToFloat64(havenAccessDenied.WithLabelValues("private", "read", "auth_required")); count < 1 {
		t.Errorf("Expected access denied >= 1, got %f", count)
	}
}

func TestMetrics_FilterMetrics(t *testing.T) {
	m := NewMetrics()

	m.RecordFilterRouted(BoxOutbox)
	m.RecordFilterRouted(BoxInbox)
	m.RecordFilterRejected(BoxPrivate, "auth_required")

	// Verify counters are recording
	if count := testutil.ToFloat64(havenFiltersRouted.WithLabelValues("outbox")); count < 1 {
		t.Errorf("Expected filters routed to outbox >= 1, got %f", count)
	}
	if count := testutil.ToFloat64(havenFiltersRejected.WithLabelValues("private", "auth_required")); count < 1 {
		t.Errorf("Expected filter rejections >= 1, got %f", count)
	}
}

func TestMetrics_RelayPublishMetrics(t *testing.T) {
	m := NewMetrics()

	m.RecordBlastrRelayPublish("wss://relay1.example.com", true)
	m.RecordBlastrRelayPublish("wss://relay1.example.com", false)
	m.RecordBlastrRelayPublish("wss://relay2.example.com", true)

	// Verify counters
	if count := testutil.ToFloat64(havenBlastrRelayPublishTotal.WithLabelValues("relay1.example.com", "success")); count < 1 {
		t.Errorf("Expected relay1 success >= 1, got %f", count)
	}
	if count := testutil.ToFloat64(havenBlastrRelayPublishTotal.WithLabelValues("relay1.example.com", "failed")); count < 1 {
		t.Errorf("Expected relay1 failed >= 1, got %f", count)
	}
}

func TestMetrics_RelayFetchMetrics(t *testing.T) {
	m := NewMetrics()

	m.RecordImporterRelayFetch("wss://relay1.example.com", true)
	m.RecordImporterRelayFetch("wss://relay1.example.com", false)

	// Verify counters
	if count := testutil.ToFloat64(havenImporterRelayFetchTotal.WithLabelValues("relay1.example.com", "success")); count < 1 {
		t.Errorf("Expected relay1 fetch success >= 1, got %f", count)
	}
	if count := testutil.ToFloat64(havenImporterRelayFetchTotal.WithLabelValues("relay1.example.com", "failed")); count < 1 {
		t.Errorf("Expected relay1 fetch failed >= 1, got %f", count)
	}
}

func TestMetrics_BoxEventsStored(t *testing.T) {
	m := NewMetrics()

	m.SetBoxEventsStored(BoxPrivate, 100)
	m.SetBoxEventsStored(BoxChat, 50)
	m.SetBoxEventsStored(BoxInbox, 200)
	m.SetBoxEventsStored(BoxOutbox, 1000)

	if count := testutil.ToFloat64(havenBoxEventsStored.WithLabelValues("private")); count != 100 {
		t.Errorf("Expected private events = 100, got %f", count)
	}
	if count := testutil.ToFloat64(havenBoxEventsStored.WithLabelValues("outbox")); count != 1000 {
		t.Errorf("Expected outbox events = 1000, got %f", count)
	}
}

func TestMetrics_TierDistribution(t *testing.T) {
	m := NewMetrics()

	m.SetMembersByTier("free", 100)
	m.SetMembersByTier("hybrid", 50)
	m.SetMembersByTier("premium", 25)
	m.SetMembersByTier("enterprise", 5)

	if count := testutil.ToFloat64(havenMembersByTier.WithLabelValues("free")); count != 100 {
		t.Errorf("Expected free tier = 100, got %f", count)
	}
	if count := testutil.ToFloat64(havenMembersByTier.WithLabelValues("premium")); count != 25 {
		t.Errorf("Expected premium tier = 25, got %f", count)
	}
}

func TestMetrics_WoTFiltering(t *testing.T) {
	m := NewMetrics()

	m.RecordWoTBlock("user_block")
	m.RecordWoTBlock("user_block")
	m.RecordWoTBlock("relay_block")
	m.RecordWoTAllow("user_trust")
	m.RecordWoTAllow("default")
	m.RecordWoTAllow("default")
	m.RecordWoTAllow("default")

	if count := testutil.ToFloat64(havenWoTBlocksTotal.WithLabelValues("user_block")); count < 2 {
		t.Errorf("Expected user_block >= 2, got %f", count)
	}
	if count := testutil.ToFloat64(havenWoTBlocksTotal.WithLabelValues("relay_block")); count < 1 {
		t.Errorf("Expected relay_block >= 1, got %f", count)
	}
	if count := testutil.ToFloat64(havenWoTAllowsTotal.WithLabelValues("user_trust")); count < 1 {
		t.Errorf("Expected user_trust allows >= 1, got %f", count)
	}
	if count := testutil.ToFloat64(havenWoTAllowsTotal.WithLabelValues("default")); count < 3 {
		t.Errorf("Expected default allows >= 3, got %f", count)
	}
}

func TestMetrics_WorkerProcessing(t *testing.T) {
	m := NewMetrics()

	// Record some processing times
	m.ObserveWorkerProcessing("blastr", 0.1)
	m.ObserveWorkerProcessing("blastr", 0.2)
	m.ObserveWorkerProcessing("importer", 1.5)
	m.ObserveWorkerProcessing("importer", 2.0)

	// Histograms are harder to test directly, but we can at least verify no panics
	// and that the histogram has observations
	if count := testutil.CollectAndCount(havenWorkerProcessingSeconds); count == 0 {
		t.Error("Expected worker processing histogram to have observations")
	}
}

// mockTierCounter implements TierCounter for testing
type mockTierCounter struct {
	counts map[string]int
	err    error
}

func (m *mockTierCounter) CountMembersByTier() (map[string]int, error) {
	return m.counts, m.err
}

func TestMetricsCollector(t *testing.T) {
	// Create a mock tier counter
	mock := &mockTierCounter{
		counts: map[string]int{
			"free":       100,
			"hybrid":     20,
			"premium":    10,
			"enterprise": 5,
		},
	}

	// Create collector with short interval for testing
	collector := NewMetricsCollector(mock, 100*time.Millisecond)
	collector.Start()

	// Wait for at least one collection cycle
	time.Sleep(150 * time.Millisecond)

	// Check that metrics were collected
	if count := testutil.ToFloat64(havenMembersByTier.WithLabelValues("free")); count != 100 {
		t.Errorf("Expected free tier = 100, got %f", count)
	}
	if count := testutil.ToFloat64(havenMembersByTier.WithLabelValues("enterprise")); count != 5 {
		t.Errorf("Expected enterprise tier = 5, got %f", count)
	}

	collector.Stop()
}

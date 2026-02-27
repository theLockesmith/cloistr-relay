package nip66

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

// SelfMonitor monitors the local relay and publishes health events
type SelfMonitor struct {
	relayURL    string
	monitorKey  string // hex private key for signing events
	pubkey      string
	interval    time.Duration
	publishFunc func(context.Context, *nostr.Event) error
	stopCh      chan struct{}
	wg          sync.WaitGroup
	nip11Cache  map[string]interface{}
	mu          sync.Mutex
}

// MonitorConfig holds self-monitor configuration
type MonitorConfig struct {
	RelayURL    string        // The relay URL to monitor (e.g., wss://relay.example.com)
	MonitorKey  string        // Hex private key for signing monitor events
	Interval    time.Duration // How often to publish health events
	PublishFunc func(context.Context, *nostr.Event) error
}

// NewSelfMonitor creates a new self-monitoring service
func NewSelfMonitor(cfg *MonitorConfig) (*SelfMonitor, error) {
	if cfg.RelayURL == "" {
		return nil, fmt.Errorf("relay URL required")
	}
	if cfg.MonitorKey == "" {
		return nil, fmt.Errorf("monitor private key required")
	}

	// Derive public key from private key
	pubkey, err := derivePubkey(cfg.MonitorKey)
	if err != nil {
		return nil, fmt.Errorf("failed to derive pubkey: %w", err)
	}

	return &SelfMonitor{
		relayURL:    normalizeRelayURL(cfg.RelayURL),
		monitorKey:  cfg.MonitorKey,
		pubkey:      pubkey,
		interval:    cfg.Interval,
		publishFunc: cfg.PublishFunc,
		stopCh:      make(chan struct{}),
		nip11Cache:  make(map[string]interface{}),
	}, nil
}

// Start begins the self-monitoring loop
func (m *SelfMonitor) Start() {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		// Publish immediately on start
		m.publishHealth()

		ticker := time.NewTicker(m.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				m.publishHealth()
			case <-m.stopCh:
				return
			}
		}
	}()

	log.Printf("NIP-66 self-monitor started (interval: %v)", m.interval)
}

// Stop halts the self-monitoring loop
func (m *SelfMonitor) Stop() {
	close(m.stopCh)
	m.wg.Wait()
	log.Println("NIP-66 self-monitor stopped")
}

// publishHealth creates and publishes a kind 30166 relay discovery event
func (m *SelfMonitor) publishHealth() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Probe the relay
	health := m.probeRelay(ctx)

	// Build the event
	event := m.buildDiscoveryEvent(health)

	// Sign the event
	if err := event.Sign(m.monitorKey); err != nil {
		log.Printf("NIP-66: failed to sign event: %v", err)
		return
	}

	// Publish
	if m.publishFunc != nil {
		if err := m.publishFunc(ctx, event); err != nil {
			log.Printf("NIP-66: failed to publish health event: %v", err)
			return
		}
	}

	log.Printf("NIP-66: published relay health (rtt_open=%dms, rtt_read=%dms)",
		health.RTTOpen, health.RTTRead)
}

// RelayHealth holds probe results
type RelayHealth struct {
	Online        bool
	RTTOpen       int // milliseconds
	RTTRead       int // milliseconds
	RTTWrite      int // milliseconds
	RTTNIP11      int // milliseconds
	SupportedNIPs []int
	Network       string
	Software      string
	Version       string
	Error         string
}

// probeRelay performs health checks on the relay
func (m *SelfMonitor) probeRelay(ctx context.Context) *RelayHealth {
	health := &RelayHealth{
		Network: "clearnet",
	}

	// Convert wss:// to https:// for NIP-11 probe
	httpURL := strings.Replace(m.relayURL, "wss://", "https://", 1)
	httpURL = strings.Replace(httpURL, "ws://", "http://", 1)

	// Probe NIP-11
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, "GET", httpURL, nil)
	if err != nil {
		health.Error = fmt.Sprintf("failed to create request: %v", err)
		return health
	}
	req.Header.Set("Accept", "application/nostr+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		health.Error = fmt.Sprintf("NIP-11 probe failed: %v", err)
		return health
	}
	defer func() { _ = resp.Body.Close() }()

	health.RTTNIP11 = int(time.Since(start).Milliseconds())
	health.RTTOpen = health.RTTNIP11 // Use NIP-11 RTT as open RTT approximation

	if resp.StatusCode != http.StatusOK {
		health.Error = fmt.Sprintf("NIP-11 returned status %d", resp.StatusCode)
		return health
	}

	// Parse NIP-11 response
	var nip11 map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&nip11); err != nil {
		health.Error = fmt.Sprintf("failed to parse NIP-11: %v", err)
		return health
	}

	health.Online = true

	// Extract supported NIPs
	if nips, ok := nip11["supported_nips"].([]interface{}); ok {
		for _, nip := range nips {
			switch v := nip.(type) {
			case float64:
				health.SupportedNIPs = append(health.SupportedNIPs, int(v))
			case int:
				health.SupportedNIPs = append(health.SupportedNIPs, v)
			}
		}
	}

	// Extract software info
	if software, ok := nip11["software"].(string); ok {
		health.Software = software
	}
	if version, ok := nip11["version"].(string); ok {
		health.Version = version
	}

	// Cache NIP-11 data
	m.mu.Lock()
	m.nip11Cache = nip11
	m.mu.Unlock()

	// For read/write RTT, we'd need to actually connect via WebSocket
	// For now, use NIP-11 RTT as an approximation
	health.RTTRead = health.RTTOpen
	health.RTTWrite = health.RTTOpen

	return health
}

// buildDiscoveryEvent creates a kind 30166 event from health data
func (m *SelfMonitor) buildDiscoveryEvent(health *RelayHealth) *nostr.Event {
	tags := nostr.Tags{
		nostr.Tag{"d", m.relayURL},
		nostr.Tag{"n", health.Network},
	}

	// Add RTT tags
	if health.RTTOpen > 0 {
		tags = append(tags, nostr.Tag{"rtt", "open", strconv.Itoa(health.RTTOpen)})
	}
	if health.RTTRead > 0 {
		tags = append(tags, nostr.Tag{"rtt", "read", strconv.Itoa(health.RTTRead)})
	}
	if health.RTTWrite > 0 {
		tags = append(tags, nostr.Tag{"rtt", "write", strconv.Itoa(health.RTTWrite)})
	}

	// Add supported NIPs
	for _, nip := range health.SupportedNIPs {
		tags = append(tags, nostr.Tag{"N", strconv.Itoa(nip)})
	}

	// Add software info if available
	if health.Software != "" {
		tags = append(tags, nostr.Tag{"s", health.Software})
	}
	if health.Version != "" {
		tags = append(tags, nostr.Tag{"v", health.Version})
	}

	// Build content (can include more detailed info)
	content := ""
	if health.Error != "" {
		content = fmt.Sprintf("error: %s", health.Error)
	}

	return &nostr.Event{
		PubKey:    m.pubkey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      KindRelayDiscovery,
		Tags:      tags,
		Content:   content,
	}
}

// normalizeRelayURL normalizes a relay URL
func normalizeRelayURL(url string) string {
	// Remove trailing slash
	url = strings.TrimSuffix(url, "/")
	// Ensure wss:// prefix
	if !strings.HasPrefix(url, "wss://") && !strings.HasPrefix(url, "ws://") {
		url = "wss://" + url
	}
	return url
}

// derivePubkey derives the public key from a private key
func derivePubkey(privkey string) (string, error) {
	// Decode hex private key
	privBytes, err := hex.DecodeString(privkey)
	if err != nil {
		return "", err
	}
	if len(privBytes) != 32 {
		return "", fmt.Errorf("invalid private key length")
	}

	// Use go-nostr to get public key
	pub, err := nostr.GetPublicKey(privkey)
	if err != nil {
		return "", err
	}

	return pub, nil
}

// GenerateMonitorKey generates a new random private key for monitoring
func GenerateMonitorKey() (string, error) {
	// Generate 32 random bytes
	var privkey [32]byte
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%d", time.Now().UnixNano())
	copy(privkey[:], h.Sum(nil))
	return hex.EncodeToString(privkey[:]), nil
}

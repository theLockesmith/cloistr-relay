// Package nip66 implements NIP-66 Relay Discovery and Monitoring
// https://github.com/nostr-protocol/nips/blob/master/66.md
package nip66

import (
	"context"
	"strings"

	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"
)

const (
	// KindRelayDiscovery is the event kind for relay discovery events (30166)
	KindRelayDiscovery = 30166

	// KindMonitorAnnouncement is the event kind for monitor announcements (10166)
	KindMonitorAnnouncement = 10166
)

// Config holds NIP-66 configuration
type Config struct {
	Enabled bool
}

// Handler manages NIP-66 relay discovery events
type Handler struct {
	config *Config
}

// NewHandler creates a new NIP-66 handler
func NewHandler(cfg *Config) *Handler {
	return &Handler{config: cfg}
}

// RegisterHandlers registers NIP-66 handlers with the relay
func RegisterHandlers(relay *khatru.Relay, cfg *Config) {
	if cfg == nil || !cfg.Enabled {
		return
	}

	h := NewHandler(cfg)

	// Validate relay discovery events (kind 30166)
	relay.RejectEvent = append(relay.RejectEvent, h.validateRelayDiscovery())
}

// validateRelayDiscovery validates kind 30166 relay discovery events
func (h *Handler) validateRelayDiscovery() func(context.Context, *nostr.Event) (bool, string) {
	return func(ctx context.Context, event *nostr.Event) (bool, string) {
		if event.Kind != KindRelayDiscovery {
			return false, ""
		}

		// Kind 30166 must have a 'd' tag with the relay URL
		dTag := getTagValue(event.Tags, "d")
		if dTag == "" {
			return true, "invalid: kind 30166 requires 'd' tag with relay URL"
		}

		// The 'd' tag should be a valid relay URL (wss:// or ws://) or hex pubkey
		if !isValidRelayURL(dTag) && !isValidHexPubkey(dTag) {
			return true, "invalid: 'd' tag must be relay URL or hex pubkey"
		}

		return false, ""
	}
}

// getTagValue extracts the first value for a given tag name
func getTagValue(tags nostr.Tags, name string) string {
	for _, tag := range tags {
		if len(tag) >= 2 && tag[0] == name {
			return tag[1]
		}
	}
	return ""
}

// isValidRelayURL checks if the string is a valid relay WebSocket URL
func isValidRelayURL(s string) bool {
	return strings.HasPrefix(s, "wss://") || strings.HasPrefix(s, "ws://")
}

// isValidHexPubkey checks if the string is a valid 64-character hex pubkey
func isValidHexPubkey(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		isDigit := c >= '0' && c <= '9'
		isLowerHex := c >= 'a' && c <= 'f'
		isUpperHex := c >= 'A' && c <= 'F'
		if !isDigit && !isLowerHex && !isUpperHex {
			return false
		}
	}
	return true
}

// RelayDiscoveryEvent represents a parsed kind 30166 event
type RelayDiscoveryEvent struct {
	RelayURL       string            // From 'd' tag
	Network        string            // clearnet, tor, i2p, etc.
	SupportedNIPs  []int             // From 'N' tags
	RelayType      string            // From 'T' tag
	RTTOpen        int               // Round-trip time to open connection (ms)
	RTTRead        int               // Round-trip time for read (ms)
	RTTWrite       int               // Round-trip time for write (ms)
	GeoHash        string            // Geographic location
	Requirements   map[string]string // auth, payment, etc.
	RawEvent       *nostr.Event
}

// ParseRelayDiscovery parses a kind 30166 event into structured data
func ParseRelayDiscovery(event *nostr.Event) (*RelayDiscoveryEvent, error) {
	if event.Kind != KindRelayDiscovery {
		return nil, nil
	}

	rd := &RelayDiscoveryEvent{
		RelayURL:     getTagValue(event.Tags, "d"),
		Network:      getTagValue(event.Tags, "n"),
		RelayType:    getTagValue(event.Tags, "T"),
		GeoHash:      getTagValue(event.Tags, "g"),
		Requirements: make(map[string]string),
		RawEvent:     event,
	}

	// Parse supported NIPs from 'N' tags
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "N" {
			// Parse NIP number
			var nip int
			for _, c := range tag[1] {
				if c >= '0' && c <= '9' {
					nip = nip*10 + int(c-'0')
				}
			}
			if nip > 0 {
				rd.SupportedNIPs = append(rd.SupportedNIPs, nip)
			}
		}
	}

	// Parse RTT values from 'rtt' tags
	for _, tag := range event.Tags {
		if len(tag) >= 3 && tag[0] == "rtt" {
			rttType := tag[1]
			var rttValue int
			for _, c := range tag[2] {
				if c >= '0' && c <= '9' {
					rttValue = rttValue*10 + int(c-'0')
				}
			}
			switch rttType {
			case "open":
				rd.RTTOpen = rttValue
			case "read":
				rd.RTTRead = rttValue
			case "write":
				rd.RTTWrite = rttValue
			}
		}
	}

	// Parse requirements
	for _, tag := range event.Tags {
		if len(tag) >= 2 {
			switch tag[0] {
			case "R": // Requirements tag
				if len(tag) >= 2 {
					rd.Requirements[tag[1]] = ""
					if len(tag) >= 3 {
						rd.Requirements[tag[1]] = tag[2]
					}
				}
			}
		}
	}

	return rd, nil
}

// MonitorAnnouncement represents a parsed kind 10166 event
type MonitorAnnouncement struct {
	Frequency  int      // Monitoring frequency in seconds
	Timeout    int      // Timeout for checks in ms
	CheckTypes []string // Types of checks performed (open, read, write, auth, nip11, dns, geo)
	GeoHash    string   // Monitor's geographic location
	RawEvent   *nostr.Event
}

// ParseMonitorAnnouncement parses a kind 10166 event into structured data
func ParseMonitorAnnouncement(event *nostr.Event) (*MonitorAnnouncement, error) {
	if event.Kind != KindMonitorAnnouncement {
		return nil, nil
	}

	ma := &MonitorAnnouncement{
		RawEvent: event,
	}

	// Parse frequency
	freqStr := getTagValue(event.Tags, "frequency")
	for _, c := range freqStr {
		if c >= '0' && c <= '9' {
			ma.Frequency = ma.Frequency*10 + int(c-'0')
		}
	}

	// Parse timeout
	timeoutStr := getTagValue(event.Tags, "timeout")
	for _, c := range timeoutStr {
		if c >= '0' && c <= '9' {
			ma.Timeout = ma.Timeout*10 + int(c-'0')
		}
	}

	// Parse geohash
	ma.GeoHash = getTagValue(event.Tags, "g")

	// Parse check types from 'c' tags
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "c" {
			ma.CheckTypes = append(ma.CheckTypes, tag[1])
		}
	}

	return ma, nil
}

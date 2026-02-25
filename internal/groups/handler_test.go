package groups

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

func TestGetGroupIDFromEvent(t *testing.T) {
	tests := []struct {
		name     string
		event    *nostr.Event
		expected string
	}{
		{
			name: "event with h tag",
			event: &nostr.Event{
				Tags: nostr.Tags{
					{"h", "group123"},
				},
			},
			expected: "group123",
		},
		{
			name: "event with multiple tags",
			event: &nostr.Event{
				Tags: nostr.Tags{
					{"p", "pubkey123"},
					{"h", "group456"},
					{"e", "event789"},
				},
			},
			expected: "group456",
		},
		{
			name: "event without h tag",
			event: &nostr.Event{
				Tags: nostr.Tags{
					{"p", "pubkey123"},
					{"e", "event789"},
				},
			},
			expected: "",
		},
		{
			name: "event with empty tags",
			event: &nostr.Event{
				Tags: nostr.Tags{},
			},
			expected: "",
		},
		{
			name: "event with h tag but no value",
			event: &nostr.Event{
				Tags: nostr.Tags{
					{"h"},
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getGroupIDFromEvent(tt.event)
			if result != tt.expected {
				t.Errorf("getGroupIDFromEvent() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetGroupIDFromFilter(t *testing.T) {
	tests := []struct {
		name     string
		filter   nostr.Filter
		expected string
	}{
		{
			name: "filter with h tag",
			filter: nostr.Filter{
				Tags: map[string][]string{
					"h": {"group123"},
				},
			},
			expected: "group123",
		},
		{
			name: "filter with multiple h values",
			filter: nostr.Filter{
				Tags: map[string][]string{
					"h": {"group123", "group456"},
				},
			},
			expected: "group123", // Returns first value
		},
		{
			name: "filter without h tag",
			filter: nostr.Filter{
				Tags: map[string][]string{
					"p": {"pubkey123"},
				},
			},
			expected: "",
		},
		{
			name: "filter with empty tags",
			filter: nostr.Filter{
				Tags: map[string][]string{},
			},
			expected: "",
		},
		{
			name: "filter with empty h values",
			filter: nostr.Filter{
				Tags: map[string][]string{
					"h": {},
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getGroupIDFromFilter(tt.filter)
			if result != tt.expected {
				t.Errorf("getGroupIDFromFilter() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestTruncateID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1234567890abcdef", "12345678"},
		{"12345678", "12345678"},
		{"1234567", "1234567"},
		{"123", "123"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := truncateID(tt.input)
			if result != tt.expected {
				t.Errorf("truncateID(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		slice    []string
		val      string
		expected bool
	}{
		{[]string{"a", "b", "c"}, "b", true},
		{[]string{"a", "b", "c"}, "d", false},
		{[]string{}, "a", false},
		{[]string{"A", "B", "C"}, "a", true}, // Case insensitive
		{[]string{"abc", "def"}, "ABC", true}, // Case insensitive
	}

	for _, tt := range tests {
		t.Run(tt.val, func(t *testing.T) {
			result := contains(tt.slice, tt.val)
			if result != tt.expected {
				t.Errorf("contains(%v, %q) = %v, want %v", tt.slice, tt.val, result, tt.expected)
			}
		})
	}
}

func TestNewHandler(t *testing.T) {
	cfg := &Config{
		Enabled:        true,
		AdminPubkeys:   []string{"admin123"},
		DefaultPrivacy: PrivacyRestricted,
	}

	// Can't test fully without a store, but can verify creation doesn't panic
	handler := NewHandler(nil, cfg)
	if handler == nil {
		t.Fatal("NewHandler returned nil")
	}

	if handler.config != cfg {
		t.Error("Handler config not set correctly")
	}
}

func TestEventKindConstants(t *testing.T) {
	// Verify event kinds match NIP-29 spec
	tests := []struct {
		name     string
		kind     int
		expected int
	}{
		{"KindJoinRequest", KindJoinRequest, 9021},
		{"KindLeaveRequest", KindLeaveRequest, 9022},
		{"KindAddUser", KindAddUser, 9000},
		{"KindRemoveUser", KindRemoveUser, 9001},
		{"KindEditMetadata", KindEditMetadata, 9002},
		{"KindDeleteEvent", KindDeleteEvent, 9005},
		{"KindCreateGroup", KindCreateGroup, 9007},
		{"KindDeleteGroup", KindDeleteGroup, 9008},
		{"KindCreateInvite", KindCreateInvite, 9009},
		{"KindGroupMetadata", KindGroupMetadata, 39000},
		{"KindGroupAdmins", KindGroupAdmins, 39001},
		{"KindGroupMembers", KindGroupMembers, 39002},
		{"KindGroupRoles", KindGroupRoles, 39003},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.kind != tt.expected {
				t.Errorf("%s = %d, want %d", tt.name, tt.kind, tt.expected)
			}
		})
	}
}

// Test extracting data from moderation event tags
func TestExtractTagValues(t *testing.T) {
	// Test extracting pubkey from p tag
	event := &nostr.Event{
		Tags: nostr.Tags{
			{"h", "group123"},
			{"p", "targetpubkey"},
			{"role", "moderator"},
		},
	}

	var pubkey, role string
	for _, tag := range event.Tags {
		if len(tag) >= 2 {
			switch tag[0] {
			case "p":
				pubkey = tag[1]
			case "role":
				role = tag[1]
			}
		}
	}

	if pubkey != "targetpubkey" {
		t.Errorf("expected pubkey 'targetpubkey', got %q", pubkey)
	}
	if role != "moderator" {
		t.Errorf("expected role 'moderator', got %q", role)
	}
}

// Test extracting name from create group event
func TestExtractGroupName(t *testing.T) {
	event := &nostr.Event{
		Kind: KindCreateGroup,
		Tags: nostr.Tags{
			{"h", "newgroup123"},
			{"name", "Test Group"},
			{"privacy", "private"},
		},
	}

	name := "Unnamed Group"
	var privacy Privacy
	for _, tag := range event.Tags {
		if len(tag) >= 2 {
			switch tag[0] {
			case "name":
				name = tag[1]
			case "privacy":
				privacy = Privacy(tag[1])
			}
		}
	}

	if name != "Test Group" {
		t.Errorf("expected name 'Test Group', got %q", name)
	}
	if privacy != PrivacyPrivate {
		t.Errorf("expected privacy 'private', got %q", privacy)
	}
}

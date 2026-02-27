package haven

import (
	"context"
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

// Test constants
const (
	ownerPubkey   = "owner123456789abcdef0123456789abcdef0123456789abcdef0123456789abc"
	alicePubkey   = "alice123456789abcdef0123456789abcdef0123456789abcdef0123456789abc"
	bobPubkey     = "bob123456789abcdef0123456789abcdef0123456789abcdef0123456789abcde"
	charliePubkey = "charlie123456789abcdef0123456789abcdef0123456789abcdef01234567890"
)

// TestRouteEvent_PrivateKinds tests routing of private kinds to private box
func TestRouteEvent_PrivateKinds(t *testing.T) {
	tests := []struct {
		name        string
		kind        int
		author      string
		expectedBox Box
	}{
		{
			name:        "draft long-form from owner goes to private",
			kind:        30024,
			author:      ownerPubkey,
			expectedBox: BoxPrivate,
		},
		{
			name:        "draft generic from owner goes to private",
			kind:        31234,
			author:      ownerPubkey,
			expectedBox: BoxPrivate,
		},
		{
			name:        "ecash wallet from owner goes to private",
			kind:        7375,
			author:      ownerPubkey,
			expectedBox: BoxPrivate,
		},
		{
			name:        "ecash history from owner goes to private",
			kind:        7376,
			author:      ownerPubkey,
			expectedBox: BoxPrivate,
		},
		{
			name:        "application data from owner goes to private",
			kind:        30078,
			author:      ownerPubkey,
			expectedBox: BoxPrivate,
		},
		{
			name:        "bookmark list from owner goes to private",
			kind:        10003,
			author:      ownerPubkey,
			expectedBox: BoxPrivate,
		},
		{
			name:        "bookmark sets from owner goes to private",
			kind:        30003,
			author:      ownerPubkey,
			expectedBox: BoxPrivate,
		},
		{
			name:        "private kind from non-owner returns unknown",
			kind:        30024,
			author:      alicePubkey,
			expectedBox: BoxUnknown,
		},
		{
			name:        "ecash from non-owner returns unknown",
			kind:        7375,
			author:      bobPubkey,
			expectedBox: BoxUnknown,
		},
	}

	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	router := NewRouter(cfg)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &nostr.Event{
				Kind:   tt.kind,
				PubKey: tt.author,
				Tags:   nostr.Tags{},
			}

			box := router.RouteEvent(event)
			if box != tt.expectedBox {
				t.Errorf("RouteEvent() = %v, want %v", box, tt.expectedBox)
			}
		})
	}
}

// TestRouteEvent_ChatKinds tests routing of chat kinds to chat box
func TestRouteEvent_ChatKinds(t *testing.T) {
	tests := []struct {
		name   string
		kind   int
		author string
	}{
		{
			name:   "encrypted DM (kind 4) goes to chat",
			kind:   4,
			author: alicePubkey,
		},
		{
			name:   "seal (kind 13) goes to chat",
			kind:   13,
			author: bobPubkey,
		},
		{
			name:   "gift wrap (kind 1059) goes to chat",
			kind:   1059,
			author: charliePubkey,
		},
		{
			name:   "gift wrap alt (kind 1060) goes to chat",
			kind:   1060,
			author: alicePubkey,
		},
		{
			name:   "chat kind from owner goes to chat",
			kind:   4,
			author: ownerPubkey,
		},
	}

	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	router := NewRouter(cfg)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &nostr.Event{
				Kind:   tt.kind,
				PubKey: tt.author,
				Tags:   nostr.Tags{},
			}

			box := router.RouteEvent(event)
			if box != BoxChat {
				t.Errorf("RouteEvent() = %v, want %v", box, BoxChat)
			}
		})
	}
}

// TestRouteEvent_OutboxKinds tests routing of owner's events to outbox
func TestRouteEvent_OutboxKinds(t *testing.T) {
	tests := []struct {
		name string
		kind int
	}{
		{"metadata", 0},
		{"text note", 1},
		{"contact list", 3},
		{"repost", 6},
		{"reaction", 7},
		{"relay list", 10002},
		{"long-form", 30023},
		{"custom kind", 12345},
	}

	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	router := NewRouter(cfg)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &nostr.Event{
				Kind:   tt.kind,
				PubKey: ownerPubkey,
				Tags:   nostr.Tags{},
			}

			box := router.RouteEvent(event)
			if box != BoxOutbox {
				t.Errorf("RouteEvent() = %v, want %v for owner's event", box, BoxOutbox)
			}
		})
	}
}

// TestRouteEvent_InboxKinds tests routing of events addressed to owner to inbox
func TestRouteEvent_InboxKinds(t *testing.T) {
	tests := []struct {
		name string
		kind int
	}{
		{"text note with p-tag", 1},
		{"repost with p-tag", 6},
		{"reaction with p-tag", 7},
		{"zap receipt with p-tag", 9735},
		{"comment with p-tag", 1111},
		{"long-form with p-tag", 30023},
	}

	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	router := NewRouter(cfg)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &nostr.Event{
				Kind:   tt.kind,
				PubKey: alicePubkey,
				Tags: nostr.Tags{
					{"p", ownerPubkey},
				},
			}

			box := router.RouteEvent(event)
			if box != BoxInbox {
				t.Errorf("RouteEvent() = %v, want %v for event addressed to owner", box, BoxInbox)
			}
		})
	}
}

// TestRouteEvent_InboxMultiplePTags tests that events with owner in p-tags go to inbox
func TestRouteEvent_InboxMultiplePTags(t *testing.T) {
	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	router := NewRouter(cfg)

	// Event with multiple p-tags, owner is one of them
	event := &nostr.Event{
		Kind:   1,
		PubKey: alicePubkey,
		Tags: nostr.Tags{
			{"p", bobPubkey},
			{"p", ownerPubkey},
			{"p", charliePubkey},
		},
	}

	box := router.RouteEvent(event)
	if box != BoxInbox {
		t.Errorf("RouteEvent() = %v, want %v for event with owner in p-tags", box, BoxInbox)
	}
}

// TestRouteEvent_Unknown tests events that don't belong to any box
func TestRouteEvent_Unknown(t *testing.T) {
	tests := []struct {
		name   string
		event  *nostr.Event
	}{
		{
			name: "event from non-owner without p-tag",
			event: &nostr.Event{
				Kind:   1,
				PubKey: alicePubkey,
				Tags:   nostr.Tags{},
			},
		},
		{
			name: "event with p-tag to different user",
			event: &nostr.Event{
				Kind:   1,
				PubKey: alicePubkey,
				Tags: nostr.Tags{
					{"p", bobPubkey},
				},
			},
		},
		{
			name: "event with only e-tags",
			event: &nostr.Event{
				Kind:   1,
				PubKey: alicePubkey,
				Tags: nostr.Tags{
					{"e", "someeventid123"},
				},
			},
		},
	}

	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	router := NewRouter(cfg)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			box := router.RouteEvent(tt.event)
			if box != BoxUnknown {
				t.Errorf("RouteEvent() = %v, want %v for unroutable event", box, BoxUnknown)
			}
		})
	}
}

// TestRouteEvent_DisabledHaven tests that disabled HAVEN returns BoxUnknown
func TestRouteEvent_DisabledHaven(t *testing.T) {
	cfg := &Config{
		Enabled:     false,
		OwnerPubkey: ownerPubkey,
	}
	router := NewRouter(cfg)

	event := &nostr.Event{
		Kind:   1,
		PubKey: ownerPubkey,
		Tags:   nostr.Tags{},
	}

	box := router.RouteEvent(event)
	if box != BoxUnknown {
		t.Errorf("RouteEvent() with disabled HAVEN = %v, want %v", box, BoxUnknown)
	}
}

// TestRouteEvent_CustomPrivateKinds tests custom private kinds from config
func TestRouteEvent_CustomPrivateKinds(t *testing.T) {
	cfg := &Config{
		Enabled:      true,
		OwnerPubkey:  ownerPubkey,
		PrivateKinds: []int{99999, 88888},
	}
	router := NewRouter(cfg)

	tests := []struct {
		name        string
		kind        int
		author      string
		expectedBox Box
	}{
		{
			name:        "custom private kind 99999 from owner",
			kind:        99999,
			author:      ownerPubkey,
			expectedBox: BoxPrivate,
		},
		{
			name:        "custom private kind 88888 from owner",
			kind:        88888,
			author:      ownerPubkey,
			expectedBox: BoxPrivate,
		},
		{
			name:        "custom private kind from non-owner",
			kind:        99999,
			author:      alicePubkey,
			expectedBox: BoxUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &nostr.Event{
				Kind:   tt.kind,
				PubKey: tt.author,
				Tags:   nostr.Tags{},
			}

			box := router.RouteEvent(event)
			if box != tt.expectedBox {
				t.Errorf("RouteEvent() = %v, want %v", box, tt.expectedBox)
			}
		})
	}
}

// TestCanAccessBox_PrivateBox tests access control for private box
func TestCanAccessBox_PrivateBox(t *testing.T) {
	tests := []struct {
		name     string
		pubkey   string
		isWrite  bool
		expected bool
	}{
		{
			name:     "owner can read private box",
			pubkey:   ownerPubkey,
			isWrite:  false,
			expected: true,
		},
		{
			name:     "owner can write private box",
			pubkey:   ownerPubkey,
			isWrite:  true,
			expected: true,
		},
		{
			name:     "non-owner cannot read private box",
			pubkey:   alicePubkey,
			isWrite:  false,
			expected: false,
		},
		{
			name:     "non-owner cannot write private box",
			pubkey:   alicePubkey,
			isWrite:  true,
			expected: false,
		},
		{
			name:     "unauthenticated cannot read private box",
			pubkey:   "",
			isWrite:  false,
			expected: false,
		},
		{
			name:     "unauthenticated cannot write private box",
			pubkey:   "",
			isWrite:  true,
			expected: false,
		},
	}

	cfg := &Config{
		Enabled:               true,
		OwnerPubkey:           ownerPubkey,
		RequireAuthForPrivate: true,
	}
	router := NewRouter(cfg)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := router.CanAccessBox(BoxPrivate, tt.pubkey, tt.isWrite)
			if result != tt.expected {
				t.Errorf("CanAccessBox(BoxPrivate, %s, %v) = %v, want %v",
					tt.pubkey, tt.isWrite, result, tt.expected)
			}
		})
	}
}

// TestCanAccessBox_ChatBox tests access control for chat box
func TestCanAccessBox_ChatBox(t *testing.T) {
	tests := []struct {
		name            string
		pubkey          string
		isWrite         bool
		requireAuth     bool
		expected        bool
	}{
		{
			name:        "authenticated user can read chat with auth required",
			pubkey:      alicePubkey,
			isWrite:     false,
			requireAuth: true,
			expected:    true,
		},
		{
			name:        "authenticated user can write chat with auth required",
			pubkey:      alicePubkey,
			isWrite:     true,
			requireAuth: true,
			expected:    true,
		},
		{
			name:        "unauthenticated cannot read chat with auth required",
			pubkey:      "",
			isWrite:     false,
			requireAuth: true,
			expected:    false,
		},
		{
			name:        "unauthenticated cannot write chat with auth required",
			pubkey:      "",
			isWrite:     true,
			requireAuth: true,
			expected:    false,
		},
		{
			name:        "unauthenticated can read chat without auth required",
			pubkey:      "",
			isWrite:     false,
			requireAuth: false,
			expected:    true,
		},
		{
			name:        "owner can access chat",
			pubkey:      ownerPubkey,
			isWrite:     false,
			requireAuth: true,
			expected:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Enabled:            true,
				OwnerPubkey:        ownerPubkey,
				RequireAuthForChat: tt.requireAuth,
			}
			router := NewRouter(cfg)

			result := router.CanAccessBox(BoxChat, tt.pubkey, tt.isWrite)
			if result != tt.expected {
				t.Errorf("CanAccessBox(BoxChat, %s, %v) = %v, want %v",
					tt.pubkey, tt.isWrite, result, tt.expected)
			}
		})
	}
}

// TestCanAccessBox_InboxBox tests access control for inbox box
func TestCanAccessBox_InboxBox(t *testing.T) {
	tests := []struct {
		name              string
		pubkey            string
		isWrite           bool
		allowPublicWrite  bool
		expected          bool
	}{
		{
			name:             "owner can read inbox",
			pubkey:           ownerPubkey,
			isWrite:          false,
			allowPublicWrite: true,
			expected:         true,
		},
		{
			name:             "non-owner cannot read inbox",
			pubkey:           alicePubkey,
			isWrite:          false,
			allowPublicWrite: true,
			expected:         false,
		},
		{
			name:             "unauthenticated cannot read inbox",
			pubkey:           "",
			isWrite:          false,
			allowPublicWrite: true,
			expected:         false,
		},
		{
			name:             "authenticated non-owner can write inbox with public write",
			pubkey:           alicePubkey,
			isWrite:          true,
			allowPublicWrite: true,
			expected:         true,
		},
		{
			name:             "unauthenticated can write inbox with public write",
			pubkey:           "",
			isWrite:          true,
			allowPublicWrite: true,
			expected:         true,
		},
		{
			name:             "unauthenticated cannot write inbox without public write",
			pubkey:           "",
			isWrite:          true,
			allowPublicWrite: false,
			expected:         false,
		},
		{
			name:             "authenticated can write inbox without public write",
			pubkey:           alicePubkey,
			isWrite:          true,
			allowPublicWrite: false,
			expected:         true,
		},
		{
			name:             "owner can write inbox",
			pubkey:           ownerPubkey,
			isWrite:          true,
			allowPublicWrite: true,
			expected:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Enabled:               true,
				OwnerPubkey:           ownerPubkey,
				AllowPublicInboxWrite: tt.allowPublicWrite,
			}
			router := NewRouter(cfg)

			result := router.CanAccessBox(BoxInbox, tt.pubkey, tt.isWrite)
			if result != tt.expected {
				t.Errorf("CanAccessBox(BoxInbox, %s, %v) = %v, want %v",
					tt.pubkey, tt.isWrite, result, tt.expected)
			}
		})
	}
}

// TestCanAccessBox_OutboxBox tests access control for outbox box
func TestCanAccessBox_OutboxBox(t *testing.T) {
	tests := []struct {
		name            string
		pubkey          string
		isWrite         bool
		allowPublicRead bool
		expected        bool
	}{
		{
			name:            "owner can read outbox",
			pubkey:          ownerPubkey,
			isWrite:         false,
			allowPublicRead: true,
			expected:        true,
		},
		{
			name:            "owner can write outbox",
			pubkey:          ownerPubkey,
			isWrite:         true,
			allowPublicRead: true,
			expected:        true,
		},
		{
			name:            "non-owner can read outbox with public read",
			pubkey:          alicePubkey,
			isWrite:         false,
			allowPublicRead: true,
			expected:        true,
		},
		{
			name:            "unauthenticated can read outbox with public read",
			pubkey:          "",
			isWrite:         false,
			allowPublicRead: true,
			expected:        true,
		},
		{
			name:            "non-owner cannot read outbox without public read",
			pubkey:          alicePubkey,
			isWrite:         false,
			allowPublicRead: false,
			expected:        false,
		},
		{
			name:            "unauthenticated cannot read outbox without public read",
			pubkey:          "",
			isWrite:         false,
			allowPublicRead: false,
			expected:        false,
		},
		{
			name:            "non-owner cannot write outbox",
			pubkey:          alicePubkey,
			isWrite:         true,
			allowPublicRead: true,
			expected:        false,
		},
		{
			name:            "unauthenticated cannot write outbox",
			pubkey:          "",
			isWrite:         true,
			allowPublicRead: true,
			expected:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Enabled:               true,
				OwnerPubkey:           ownerPubkey,
				AllowPublicOutboxRead: tt.allowPublicRead,
			}
			router := NewRouter(cfg)

			result := router.CanAccessBox(BoxOutbox, tt.pubkey, tt.isWrite)
			if result != tt.expected {
				t.Errorf("CanAccessBox(BoxOutbox, %s, %v) = %v, want %v",
					tt.pubkey, tt.isWrite, result, tt.expected)
			}
		})
	}
}

// TestCanAccessBox_UnknownBox tests access control for unknown box
func TestCanAccessBox_UnknownBox(t *testing.T) {
	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	router := NewRouter(cfg)

	result := router.CanAccessBox(BoxUnknown, ownerPubkey, false)
	if result {
		t.Error("CanAccessBox(BoxUnknown) should return false")
	}
}

// TestRouteFilter_ByKind tests filter routing based on kinds
func TestRouteFilter_ByKind(t *testing.T) {
	tests := []struct {
		name         string
		kinds        []int
		authedPubkey string
		expectedBoxes []Box
	}{
		{
			name:          "private kinds for owner",
			kinds:         []int{30024, 7375},
			authedPubkey:  ownerPubkey,
			expectedBoxes: []Box{BoxPrivate},
		},
		{
			name:          "chat kinds for authenticated user",
			kinds:         []int{4, 1059},
			authedPubkey:  alicePubkey,
			expectedBoxes: []Box{BoxChat},
		},
		{
			name:          "mixed kinds",
			kinds:         []int{1, 4, 30024},
			authedPubkey:  ownerPubkey,
			expectedBoxes: []Box{BoxPrivate, BoxChat},
		},
	}

	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	router := NewRouter(cfg)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := nostr.Filter{
				Kinds: tt.kinds,
			}

			boxes := router.RouteFilter(filter, tt.authedPubkey)

			// Check if all expected boxes are present
			for _, expectedBox := range tt.expectedBoxes {
				found := false
				for _, box := range boxes {
					if box == expectedBox {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("RouteFilter() missing expected box %v, got %v", expectedBox, boxes)
				}
			}
		})
	}
}

// TestRouteFilter_ByAuthor tests filter routing based on author
func TestRouteFilter_ByAuthor(t *testing.T) {
	tests := []struct {
		name         string
		authors      []string
		authedPubkey string
		wantOutbox   bool
	}{
		{
			name:         "filter by owner author returns outbox",
			authors:      []string{ownerPubkey},
			authedPubkey: alicePubkey,
			wantOutbox:   true,
		},
		{
			name:         "filter by non-owner author does not return outbox",
			authors:      []string{alicePubkey},
			authedPubkey: alicePubkey,
			wantOutbox:   false,
		},
		{
			name:         "filter with owner in authors returns outbox",
			authors:      []string{alicePubkey, ownerPubkey, bobPubkey},
			authedPubkey: alicePubkey,
			wantOutbox:   true,
		},
	}

	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	router := NewRouter(cfg)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := nostr.Filter{
				Authors: tt.authors,
			}

			boxes := router.RouteFilter(filter, tt.authedPubkey)

			foundOutbox := false
			for _, box := range boxes {
				if box == BoxOutbox {
					foundOutbox = true
					break
				}
			}

			if foundOutbox != tt.wantOutbox {
				t.Errorf("RouteFilter() outbox presence = %v, want %v, boxes: %v",
					foundOutbox, tt.wantOutbox, boxes)
			}
		})
	}
}

// TestRouteFilter_ByPTag tests filter routing based on p-tags
func TestRouteFilter_ByPTag(t *testing.T) {
	tests := []struct {
		name         string
		pTags        []string
		authedPubkey string
		wantInbox    bool
	}{
		{
			name:         "filter with owner p-tag returns inbox",
			pTags:        []string{ownerPubkey},
			authedPubkey: alicePubkey,
			wantInbox:    true,
		},
		{
			name:         "filter without owner p-tag does not return inbox",
			pTags:        []string{alicePubkey},
			authedPubkey: alicePubkey,
			wantInbox:    false,
		},
		{
			name:         "filter with owner in p-tags returns inbox",
			pTags:        []string{alicePubkey, ownerPubkey, bobPubkey},
			authedPubkey: alicePubkey,
			wantInbox:    true,
		},
	}

	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	router := NewRouter(cfg)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := nostr.Filter{
				Tags: nostr.TagMap{
					"p": tt.pTags,
				},
			}

			boxes := router.RouteFilter(filter, tt.authedPubkey)

			foundInbox := false
			for _, box := range boxes {
				if box == BoxInbox {
					foundInbox = true
					break
				}
			}

			if foundInbox != tt.wantInbox {
				t.Errorf("RouteFilter() inbox presence = %v, want %v, boxes: %v",
					foundInbox, tt.wantInbox, boxes)
			}
		})
	}
}

// TestRouteFilter_DefaultBehavior tests default filter routing
func TestRouteFilter_DefaultBehavior(t *testing.T) {
	tests := []struct {
		name          string
		authedPubkey  string
		expectedBoxes []Box
	}{
		{
			name:         "authenticated owner gets all boxes",
			authedPubkey: ownerPubkey,
			expectedBoxes: []Box{BoxPrivate, BoxChat, BoxInbox, BoxOutbox},
		},
		{
			name:          "unauthenticated user gets only outbox",
			authedPubkey:  "",
			expectedBoxes: []Box{BoxOutbox},
		},
		{
			name:          "authenticated non-owner gets only outbox",
			authedPubkey:  alicePubkey,
			expectedBoxes: []Box{BoxOutbox},
		},
	}

	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	router := NewRouter(cfg)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Empty filter - no kinds, authors, or tags
			filter := nostr.Filter{}

			boxes := router.RouteFilter(filter, tt.authedPubkey)

			if len(boxes) != len(tt.expectedBoxes) {
				t.Errorf("RouteFilter() returned %d boxes, want %d: got %v, want %v",
					len(boxes), len(tt.expectedBoxes), boxes, tt.expectedBoxes)
				return
			}

			// Check if all expected boxes are present
			for _, expectedBox := range tt.expectedBoxes {
				found := false
				for _, box := range boxes {
					if box == expectedBox {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("RouteFilter() missing expected box %v, got %v", expectedBox, boxes)
				}
			}
		})
	}
}

// TestRouteFilter_DisabledHaven tests that disabled HAVEN returns nil
func TestRouteFilter_DisabledHaven(t *testing.T) {
	cfg := &Config{
		Enabled:     false,
		OwnerPubkey: ownerPubkey,
	}
	router := NewRouter(cfg)

	filter := nostr.Filter{
		Authors: []string{ownerPubkey},
	}

	boxes := router.RouteFilter(filter, alicePubkey)
	if boxes != nil {
		t.Errorf("RouteFilter() with disabled HAVEN = %v, want nil", boxes)
	}
}

// TestGetBoxForKind tests GetBoxForKind method
func TestGetBoxForKind(t *testing.T) {
	tests := []struct {
		name        string
		kind        int
		expectedBox Box
	}{
		{
			name:        "private kind returns private box",
			kind:        30024,
			expectedBox: BoxPrivate,
		},
		{
			name:        "chat kind returns chat box",
			kind:        4,
			expectedBox: BoxChat,
		},
		{
			name:        "gift wrap returns chat box",
			kind:        1059,
			expectedBox: BoxChat,
		},
		{
			name:        "text note returns unknown (depends on author)",
			kind:        1,
			expectedBox: BoxUnknown,
		},
		{
			name:        "unknown kind returns unknown",
			kind:        99999,
			expectedBox: BoxUnknown,
		},
	}

	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	router := NewRouter(cfg)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			box := router.GetBoxForKind(tt.kind)
			if box != tt.expectedBox {
				t.Errorf("GetBoxForKind(%d) = %v, want %v", tt.kind, box, tt.expectedBox)
			}
		})
	}
}

// TestIsOwner tests IsOwner method
func TestIsOwner(t *testing.T) {
	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	router := NewRouter(cfg)

	tests := []struct {
		name     string
		pubkey   string
		expected bool
	}{
		{
			name:     "owner pubkey returns true",
			pubkey:   ownerPubkey,
			expected: true,
		},
		{
			name:     "non-owner pubkey returns false",
			pubkey:   alicePubkey,
			expected: false,
		},
		{
			name:     "empty pubkey returns false",
			pubkey:   "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := router.IsOwner(tt.pubkey)
			if result != tt.expected {
				t.Errorf("IsOwner(%s) = %v, want %v", tt.pubkey, result, tt.expected)
			}
		})
	}
}

// TestIsAddressedToOwner tests the isAddressedToOwner helper
func TestIsAddressedToOwner(t *testing.T) {
	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	router := NewRouter(cfg)

	tests := []struct {
		name     string
		event    *nostr.Event
		expected bool
	}{
		{
			name: "event with owner in p-tag",
			event: &nostr.Event{
				Tags: nostr.Tags{
					{"p", ownerPubkey},
				},
			},
			expected: true,
		},
		{
			name: "event with owner in second p-tag",
			event: &nostr.Event{
				Tags: nostr.Tags{
					{"p", alicePubkey},
					{"p", ownerPubkey},
				},
			},
			expected: true,
		},
		{
			name: "event without owner in p-tags",
			event: &nostr.Event{
				Tags: nostr.Tags{
					{"p", alicePubkey},
					{"p", bobPubkey},
				},
			},
			expected: false,
		},
		{
			name: "event with no p-tags",
			event: &nostr.Event{
				Tags: nostr.Tags{
					{"e", "someeventid"},
				},
			},
			expected: false,
		},
		{
			name: "event with empty tags",
			event: &nostr.Event{
				Tags: nostr.Tags{},
			},
			expected: false,
		},
		{
			name: "event with malformed p-tag",
			event: &nostr.Event{
				Tags: nostr.Tags{
					{"p"}, // Missing pubkey value
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := router.isAddressedToOwner(tt.event)
			if result != tt.expected {
				t.Errorf("isAddressedToOwner() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestIsAddressedToOwner_EmptyOwner tests behavior with empty owner pubkey
func TestIsAddressedToOwner_EmptyOwner(t *testing.T) {
	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: "", // No owner set
	}
	router := NewRouter(cfg)

	event := &nostr.Event{
		Tags: nostr.Tags{
			{"p", "somepubkey"},
		},
	}

	result := router.isAddressedToOwner(event)
	if result {
		t.Error("isAddressedToOwner() with empty owner should return false")
	}
}

// TestNewRouter_NilConfig tests router creation with nil config
func TestNewRouter_NilConfig(t *testing.T) {
	router := NewRouter(nil)
	if router == nil {
		t.Fatal("NewRouter(nil) should return a valid router")
	}
	if router.config == nil {
		t.Error("NewRouter(nil) should use DefaultConfig")
	}
	if router.config.Enabled {
		t.Error("DefaultConfig should have Enabled=false")
	}
}

// TestNewRouter_DefaultConfig tests default configuration
func TestNewRouter_DefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Enabled {
		t.Error("Default config should have Enabled=false")
	}
	if !cfg.AllowPublicOutboxRead {
		t.Error("Default config should allow public outbox read")
	}
	if !cfg.AllowPublicInboxWrite {
		t.Error("Default config should allow public inbox write")
	}
	if !cfg.RequireAuthForChat {
		t.Error("Default config should require auth for chat")
	}
	if !cfg.RequireAuthForPrivate {
		t.Error("Default config should require auth for private")
	}
}

// TestBox_String tests Box.String() method
func TestBox_String(t *testing.T) {
	tests := []struct {
		box      Box
		expected string
	}{
		{BoxPrivate, "private"},
		{BoxChat, "chat"},
		{BoxInbox, "inbox"},
		{BoxOutbox, "outbox"},
		{BoxUnknown, "unknown"},
		{Box(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.box.String(); got != tt.expected {
				t.Errorf("Box.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestDefaultKindConstants tests that default kind slices are correctly defined
func TestDefaultKindConstants(t *testing.T) {
	// Test private kinds
	privateKinds := map[int]bool{
		30024: true, // Draft long-form
		31234: true, // Draft generic
		7375:  true, // Cashu wallet
		7376:  true, // Cashu history
		30078: true, // App-specific data
		10003: true, // Bookmark list
		30003: true, // Bookmark sets
	}

	for _, kind := range DefaultPrivateKinds {
		if !privateKinds[kind] {
			t.Errorf("Unexpected kind %d in DefaultPrivateKinds", kind)
		}
	}

	// Test chat kinds
	chatKinds := map[int]bool{
		4:    true, // Encrypted DM
		13:   true, // Seal
		1059: true, // Gift wrap
		1060: true, // Gift wrap alt
	}

	for _, kind := range DefaultChatKinds {
		if !chatKinds[kind] {
			t.Errorf("Unexpected kind %d in DefaultChatKinds", kind)
		}
	}

	// Test inbox kinds
	if len(DefaultInboxKinds) == 0 {
		t.Error("DefaultInboxKinds should not be empty")
	}

	// Test outbox kinds
	if len(DefaultOutboxKinds) == 0 {
		t.Error("DefaultOutboxKinds should not be empty")
	}
}

// mockEventLookup implements EventLookup for testing E-tag routing
type mockEventLookup struct {
	events map[string]*nostr.Event
}

func (m *mockEventLookup) GetEventByID(ctx context.Context, id string) (*nostr.Event, error) {
	if event, ok := m.events[id]; ok {
		return event, nil
	}
	return nil, nil
}

// TestRouteEvent_ETagRouting_ReactionToOwnerEvent tests that reactions to owner's events go to inbox
func TestRouteEvent_ETagRouting_ReactionToOwnerEvent(t *testing.T) {
	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	router := NewRouter(cfg)

	// Set up mock event lookup with owner's event
	ownerEventID := "ownerevent123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	lookup := &mockEventLookup{
		events: map[string]*nostr.Event{
			ownerEventID: {
				ID:     ownerEventID,
				PubKey: ownerPubkey,
				Kind:   1,
			},
		},
	}
	router.SetEventLookup(lookup)

	// Reaction (kind 7) to owner's event
	reaction := &nostr.Event{
		Kind:   7,
		PubKey: alicePubkey,
		Tags: nostr.Tags{
			{"e", ownerEventID},
		},
	}

	box := router.RouteEvent(reaction)
	if box != BoxInbox {
		t.Errorf("RouteEvent() for reaction to owner's event = %v, want %v", box, BoxInbox)
	}
}

// TestRouteEvent_ETagRouting_RepostOfOwnerEvent tests that reposts of owner's events go to inbox
func TestRouteEvent_ETagRouting_RepostOfOwnerEvent(t *testing.T) {
	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	router := NewRouter(cfg)

	// Set up mock event lookup with owner's event
	ownerEventID := "ownerevent123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	lookup := &mockEventLookup{
		events: map[string]*nostr.Event{
			ownerEventID: {
				ID:     ownerEventID,
				PubKey: ownerPubkey,
				Kind:   1,
			},
		},
	}
	router.SetEventLookup(lookup)

	// Repost (kind 6) of owner's event
	repost := &nostr.Event{
		Kind:   6,
		PubKey: bobPubkey,
		Tags: nostr.Tags{
			{"e", ownerEventID},
		},
	}

	box := router.RouteEvent(repost)
	if box != BoxInbox {
		t.Errorf("RouteEvent() for repost of owner's event = %v, want %v", box, BoxInbox)
	}
}

// TestRouteEvent_ETagRouting_ReactionToOthersEvent tests that reactions to non-owner events return unknown
func TestRouteEvent_ETagRouting_ReactionToOthersEvent(t *testing.T) {
	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	router := NewRouter(cfg)

	// Set up mock event lookup with someone else's event
	otherEventID := "otherevent123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	lookup := &mockEventLookup{
		events: map[string]*nostr.Event{
			otherEventID: {
				ID:     otherEventID,
				PubKey: bobPubkey, // Not the owner
				Kind:   1,
			},
		},
	}
	router.SetEventLookup(lookup)

	// Reaction (kind 7) to someone else's event
	reaction := &nostr.Event{
		Kind:   7,
		PubKey: alicePubkey,
		Tags: nostr.Tags{
			{"e", otherEventID},
		},
	}

	box := router.RouteEvent(reaction)
	if box != BoxUnknown {
		t.Errorf("RouteEvent() for reaction to non-owner's event = %v, want %v", box, BoxUnknown)
	}
}

// TestRouteEvent_ETagRouting_ReactionWithPTag tests that p-tag takes precedence over e-tag lookup
func TestRouteEvent_ETagRouting_ReactionWithPTag(t *testing.T) {
	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	router := NewRouter(cfg)

	// Set up mock event lookup - no events (to ensure e-tag lookup isn't needed)
	lookup := &mockEventLookup{
		events: map[string]*nostr.Event{},
	}
	router.SetEventLookup(lookup)

	// Reaction with p-tag to owner (should go to inbox without e-tag lookup)
	reaction := &nostr.Event{
		Kind:   7,
		PubKey: alicePubkey,
		Tags: nostr.Tags{
			{"p", ownerPubkey},
			{"e", "nonexistenteventid"},
		},
	}

	box := router.RouteEvent(reaction)
	if box != BoxInbox {
		t.Errorf("RouteEvent() for reaction with owner p-tag = %v, want %v", box, BoxInbox)
	}
}

// TestRouteEvent_ETagRouting_WithoutEventLookup tests behavior without event lookup set
func TestRouteEvent_ETagRouting_WithoutEventLookup(t *testing.T) {
	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	router := NewRouter(cfg)
	// Don't set event lookup

	// Reaction without p-tag (can't determine if it's to owner's event)
	reaction := &nostr.Event{
		Kind:   7,
		PubKey: alicePubkey,
		Tags: nostr.Tags{
			{"e", "someeventid"},
		},
	}

	box := router.RouteEvent(reaction)
	if box != BoxUnknown {
		t.Errorf("RouteEvent() for reaction without event lookup = %v, want %v", box, BoxUnknown)
	}
}

// TestRouteEvent_ETagRouting_EventNotFound tests behavior when referenced event is not found
func TestRouteEvent_ETagRouting_EventNotFound(t *testing.T) {
	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	router := NewRouter(cfg)

	// Set up mock event lookup with no events
	lookup := &mockEventLookup{
		events: map[string]*nostr.Event{},
	}
	router.SetEventLookup(lookup)

	// Reaction to non-existent event
	reaction := &nostr.Event{
		Kind:   7,
		PubKey: alicePubkey,
		Tags: nostr.Tags{
			{"e", "nonexistenteventid"},
		},
	}

	box := router.RouteEvent(reaction)
	if box != BoxUnknown {
		t.Errorf("RouteEvent() for reaction to non-existent event = %v, want %v", box, BoxUnknown)
	}
}

// TestRouteEvent_ETagRouting_MultipleETags tests that any e-tag referencing owner's event routes to inbox
func TestRouteEvent_ETagRouting_MultipleETags(t *testing.T) {
	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	router := NewRouter(cfg)

	// Set up mock event lookup with owner's event as second e-tag
	ownerEventID := "ownerevent123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	otherEventID := "otherevent123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	lookup := &mockEventLookup{
		events: map[string]*nostr.Event{
			ownerEventID: {
				ID:     ownerEventID,
				PubKey: ownerPubkey,
				Kind:   1,
			},
			otherEventID: {
				ID:     otherEventID,
				PubKey: bobPubkey,
				Kind:   1,
			},
		},
	}
	router.SetEventLookup(lookup)

	// Reaction with multiple e-tags, owner's event is second
	reaction := &nostr.Event{
		Kind:   7,
		PubKey: alicePubkey,
		Tags: nostr.Tags{
			{"e", otherEventID},
			{"e", ownerEventID},
		},
	}

	box := router.RouteEvent(reaction)
	if box != BoxInbox {
		t.Errorf("RouteEvent() for reaction with owner's event in multiple e-tags = %v, want %v", box, BoxInbox)
	}
}

// TestRouteEvent_ETagRouting_NonReactionKinds tests that e-tag routing only applies to kind 6 and 7
func TestRouteEvent_ETagRouting_NonReactionKinds(t *testing.T) {
	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	router := NewRouter(cfg)

	// Set up mock event lookup with owner's event
	ownerEventID := "ownerevent123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	lookup := &mockEventLookup{
		events: map[string]*nostr.Event{
			ownerEventID: {
				ID:     ownerEventID,
				PubKey: ownerPubkey,
				Kind:   1,
			},
		},
	}
	router.SetEventLookup(lookup)

	// Text note (kind 1) with e-tag referencing owner's event - should NOT use e-tag routing
	textNote := &nostr.Event{
		Kind:   1,
		PubKey: alicePubkey,
		Tags: nostr.Tags{
			{"e", ownerEventID},
		},
	}

	box := router.RouteEvent(textNote)
	// Kind 1 without p-tag to owner should be unknown (e-tag routing is only for kind 6/7)
	if box != BoxUnknown {
		t.Errorf("RouteEvent() for text note with e-tag = %v, want %v (e-tag routing is only for kind 6/7)", box, BoxUnknown)
	}
}

// TestSetEventLookup tests the SetEventLookup method
func TestSetEventLookup(t *testing.T) {
	cfg := &Config{
		Enabled:     true,
		OwnerPubkey: ownerPubkey,
	}
	router := NewRouter(cfg)

	// Initially no event lookup
	if router.eventLookup != nil {
		t.Error("Router should initially have nil eventLookup")
	}

	// Set event lookup
	lookup := &mockEventLookup{}
	router.SetEventLookup(lookup)

	if router.eventLookup == nil {
		t.Error("SetEventLookup should set the eventLookup field")
	}
}

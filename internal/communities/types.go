// Package communities implements NIP-72 moderated communities
//
// NIP-72 provides Reddit-style communities where:
// - Anyone can post content to a community
// - Moderators approve posts for display
// - Approved posts are visible to the community
//
// Unlike NIP-29 relay-based groups, communities are:
// - Public by default (anyone can post)
// - Moderation-based (approvals instead of membership)
// - Distributed (can span multiple relays)
//
// Reference: https://github.com/nostr-protocol/nips/blob/master/72.md
package communities

import (
	"encoding/json"

	"github.com/nbd-wtf/go-nostr"
)

// Event kinds for NIP-72
const (
	// KindCommunityDefinition is the replaceable event defining a community
	KindCommunityDefinition = 34550

	// KindCommunityPost is a post to a community (NIP-22 compatible)
	KindCommunityPost = 1111

	// KindApproval is a moderator approval event
	KindApproval = 4550
)

// Moderator represents a community moderator
type Moderator struct {
	// Pubkey is the moderator's hex pubkey
	Pubkey string
	// RelayHint is an optional relay URL for the moderator
	RelayHint string
	// Role is an optional role description
	Role string
}

// Community represents a NIP-72 community definition
type Community struct {
	// ID is the community identifier (d-tag value)
	ID string
	// OwnerPubkey is the pubkey that created the community
	OwnerPubkey string
	// Name is the display name
	Name string
	// Description is the community description
	Description string
	// Image is the community image URL
	Image string
	// ImageDimensions are optional width x height
	ImageDimensions string
	// Rules are the community rules
	Rules string
	// Moderators are the community moderators
	Moderators []Moderator
	// RelayURLs are preferred relays for the community
	RelayURLs []string
}

// Approval represents a moderator approval event
type Approval struct {
	// CommunityRef is the a-tag reference to the community
	CommunityRef string
	// PostID is the event ID being approved (e-tag)
	PostID string
	// PostRef is the addressable event reference (a-tag) if applicable
	PostRef string
	// AuthorPubkey is the post author's pubkey (p-tag)
	AuthorPubkey string
	// PostKind is the kind of the approved post (k-tag)
	PostKind int
	// PostJSON is the full post event in the content field
	PostJSON string
}

// ParseCommunityDefinition parses a kind 34550 event into a Community
func ParseCommunityDefinition(event *nostr.Event) *Community {
	if event.Kind != KindCommunityDefinition {
		return nil
	}

	community := &Community{
		OwnerPubkey: event.PubKey,
	}

	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}

		switch tag[0] {
		case "d":
			community.ID = tag[1]
		case "name":
			community.Name = tag[1]
		case "description":
			community.Description = tag[1]
		case "image":
			community.Image = tag[1]
			if len(tag) >= 3 {
				community.ImageDimensions = tag[2]
			}
		case "rules":
			community.Rules = tag[1]
		case "relay":
			community.RelayURLs = append(community.RelayURLs, tag[1])
		case "p":
			// Check if it's a moderator
			isModerator := false
			relayHint := ""
			role := ""

			for i, v := range tag {
				if i == 2 && v != "" {
					relayHint = v
				}
				if v == "moderator" {
					isModerator = true
				}
				if i >= 3 && v != "moderator" && v != "" {
					role = v
				}
			}

			if isModerator {
				community.Moderators = append(community.Moderators, Moderator{
					Pubkey:    tag[1],
					RelayHint: relayHint,
					Role:      role,
				})
			}
		}
	}

	return community
}

// CreateCommunityDefinitionEvent creates a kind 34550 community definition
func CreateCommunityDefinitionEvent(community *Community) *nostr.Event {
	event := &nostr.Event{
		Kind:      KindCommunityDefinition,
		PubKey:    community.OwnerPubkey,
		CreatedAt: nostr.Now(),
		Content:   "",
		Tags:      nostr.Tags{},
	}

	// Add d-tag (required for replaceable events)
	event.Tags = append(event.Tags, nostr.Tag{"d", community.ID})

	// Add metadata tags
	if community.Name != "" {
		event.Tags = append(event.Tags, nostr.Tag{"name", community.Name})
	}
	if community.Description != "" {
		event.Tags = append(event.Tags, nostr.Tag{"description", community.Description})
	}
	if community.Image != "" {
		if community.ImageDimensions != "" {
			event.Tags = append(event.Tags, nostr.Tag{"image", community.Image, community.ImageDimensions})
		} else {
			event.Tags = append(event.Tags, nostr.Tag{"image", community.Image})
		}
	}
	if community.Rules != "" {
		event.Tags = append(event.Tags, nostr.Tag{"rules", community.Rules})
	}

	// Add relay URLs
	for _, relay := range community.RelayURLs {
		event.Tags = append(event.Tags, nostr.Tag{"relay", relay})
	}

	// Add moderators
	for _, mod := range community.Moderators {
		tag := nostr.Tag{"p", mod.Pubkey}
		if mod.RelayHint != "" {
			tag = append(tag, mod.RelayHint)
		} else {
			tag = append(tag, "")
		}
		tag = append(tag, "moderator")
		if mod.Role != "" {
			tag = append(tag, mod.Role)
		}
		event.Tags = append(event.Tags, tag)
	}

	return event
}

// IsCommunityPost checks if an event is a valid community post
func IsCommunityPost(event *nostr.Event) bool {
	if event.Kind != KindCommunityPost {
		return false
	}

	// Must have A tag referencing a community
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "A" {
			return true
		}
	}

	return false
}

// GetCommunityRef extracts the community reference from a post
func GetCommunityRef(event *nostr.Event) string {
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "A" {
			return tag[1]
		}
	}
	return ""
}

// ParseApproval parses a kind 4550 approval event
func ParseApproval(event *nostr.Event) *Approval {
	if event.Kind != KindApproval {
		return nil
	}

	approval := &Approval{
		PostJSON: event.Content,
	}

	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}

		switch tag[0] {
		case "a":
			// Could be community ref or post ref
			// Community ref format: 34550:pubkey:d-tag
			// Post ref format: other-kind:pubkey:d-tag
			if len(tag[1]) > 5 && tag[1][:5] == "34550" {
				approval.CommunityRef = tag[1]
			} else {
				approval.PostRef = tag[1]
			}
		case "e":
			approval.PostID = tag[1]
		case "p":
			approval.AuthorPubkey = tag[1]
		case "k":
			// Parse kind as int
			var kind int
			if _, err := json.Marshal(tag[1]); err == nil {
				// Try to parse
				for _, c := range tag[1] {
					if c >= '0' && c <= '9' {
						kind = kind*10 + int(c-'0')
					}
				}
			}
			approval.PostKind = kind
		}
	}

	return approval
}

// CreateApprovalEvent creates a kind 4550 approval event
func CreateApprovalEvent(moderatorPubkey string, approval *Approval, postEvent *nostr.Event) *nostr.Event {
	event := &nostr.Event{
		Kind:      KindApproval,
		PubKey:    moderatorPubkey,
		CreatedAt: nostr.Now(),
		Tags:      nostr.Tags{},
	}

	// Include the post event JSON in content
	if postEvent != nil {
		postJSON, _ := json.Marshal(postEvent)
		event.Content = string(postJSON)
	} else {
		event.Content = approval.PostJSON
	}

	// Add community reference
	if approval.CommunityRef != "" {
		event.Tags = append(event.Tags, nostr.Tag{"a", approval.CommunityRef})
	}

	// Add post reference
	if approval.PostID != "" {
		event.Tags = append(event.Tags, nostr.Tag{"e", approval.PostID})
	}
	if approval.PostRef != "" {
		event.Tags = append(event.Tags, nostr.Tag{"a", approval.PostRef})
	}

	// Add author pubkey
	if approval.AuthorPubkey != "" {
		event.Tags = append(event.Tags, nostr.Tag{"p", approval.AuthorPubkey})
	}

	// Add post kind
	if approval.PostKind > 0 {
		event.Tags = append(event.Tags, nostr.Tag{"k", string(rune(approval.PostKind + '0'))})
	}

	return event
}

// IsCommunityKind returns true if the kind is NIP-72 related
func IsCommunityKind(kind int) bool {
	switch kind {
	case KindCommunityDefinition, KindCommunityPost, KindApproval:
		return true
	default:
		return false
	}
}

// IsModerator checks if a pubkey is a moderator of a community
func (c *Community) IsModerator(pubkey string) bool {
	// Owner is always a moderator
	if c.OwnerPubkey == pubkey {
		return true
	}

	for _, mod := range c.Moderators {
		if mod.Pubkey == pubkey {
			return true
		}
	}

	return false
}

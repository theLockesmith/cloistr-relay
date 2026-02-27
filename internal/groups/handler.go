package groups

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"
)

// Handler manages NIP-29 group event processing
type Handler struct {
	store    *Store
	config   *Config
	relay    *khatru.Relay
}

// NewHandler creates a new NIP-29 handler
func NewHandler(store *Store, cfg *Config) *Handler {
	return &Handler{
		store:  store,
		config: cfg,
	}
}

// RegisterHandlers registers all NIP-29 handlers with the relay
func (h *Handler) RegisterHandlers(relay *khatru.Relay) {
	h.relay = relay

	// Reject events that don't meet NIP-29 requirements
	relay.RejectEvent = append(relay.RejectEvent, h.RejectEvent())

	// Reject filters for groups the user can't access
	relay.RejectFilter = append(relay.RejectFilter, h.RejectFilter())

	// Process moderation and management events
	relay.OnEventSaved = append(relay.OnEventSaved, h.OnEventSaved())

	log.Println("NIP-29: handlers registered")
}

// getGroupIDFromEvent extracts the group ID from the h tag
func getGroupIDFromEvent(event *nostr.Event) string {
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "h" {
			return tag[1]
		}
	}
	return ""
}

// getGroupIDFromFilter extracts the group ID from a filter's #h tag
func getGroupIDFromFilter(filter nostr.Filter) string {
	if hTags, ok := filter.Tags["h"]; ok && len(hTags) > 0 {
		return hTags[0]
	}
	return ""
}

// RejectEvent validates events against group membership and permissions
func (h *Handler) RejectEvent() func(context.Context, *nostr.Event) (bool, string) {
	return func(ctx context.Context, event *nostr.Event) (bool, string) {
		if !h.config.Enabled {
			return false, ""
		}

		// Check for h tag (group identifier)
		groupID := getGroupIDFromEvent(event)
		if groupID == "" {
			// Not a group event, allow
			return false, ""
		}

		authedPubkey := khatru.GetAuthed(ctx)

		// Handle group creation
		if event.Kind == KindCreateGroup {
			return h.handleCreateGroupEvent(ctx, event, authedPubkey)
		}

		// Get the group
		group, err := h.store.GetGroup(ctx, groupID)
		if err != nil {
			if err == ErrGroupNotFound {
				return true, "restricted: group not found"
			}
			log.Printf("NIP-29: error getting group: %v", err)
			return true, "error: internal server error"
		}

		// Handle moderation events
		if IsModeratorKind(event.Kind) {
			return h.handleModerationEvent(ctx, event, group, authedPubkey)
		}

		// Handle management events (join/leave)
		if IsGroupManagementKind(event.Kind) {
			return h.handleManagementEvent(ctx, event, group, authedPubkey)
		}

		// Regular group event - check write permission
		isMember := h.store.IsMember(ctx, groupID, event.PubKey)
		if !group.Privacy.CanWrite(isMember) {
			if authedPubkey == "" {
				return true, "auth-required: authentication required to post to this group"
			}
			return true, "restricted: only members can post to this group"
		}

		return false, ""
	}
}

// handleCreateGroupEvent processes group creation
func (h *Handler) handleCreateGroupEvent(ctx context.Context, event *nostr.Event, authedPubkey string) (bool, string) {
	// Check if user can create groups
	if !h.config.AllowPublicGroupCreation {
		isAdmin := false
		for _, admin := range h.config.AdminPubkeys {
			if event.PubKey == admin {
				isAdmin = true
				break
			}
		}
		if !isAdmin {
			return true, "restricted: group creation not allowed"
		}
	}

	// Require authentication
	if authedPubkey == "" || authedPubkey != event.PubKey {
		return true, "auth-required: authentication required to create groups"
	}

	return false, ""
}

// handleModerationEvent processes moderation events
func (h *Handler) handleModerationEvent(ctx context.Context, event *nostr.Event, group *Group, authedPubkey string) (bool, string) {
	// Require authentication
	if authedPubkey == "" || authedPubkey != event.PubKey {
		return true, "auth-required: authentication required for moderation"
	}

	// Check if user is admin
	if !h.store.IsAdmin(ctx, group.ID, event.PubKey) {
		return true, "restricted: only admins can perform moderation actions"
	}

	return false, ""
}

// handleManagementEvent processes join/leave requests
func (h *Handler) handleManagementEvent(ctx context.Context, event *nostr.Event, group *Group, authedPubkey string) (bool, string) {
	// Require authentication
	if authedPubkey == "" || authedPubkey != event.PubKey {
		return true, "auth-required: authentication required"
	}

	if event.Kind == KindJoinRequest {
		// Check if joins are allowed
		if !group.Privacy.CanJoin() {
			return true, "restricted: this group does not accept join requests"
		}
	}

	return false, ""
}

// RejectFilter validates filter access against group permissions
func (h *Handler) RejectFilter() func(context.Context, nostr.Filter) (bool, string) {
	return func(ctx context.Context, filter nostr.Filter) (bool, string) {
		if !h.config.Enabled {
			return false, ""
		}

		groupID := getGroupIDFromFilter(filter)
		if groupID == "" {
			// Not a group query, allow
			return false, ""
		}

		// Get the group
		group, err := h.store.GetGroup(ctx, groupID)
		if err != nil {
			if err == ErrGroupNotFound {
				return true, "restricted: group not found"
			}
			return true, "error: internal server error"
		}

		authedPubkey := khatru.GetAuthed(ctx)
		isMember := h.store.IsMember(ctx, groupID, authedPubkey)

		// Check read permission
		if !group.Privacy.CanRead(isMember) {
			if authedPubkey == "" {
				return true, "auth-required: authentication required to read this group"
			}
			return true, "restricted: only members can read this group"
		}

		// Check metadata visibility
		if !group.Privacy.ShowMetadata(isMember) {
			// Check if querying metadata kinds
			for _, kind := range filter.Kinds {
				if IsGroupMetadataKind(kind) {
					return true, "restricted: group metadata is hidden"
				}
			}
		}

		return false, ""
	}
}

// OnEventSaved processes events after they're saved
func (h *Handler) OnEventSaved() func(context.Context, *nostr.Event) {
	return func(ctx context.Context, event *nostr.Event) {
		if !h.config.Enabled {
			return
		}

		groupID := getGroupIDFromEvent(event)
		if groupID == "" {
			return
		}

		// Process based on event kind
		switch event.Kind {
		case KindCreateGroup:
			h.processCreateGroup(ctx, event, groupID)
		case KindDeleteGroup:
			h.processDeleteGroup(ctx, event, groupID)
		case KindAddUser:
			h.processAddUser(ctx, event, groupID)
		case KindRemoveUser:
			h.processRemoveUser(ctx, event, groupID)
		case KindEditMetadata:
			h.processEditMetadata(ctx, event, groupID)
		case KindJoinRequest:
			h.processJoinRequest(ctx, event, groupID)
		case KindLeaveRequest:
			h.processLeaveRequest(ctx, event, groupID)
		case KindCreateInvite:
			h.processCreateInvite(ctx, event, groupID)
		}
	}
}

// processCreateGroup handles group creation
func (h *Handler) processCreateGroup(ctx context.Context, event *nostr.Event, groupID string) {
	// Extract group name from tags
	name := "Unnamed Group"
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "name" {
			name = tag[1]
			break
		}
	}

	privacy := h.config.DefaultPrivacy
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "privacy" {
			privacy = Privacy(tag[1])
			break
		}
	}

	group, err := h.store.CreateGroup(ctx, event.PubKey, name, privacy)
	if err != nil {
		log.Printf("NIP-29: failed to create group: %v", err)
		return
	}

	// Publish group metadata
	h.publishGroupMetadata(ctx, group)
}

// processDeleteGroup handles group deletion
func (h *Handler) processDeleteGroup(ctx context.Context, event *nostr.Event, groupID string) {
	if err := h.store.DeleteGroup(ctx, groupID); err != nil {
		log.Printf("NIP-29: failed to delete group: %v", err)
	}
}

// processAddUser handles adding users to a group
func (h *Handler) processAddUser(ctx context.Context, event *nostr.Event, groupID string) {
	// Extract pubkey and role from tags
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

	if pubkey == "" {
		log.Printf("NIP-29: add user event missing pubkey")
		return
	}

	if err := h.store.AddMember(ctx, groupID, pubkey, role, event.PubKey); err != nil {
		log.Printf("NIP-29: failed to add user: %v", err)
		return
	}

	// Update published member list
	h.publishMemberList(ctx, groupID)
}

// processRemoveUser handles removing users from a group
func (h *Handler) processRemoveUser(ctx context.Context, event *nostr.Event, groupID string) {
	// Extract pubkey from p tag
	var pubkey string
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "p" {
			pubkey = tag[1]
			break
		}
	}

	if pubkey == "" {
		log.Printf("NIP-29: remove user event missing pubkey")
		return
	}

	if err := h.store.RemoveMember(ctx, groupID, pubkey); err != nil {
		log.Printf("NIP-29: failed to remove user: %v", err)
		return
	}

	// Update published member list
	h.publishMemberList(ctx, groupID)
}

// processEditMetadata handles metadata updates
func (h *Handler) processEditMetadata(ctx context.Context, event *nostr.Event, groupID string) {
	var name, picture, about string
	var privacy Privacy

	for _, tag := range event.Tags {
		if len(tag) >= 2 {
			switch tag[0] {
			case "name":
				name = tag[1]
			case "picture":
				picture = tag[1]
			case "about":
				about = tag[1]
			case "privacy":
				privacy = Privacy(tag[1])
			}
		}
	}

	if err := h.store.UpdateGroupMetadata(ctx, groupID, name, picture, about, privacy); err != nil {
		log.Printf("NIP-29: failed to update metadata: %v", err)
		return
	}

	// Publish updated metadata
	group, _ := h.store.GetGroup(ctx, groupID)
	if group != nil {
		h.publishGroupMetadata(ctx, group)
	}
}

// processJoinRequest handles join requests
func (h *Handler) processJoinRequest(ctx context.Context, event *nostr.Event, groupID string) {
	// Check for invite code
	var inviteCode string
	for _, tag := range event.Tags {
		if len(tag) >= 2 && tag[0] == "code" {
			inviteCode = tag[1]
			break
		}
	}

	if inviteCode != "" {
		// Use invite code
		_, err := h.store.UseInvite(ctx, inviteCode, event.PubKey)
		if err != nil {
			log.Printf("NIP-29: failed to use invite: %v", err)
			return
		}
	} else {
		// Regular join request - add as pending member
		// For now, auto-approve for open/restricted groups
		group, err := h.store.GetGroup(ctx, groupID)
		if err != nil {
			return
		}

		if group.Privacy == PrivacyOpen || group.Privacy == PrivacyRestricted {
			if err := h.store.AddMember(ctx, groupID, event.PubKey, "", "self"); err != nil {
				log.Printf("NIP-29: failed to add member: %v", err)
				return
			}
		}
	}

	// Update member list
	h.publishMemberList(ctx, groupID)
}

// processLeaveRequest handles leave requests
func (h *Handler) processLeaveRequest(ctx context.Context, event *nostr.Event, groupID string) {
	if err := h.store.RemoveMember(ctx, groupID, event.PubKey); err != nil {
		log.Printf("NIP-29: failed to remove member: %v", err)
		return
	}

	// Update member list
	h.publishMemberList(ctx, groupID)
}

// processCreateInvite handles invite code creation
func (h *Handler) processCreateInvite(ctx context.Context, event *nostr.Event, groupID string) {
	maxUses := 0
	var expiresAt time.Time

	for _, tag := range event.Tags {
		if len(tag) >= 2 {
			switch tag[0] {
			case "max_uses":
				_, _ = fmt.Sscanf(tag[1], "%d", &maxUses)
			case "expires":
				// Parse ISO timestamp
				expiresAt, _ = time.Parse(time.RFC3339, tag[1])
			}
		}
	}

	if expiresAt.IsZero() {
		expiresAt = time.Now().Add(h.config.InviteCodeExpiry)
	}

	invite, err := h.store.CreateInvite(ctx, groupID, event.PubKey, maxUses, expiresAt)
	if err != nil {
		log.Printf("NIP-29: failed to create invite: %v", err)
		return
	}

	log.Printf("NIP-29: created invite %s for group %s", invite.Code[:8], groupID[:8])
}

// publishGroupMetadata publishes kind 39000 event
func (h *Handler) publishGroupMetadata(ctx context.Context, group *Group) {
	if h.relay == nil {
		return
	}

	content, _ := json.Marshal(GroupMetadata{
		ID:      group.ID,
		Name:    group.Name,
		Picture: group.Picture,
		About:   group.About,
		Privacy: group.Privacy,
	})

	// Note: In a real implementation, this would be signed by the relay's key
	// For now, we just log that metadata should be published
	log.Printf("NIP-29: would publish metadata for group %s: %s", group.ID[:8], string(content))
}

// publishMemberList publishes kind 39002 event
func (h *Handler) publishMemberList(ctx context.Context, groupID string) {
	members, err := h.store.ListMembers(ctx, groupID)
	if err != nil {
		log.Printf("NIP-29: failed to list members: %v", err)
		return
	}

	tags := make([]string, 0, len(members))
	for _, m := range members {
		tags = append(tags, m.Pubkey)
	}
	_ = tags // Used for future publishing

	log.Printf("NIP-29: would publish member list for group %s: %d members", groupID[:8], len(members))
}

// truncateID safely truncates an ID for logging
func truncateID(s string) string {
	if len(s) >= 8 {
		return s[:8]
	}
	return s
}

// Helper to check if a string slice contains a value
func contains(slice []string, val string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, val) {
			return true
		}
	}
	return false
}

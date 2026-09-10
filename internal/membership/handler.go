package membership

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

// JoinHandler processes NIP-43 join requests (kind 28934) and publishes
// membership notifications (kind 8000). It validates invite codes, adds
// the member at TierFree, and signs a notification with the relay's key.
type JoinHandler struct {
	store     *Store
	secretKey string // relay signing key (hex)
	pubkey    string // derived from secretKey

	// addEvent stores and broadcasts an event via khatru. Injected from
	// main so the handler does not import cmd/.
	addEvent func(ctx context.Context, evt *nostr.Event) (bool, error)

	// requireInvite means a valid invite code is mandatory. When false,
	// join requests without an invite are accepted as free-tier members.
	requireInvite bool
}

// JoinHandlerConfig configures the NIP-43 join handler.
type JoinHandlerConfig struct {
	Store         *Store
	SecretKey     string // RELAY_SECRET_KEY (hex)
	RequireInvite bool   // Reject join requests that lack a valid invite
	AddEvent      func(ctx context.Context, evt *nostr.Event) (bool, error)
}

// NewJoinHandler creates a handler for NIP-43 join requests.
// Returns an error if the secret key is invalid.
func NewJoinHandler(cfg JoinHandlerConfig) (*JoinHandler, error) {
	if cfg.SecretKey == "" {
		return nil, fmt.Errorf("membership: RELAY_SECRET_KEY is required for NIP-43 join handler")
	}
	pubkey, err := nostr.GetPublicKey(cfg.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("membership: invalid RELAY_SECRET_KEY: %w", err)
	}
	if cfg.AddEvent == nil {
		return nil, fmt.Errorf("membership: AddEvent function is required")
	}
	return &JoinHandler{
		store:         cfg.Store,
		secretKey:     cfg.SecretKey,
		pubkey:        pubkey,
		addEvent:      cfg.AddEvent,
		requireInvite: cfg.RequireInvite,
	}, nil
}

// RejectJoinRequest returns a khatru RejectEvent handler that validates
// kind 28934 join requests. It rejects events that fail validation; valid
// join requests pass through so khatru stores them normally.
func (h *JoinHandler) RejectJoinRequest() func(ctx context.Context, event *nostr.Event) (reject bool, msg string) {
	return func(ctx context.Context, event *nostr.Event) (bool, string) {
		if event.Kind != KindJoinRequest {
			return false, "" // not our kind
		}

		// NIP-43 join requests must have the NIP-70 protected tag.
		if !HasProtectedTag(event) {
			return true, "restricted: join requests require the NIP-70 protected tag"
		}

		pubkey, inviteCode := ParseJoinRequest(event)
		if pubkey == "" {
			return true, "invalid: join request missing pubkey"
		}

		// Already a member? Allow the event through (idempotent).
		isMember, err := h.store.IsMember(ctx, pubkey)
		if err != nil {
			log.Printf("membership: error checking membership for %s: %v", pubkey[:8], err)
			return true, "error: internal error checking membership"
		}
		if isMember {
			// Already a member. Let the event store (it's harmless) but
			// don't re-process it in OnEventSaved.
			return false, ""
		}

		// Validate invite code if required or provided.
		if inviteCode != "" {
			invite, err := h.store.GetInvite(ctx, inviteCode)
			if err != nil {
				log.Printf("membership: error looking up invite %s: %v", inviteCode, err)
				return true, "error: internal error validating invite"
			}
			if invite == nil {
				return true, "restricted: invalid invite code"
			}
			if !invite.IsValidInvite() {
				return true, "restricted: invite code expired or fully used"
			}
		} else if h.requireInvite {
			return true, "restricted: an invite code is required to join this relay"
		}

		return false, ""
	}
}

// OnJoinRequestSaved returns a khatru OnEventSaved handler that processes
// accepted join requests: adds the member, consumes the invite, and
// publishes a kind 8000 notification.
func (h *JoinHandler) OnJoinRequestSaved() func(ctx context.Context, event *nostr.Event) {
	return func(ctx context.Context, event *nostr.Event) {
		if event.Kind != KindJoinRequest {
			return
		}

		pubkey, inviteCode := ParseJoinRequest(event)

		// Re-check membership (race guard).
		isMember, err := h.store.IsMember(ctx, pubkey)
		if err != nil {
			log.Printf("membership: join post-save membership check failed for %s: %v", pubkey[:8], err)
			return
		}
		if isMember {
			return // already processed
		}

		// Consume invite if one was provided.
		if inviteCode != "" {
			if err := h.store.UseInvite(ctx, inviteCode); err != nil {
				log.Printf("membership: failed to consume invite %s for %s: %v", inviteCode, pubkey[:8], err)
				// Continue anyway; the invite was validated in RejectJoinRequest.
			}
		}

		// Add the member at free tier.
		member := Member{
			Pubkey:     pubkey,
			JoinedAt:   time.Now(),
			InviteCode: inviteCode,
			Tier:       TierFree,
		}
		if err := h.store.AddMember(ctx, member); err != nil {
			log.Printf("membership: failed to add member %s: %v", pubkey[:8], err)
			return
		}

		log.Printf("membership: added %s as free-tier member (invite=%q)", pubkey[:8], inviteCode)

		// Publish kind 8000 add-member notification.
		notification := CreateAddMemberEvent(h.pubkey, pubkey)
		if err := notification.Sign(h.secretKey); err != nil {
			log.Printf("membership: failed to sign kind 8000 for %s: %v", pubkey[:8], err)
			return
		}
		if _, err := h.addEvent(ctx, notification); err != nil {
			log.Printf("membership: failed to publish kind 8000 for %s: %v", pubkey[:8], err)
			return
		}

		log.Printf("membership: published kind 8000 notification for %s", pubkey[:8])
	}
}

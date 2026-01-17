package handlers

import (
	"context"
	"encoding/json"
	"log"

	"github.com/fiatjaf/khatru"
	"gitlab.com/coldforge/coldforge-relay/internal/relay"
)

// RegisterHandlers registers all event handlers with the relay
func RegisterHandlers(r *relay.Relay) {
	// NIP-01: Accept Events
	// This handler is called when a client sends a new event
	r.OnEvent([]func(*khatru.Event, *khatru.Client) error{
		func(evt *khatru.Event, client *khatru.Client) error {
			log.Printf("Received event: kind=%d from %s", evt.Kind, evt.PubKey)
			// Validate and store the event
			// The actual storage is handled by khatru's backend
			return nil
		},
	})

	// NIP-01: Handle Subscriptions
	// This handler is called when a client creates a subscription
	r.OnSubscribe([]func(*khatru.Subscription, *khatru.Client) error{
		func(sub *khatru.Subscription, client *khatru.Client) error {
			log.Printf("New subscription: %s with %d filters", sub.Id, len(sub.Filters))
			// Query events matching the subscription filters
			// Results are handled by khatru's subscription system
			return nil
		},
	})

	// Handle disconnections
	r.OnDisconnect([]func(*khatru.Client){
		func(client *khatru.Client) {
			log.Printf("Client disconnected: %v", client.RemoteAddr)
		},
	})

	log.Println("Event handlers registered")
}

// LogEvent logs event details for debugging
func LogEvent(evt *khatru.Event) {
	data, _ := json.MarshalIndent(evt, "", "  ")
	log.Printf("Event details: %s", string(data))
}

// ValidateEvent performs custom validation on events
// Returns true if the event is valid, false otherwise
func ValidateEvent(ctx context.Context, evt *khatru.Event) bool {
	// Add custom validation logic here
	// For now, accept all valid Nostr events
	return evt.Sig != "" && evt.PubKey != "" && evt.ID != ""
}

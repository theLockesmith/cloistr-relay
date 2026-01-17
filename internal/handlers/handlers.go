package handlers

import (
	"context"
	"log"

	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"
)

// RegisterHandlers registers all event handlers with the relay
func RegisterHandlers(relay *khatru.Relay) {
	// Reject events based on custom policies
	relay.RejectEvent = append(relay.RejectEvent, rejectInvalidEvents)

	// Reject filters based on custom policies
	relay.RejectFilter = append(relay.RejectFilter, rejectComplexFilters)

	// Log connections
	relay.OnConnect = append(relay.OnConnect, func(ctx context.Context) {
		log.Printf("Client connected")
	})

	// Log disconnections
	relay.OnDisconnect = append(relay.OnDisconnect, func(ctx context.Context) {
		log.Printf("Client disconnected")
	})

	log.Println("Event handlers registered")
}

// rejectInvalidEvents validates events and rejects invalid ones
func rejectInvalidEvents(ctx context.Context, event *nostr.Event) (reject bool, msg string) {
	// Verify event signature
	ok, err := event.CheckSignature()
	if err != nil || !ok {
		return true, "invalid: signature verification failed"
	}

	// Verify event ID matches content hash
	if event.GetID() != event.ID {
		return true, "invalid: event id mismatch"
	}

	// Reject events too far in the future (5 minutes tolerance)
	if event.CreatedAt.Time().Unix() > nostr.Now().Time().Unix()+300 {
		return true, "invalid: event created_at too far in the future"
	}

	return false, ""
}

// rejectComplexFilters prevents resource-intensive queries
func rejectComplexFilters(ctx context.Context, filter nostr.Filter) (reject bool, msg string) {
	// Limit the number of authors in a single filter
	if len(filter.Authors) > 100 {
		return true, "error: too many authors in filter"
	}

	// Limit the number of IDs in a single filter
	if len(filter.IDs) > 500 {
		return true, "error: too many ids in filter"
	}

	// Limit the number of kinds in a single filter
	if len(filter.Kinds) > 20 {
		return true, "error: too many kinds in filter"
	}

	return false, ""
}

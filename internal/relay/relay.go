package relay

import (
	"context"
	"log"

	"github.com/fiatjaf/eventstore/postgresql"
	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"
	"gitlab.com/coldforge/coldforge-relay/internal/config"
	"gitlab.com/coldforge/coldforge-relay/internal/handlers"
	"gitlab.com/coldforge/coldforge-relay/internal/search"
)

// NewRelay creates and configures a khatru relay with PostgreSQL storage
func NewRelay(cfg *config.Config, db *postgresql.PostgresBackend, searchBackend *search.SearchBackend) *khatru.Relay {
	relay := khatru.NewRelay()

	// Configure relay metadata (NIP-11)
	relay.Info.Name = cfg.RelayName
	relay.Info.Description = "Coldforge Nostr relay - built with khatru"
	relay.Info.PubKey = cfg.RelayPubkey
	relay.Info.Contact = cfg.RelayContact
	relay.Info.SupportedNIPs = []any{1, 9, 11, 13, 22, 33, 40, 42, 45, 46, 50, 77, 86}

	// Enable NIP-77 Negentropy sync
	relay.Negentropy = true

	// Wire up the PostgreSQL backend for event storage
	relay.StoreEvent = append(relay.StoreEvent, db.SaveEvent)
	relay.DeleteEvent = append(relay.DeleteEvent, db.DeleteEvent)
	relay.CountEvents = append(relay.CountEvents, db.CountEvents)

	// Wire up query handlers with NIP-50 search and NIP-40 expiration filtering
	// Single handler that routes to search backend or PostgreSQL backend
	relay.QueryEvents = append(relay.QueryEvents, func(ctx context.Context, filter nostr.Filter) (chan *nostr.Event, error) {
		var sourceCh chan *nostr.Event
		var err error

		// NIP-50: If filter has search term, use search backend
		if searchBackend != nil && search.HasSearch(filter) {
			log.Printf("NIP-50 search query: %q", filter.Search)
			sourceCh, err = searchBackend.QueryEvents(ctx, filter)
		} else {
			// Otherwise, use standard PostgreSQL backend
			sourceCh, err = db.QueryEvents(ctx, filter)
		}

		if err != nil {
			return nil, err
		}

		// NIP-40: Filter out expired events
		resultCh := make(chan *nostr.Event)
		go func() {
			defer close(resultCh)
			for event := range sourceCh {
				// Skip expired events
				if handlers.IsExpired(event) {
					continue
				}
				select {
				case resultCh <- event:
				case <-ctx.Done():
					return
				}
			}
		}()

		return resultCh, nil
	})

	log.Printf("Relay '%s' initialized with PostgreSQL storage", cfg.RelayName)
	log.Println("NIP-77 negentropy sync enabled")
	if searchBackend != nil {
		log.Println("NIP-50 search enabled")
	}

	return relay
}

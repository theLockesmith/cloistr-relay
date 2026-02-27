package relay

import (
	"context"
	"log"

	"github.com/fiatjaf/eventstore/postgresql"
	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"
	"git.coldforge.xyz/coldforge/cloistr-relay/internal/config"
	"git.coldforge.xyz/coldforge/cloistr-relay/internal/eventcache"
	"git.coldforge.xyz/coldforge/cloistr-relay/internal/handlers"
	"git.coldforge.xyz/coldforge/cloistr-relay/internal/search"
	"git.coldforge.xyz/coldforge/cloistr-relay/internal/writeahead"
)

// Version is the relay software version (set at build time or default)
var Version = "0.6.0"

// NewRelay creates and configures a khatru relay with PostgreSQL storage
func NewRelay(cfg *config.Config, db *postgresql.PostgresBackend, searchBackend *search.SearchBackend) *khatru.Relay {
	relay := khatru.NewRelay()

	// Configure relay metadata (NIP-11)
	relay.Info.Name = cfg.RelayName
	relay.Info.Description = "Cloistr Nostr relay - built with khatru"
	relay.Info.PubKey = cfg.RelayPubkey
	relay.Info.Contact = cfg.RelayContact
	relay.Info.SupportedNIPs = []any{1, 9, 11, 13, 22, 33, 40, 42, 45, 46, 50, 57, 59, 66, 70, 77, 86, 94}
	relay.Info.Software = "https://git.coldforge.xyz/coldforge/cloistr-relay"
	relay.Info.Version = Version

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

// NewRelayWithOptions creates a relay with optional event cache and write-ahead log
func NewRelayWithOptions(cfg *config.Config, db *postgresql.PostgresBackend, searchBackend *search.SearchBackend, evtCache *eventcache.Cache, wal *writeahead.WAL) *khatru.Relay {
	relay := khatru.NewRelay()

	// Configure relay metadata (NIP-11)
	relay.Info.Name = cfg.RelayName
	relay.Info.Description = "Cloistr Nostr relay - built with khatru"
	relay.Info.PubKey = cfg.RelayPubkey
	relay.Info.Contact = cfg.RelayContact
	relay.Info.SupportedNIPs = []any{1, 9, 11, 13, 22, 33, 40, 42, 45, 46, 50, 57, 59, 66, 70, 77, 86, 94}
	relay.Info.Software = "https://git.coldforge.xyz/coldforge/cloistr-relay"
	relay.Info.Version = Version

	// Enable NIP-77 Negentropy sync
	relay.Negentropy = true

	// Wire up event storage - use WAL if available, otherwise direct to PostgreSQL
	if wal != nil {
		relay.StoreEvent = append(relay.StoreEvent, wal.CreateSaveEventHandler())
		log.Println("Event storage: Write-ahead log -> PostgreSQL")
	} else {
		relay.StoreEvent = append(relay.StoreEvent, db.SaveEvent)
	}
	relay.DeleteEvent = append(relay.DeleteEvent, db.DeleteEvent)
	relay.CountEvents = append(relay.CountEvents, db.CountEvents)

	// Wire up query handlers with caching, NIP-50 search, and NIP-40 expiration filtering
	relay.QueryEvents = append(relay.QueryEvents, func(ctx context.Context, filter nostr.Filter) (chan *nostr.Event, error) {
		var sourceCh chan *nostr.Event
		var err error

		// Try cache first for simple single-ID lookups
		if evtCache != nil && len(filter.IDs) == 1 && len(filter.Authors) == 0 && len(filter.Kinds) == 0 {
			if event, _ := evtCache.Get(ctx, filter.IDs[0]); event != nil {
				resultCh := make(chan *nostr.Event, 1)
				resultCh <- event
				close(resultCh)
				return resultCh, nil
			}
		}

		// Try cache for profile lookups (single author, kind 0)
		if evtCache != nil && len(filter.Authors) == 1 && len(filter.Kinds) == 1 && filter.Kinds[0] == 0 {
			if event, _ := evtCache.GetProfile(ctx, filter.Authors[0]); event != nil {
				resultCh := make(chan *nostr.Event, 1)
				resultCh <- event
				close(resultCh)
				return resultCh, nil
			}
		}

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

		// NIP-40: Filter out expired events and cache cacheable events
		resultCh := make(chan *nostr.Event)
		go func() {
			defer close(resultCh)
			for event := range sourceCh {
				// Skip expired events
				if handlers.IsExpired(event) {
					continue
				}

				// Cache cacheable events
				if evtCache != nil && eventcache.IsCacheable(event.Kind) {
					_ = evtCache.Set(ctx, event)
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
	if evtCache != nil {
		log.Println("Event cache enabled")
	}
	if wal != nil {
		log.Println("Write-ahead log enabled")
	}

	return relay
}

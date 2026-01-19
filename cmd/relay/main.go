package main

import (
	"fmt"
	"log"
	"net/http"

	"gitlab.com/coldforge/coldforge-relay/internal/auth"
	"gitlab.com/coldforge/coldforge-relay/internal/config"
	"gitlab.com/coldforge/coldforge-relay/internal/handlers"
	"gitlab.com/coldforge/coldforge-relay/internal/relay"
	"gitlab.com/coldforge/coldforge-relay/internal/search"
	"gitlab.com/coldforge/coldforge-relay/internal/storage"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize PostgreSQL storage backend
	db, err := storage.NewPostgresBackend(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Initialize search backend (NIP-50)
	rawDB, err := storage.NewRawConnection(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize search database: %v", err)
	}
	defer rawDB.Close()

	searchBackend := search.NewSearchBackend(rawDB)
	if err := searchBackend.InitSchema(); err != nil {
		log.Printf("Warning: Failed to initialize search index: %v", err)
		// Continue without search
		searchBackend = nil
	}

	// Create the relay
	r := relay.NewRelay(cfg, db, searchBackend)

	// Register custom handlers (validation, filtering)
	handlers.RegisterHandlers(r, cfg)

	// Register NIP-42 authentication handlers
	authCfg := parseAuthConfig(cfg)
	auth.RegisterAuthHandlers(r, authCfg)

	// Start the relay server
	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("Starting Coldforge relay on %s", addr)
	log.Printf("Relay name: %s", cfg.RelayName)

	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("Failed to start relay: %v", err)
	}
}

// parseAuthConfig converts config auth settings to auth.Config
func parseAuthConfig(cfg *config.Config) *auth.Config {
	authCfg := &auth.Config{
		Policy:         auth.PolicyOpen,
		AllowedPubkeys: cfg.AllowedPubkeys,
	}

	switch cfg.AuthPolicy {
	case "auth-read":
		authCfg.Policy = auth.PolicyAuthRead
	case "auth-write":
		authCfg.Policy = auth.PolicyAuthWrite
	case "auth-all":
		authCfg.Policy = auth.PolicyAuthAll
	default:
		// Default to open
		authCfg.Policy = auth.PolicyOpen
	}

	return authCfg
}

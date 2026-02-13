package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/nbd-wtf/go-nostr"

	"gitlab.com/coldforge/coldforge-relay/internal/auth"
	"gitlab.com/coldforge/coldforge-relay/internal/cache"
	"gitlab.com/coldforge/coldforge-relay/internal/config"
	"gitlab.com/coldforge/coldforge-relay/internal/eventcache"
	"gitlab.com/coldforge/coldforge-relay/internal/giftwrap"
	"gitlab.com/coldforge/coldforge-relay/internal/handlers"
	"gitlab.com/coldforge/coldforge-relay/internal/haven"
	"gitlab.com/coldforge/coldforge-relay/internal/management"
	"gitlab.com/coldforge/coldforge-relay/internal/metrics"
	"gitlab.com/coldforge/coldforge-relay/internal/protected"
	"gitlab.com/coldforge/coldforge-relay/internal/ratelimit"
	"gitlab.com/coldforge/coldforge-relay/internal/relay"
	"gitlab.com/coldforge/coldforge-relay/internal/search"
	"gitlab.com/coldforge/coldforge-relay/internal/session"
	"gitlab.com/coldforge/coldforge-relay/internal/storage"
	"gitlab.com/coldforge/coldforge-relay/internal/wot"
	"gitlab.com/coldforge/coldforge-relay/internal/logging"
	"gitlab.com/coldforge/coldforge-relay/internal/middleware"
	"gitlab.com/coldforge/coldforge-relay/internal/nip66"
	"gitlab.com/coldforge/coldforge-relay/internal/tracing"
	"gitlab.com/coldforge/coldforge-relay/internal/writeahead"
	"gitlab.com/coldforge/coldforge-relay/internal/zaps"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize structured logging
	logging.Init(&logging.Config{
		Level:     cfg.LogLevel,
		UseJSON:   cfg.LogFormat == "json",
		Component: "relay",
	})

	// Initialize tracing
	tracing.Init(&tracing.Config{
		ServiceName: "coldforge-relay",
		Enabled:     true,
	})

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

	// Optimize database indexes for common query patterns
	if err := storage.OptimizeIndexes(rawDB); err != nil {
		log.Printf("Warning: Failed to optimize indexes: %v", err)
	}

	// Initialize cache (Dragonfly/Redis)
	var cacheClient *cache.Client
	if cfg.CacheURL != "" {
		cacheCfg := &cache.Config{
			URL:     cfg.CacheURL,
			Enabled: true,
			TTL:     5 * time.Minute,
		}
		var err error
		cacheClient, err = cache.New(cacheCfg)
		if err != nil {
			log.Printf("Warning: Failed to connect to cache: %v", err)
			// Continue without cache
		} else {
			defer cacheClient.Close()
			log.Println("Cache connected (Dragonfly/Redis)")
		}
	}

	// Initialize event cache (if enabled and cache available)
	var evtCache *eventcache.Cache
	if cacheClient != nil && cfg.EventCacheEnabled {
		evtCache = eventcache.New(cacheClient.RedisClient(), eventcache.DefaultConfig())
		log.Println("Hot event cache enabled (Dragonfly/Redis)")
	}

	// Initialize session store (if enabled and cache available)
	// Session store is available for use by handlers that need cross-replica state
	var sessionStore *session.Store
	if cacheClient != nil && cfg.SessionStoreEnabled {
		sessionStore = session.New(cacheClient.RedisClient(), session.DefaultConfig())
		log.Println("Distributed session store enabled (Dragonfly/Redis)")
	}
	_ = sessionStore // Available for future handler integration

	// Initialize write-ahead log (if enabled and cache available)
	var wal *writeahead.WAL
	if cacheClient != nil && cfg.WriteAheadEnabled {
		wal = writeahead.New(cacheClient.RedisClient(), db, writeahead.DefaultConfig())
		wal.Start()
		defer wal.Stop()
		log.Println("Write-ahead logging enabled (Dragonfly/Redis)")
	}

	// Create the relay (with optional event cache and write-ahead log)
	r := relay.NewRelayWithOptions(cfg, db, searchBackend, evtCache, wal)

	// Register custom handlers (validation, filtering)
	// Pass whether distributed rate limiting is active so in-memory rate limiting can be skipped
	useDistributedRateLimit := cacheClient != nil && cfg.RateLimitDistributed
	handlers.RegisterHandlers(r, cfg, useDistributedRateLimit)

	// Register distributed rate limiting (if enabled)
	if useDistributedRateLimit {
		rateLimitCfg := &ratelimit.Config{
			Enabled:              true,
			EventsPerSecond:      cfg.RateLimitEventsPerSec,
			FiltersPerSecond:     cfg.RateLimitFiltersPerSec,
			ConnectionsPerSecond: cfg.RateLimitConnectionsPerSec,
			BurstMultiplier:      5,
			WindowSize:           time.Second,
		}
		ratelimit.RegisterHandlers(r, cacheClient.RedisClient(), rateLimitCfg)
		log.Println("Distributed rate limiting enabled (Dragonfly/Redis)")
	}

	// Initialize NIP-86 management API
	var mgmtStore *management.Store
	if len(cfg.AdminPubkeys) > 0 {
		mgmtStore = management.NewStore(rawDB)
		if err := mgmtStore.Init(); err != nil {
			log.Fatalf("Failed to initialize management store: %v", err)
		}
		// Register ban checking handlers
		management.RegisterBanHandlers(r, mgmtStore)
		log.Printf("NIP-86 management API enabled for %d admin pubkeys", len(cfg.AdminPubkeys))
	}

	// Initialize WoT filtering (if enabled)
	if cfg.WoTEnabled && cfg.WoTOwnerPubkey != "" {
		wotStore := wot.NewStore(rawDB, 5*time.Minute)
		if err := wotStore.Init(); err != nil {
			log.Fatalf("Failed to initialize WoT store: %v", err)
		}

		// Connect external cache to WoT store
		if cacheClient != nil {
			wotStore.SetExternalCache(cacheClient)
			log.Println("WoT using external cache (Dragonfly/Redis)")
		}

		// Build WoT config
		wotCfg := &wot.Config{
			Enabled:        true,
			OwnerPubkey:    cfg.WoTOwnerPubkey,
			Policies:       wot.DefaultPolicies(),
			CacheTTL:       5 * time.Minute,
			MaxFollowDepth: 2,
			UsePageRank:    cfg.WoTUsePageRank,
		}

		// Set PageRank interval (default 60 minutes)
		if cfg.WoTPageRankInterval > 0 {
			wotCfg.PageRankInterval = time.Duration(cfg.WoTPageRankInterval) * time.Minute
		} else {
			wotCfg.PageRankInterval = 60 * time.Minute
		}

		// Apply custom policy overrides if configured
		if cfg.WoTUnknownPoWBits > 0 {
			policy := wotCfg.Policies[wot.TrustLevelUnknown]
			policy.MinPoWDifficulty = cfg.WoTUnknownPoWBits
			wotCfg.Policies[wot.TrustLevelUnknown] = policy
		}
		if cfg.WoTUnknownRateLimit > 0 {
			policy := wotCfg.Policies[wot.TrustLevelUnknown]
			policy.EventsPerSecond = cfg.WoTUnknownRateLimit
			wotCfg.Policies[wot.TrustLevelUnknown] = policy
		}

		wot.RegisterHandlers(r, wotStore, wotCfg)
	}

	// Initialize NIP-59 Gift Wrap (if enabled)
	if cfg.GiftWrapEnabled {
		gwCfg := &giftwrap.Config{
			Enabled:                true,
			RequireAuthForGiftWrap: cfg.GiftWrapRequireAuth,
		}
		giftwrap.RegisterHandlers(r, gwCfg)
	}

	// Initialize NIP-57 Zaps (if enabled)
	if cfg.ZapsEnabled {
		zapsCfg := &zaps.Config{
			Enabled:          true,
			ValidateReceipts: cfg.ZapsValidateReceipt,
			RequireBolt11:    false,
		}
		zaps.RegisterHandlers(r, zapsCfg)
	}

	// Initialize NIP-70 Protected Events (if enabled)
	if cfg.ProtectedEventsEnabled {
		protectedCfg := &protected.Config{
			Enabled:              true,
			AllowProtectedEvents: cfg.ProtectedEventsAllow,
		}
		protected.RegisterHandlers(r, protectedCfg)
	}

	// Initialize NIP-66 Relay Discovery (if enabled)
	var selfMonitor *nip66.SelfMonitor
	if cfg.NIP66Enabled {
		nip66Cfg := &nip66.Config{
			Enabled: true,
		}
		nip66.RegisterHandlers(r, nip66Cfg)
		log.Println("NIP-66 relay discovery enabled")

		// Start self-monitor if configured
		if cfg.NIP66SelfMonitor && cfg.NIP66MonitorKey != "" {
			monitorCfg := &nip66.MonitorConfig{
				RelayURL:   cfg.RelayURL,
				MonitorKey: cfg.NIP66MonitorKey,
				Interval:   5 * time.Minute,
				PublishFunc: func(ctx context.Context, event *nostr.Event) error {
					return db.SaveEvent(ctx, event)
				},
			}
			var err error
			selfMonitor, err = nip66.NewSelfMonitor(monitorCfg)
			if err != nil {
				log.Printf("Warning: Failed to initialize NIP-66 self-monitor: %v", err)
			} else {
				selfMonitor.Start()
				defer selfMonitor.Stop()
				log.Println("NIP-66 self-monitor started")
			}
		}
	}

	// Initialize HAVEN-style box routing (if enabled)
	var havenSystem *haven.HavenSystem
	if cfg.HavenEnabled && cfg.HavenOwnerPubkey != "" {
		havenCfg := &haven.Config{
			Enabled:               true,
			OwnerPubkey:           cfg.HavenOwnerPubkey,
			PrivateKinds:          cfg.HavenPrivateKinds,
			AllowPublicOutboxRead: cfg.HavenAllowPublicOutboxRead,
			AllowPublicInboxWrite: cfg.HavenAllowPublicInboxWrite,
			RequireAuthForChat:    cfg.HavenRequireAuthForChat,
			RequireAuthForPrivate: cfg.HavenRequireAuthForPrivate,
			BlastrEnabled:         cfg.HavenBlastrEnabled,
			BlastrRelays:          cfg.HavenBlastrRelays,
			ImporterEnabled:       cfg.HavenImporterEnabled,
			ImporterRelays:        cfg.HavenImporterRelays,
		}
		// Use RegisterFullSystem to enable Blastr and Importer
		havenSystem = haven.RegisterFullSystem(r, havenCfg, db.SaveEvent)
		if havenSystem != nil {
			defer havenSystem.Stop()
		}
	}

	// Register NIP-42 authentication handlers
	authCfg := parseAuthConfig(cfg)
	auth.RegisterAuthHandlers(r, authCfg)

	// Register Prometheus metrics
	metrics.RegisterRelayMetrics(r)
	metrics.RegisterDBPoolMetrics(rawDB, 15*time.Second)
	log.Println("Prometheus metrics enabled at /metrics")

	// Register observability (structured logging + tracing)
	middleware.RegisterObservability(r)
	log.Println("Structured logging and tracing enabled")

	// Create HTTP mux to serve both relay and metrics
	mux := http.NewServeMux()
	mux.Handle("/", r)
	mux.Handle("/metrics", metrics.Handler())

	// Health check endpoint (simple, for Kubernetes probes)
	mux.HandleFunc("/health", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// NIP-86 management API endpoint
	if mgmtStore != nil {
		mgmtHandler := management.NewHandler(mgmtStore, cfg.AdminPubkeys)
		mux.Handle("/management", mgmtHandler)
		log.Println("NIP-86 management API enabled at /management")
	}

	// Profiling endpoints (for performance debugging)
	if cfg.PProfEnabled {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
		mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
		mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
		mux.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
		mux.Handle("/debug/pprof/block", pprof.Handler("block"))
		mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
		log.Println("pprof profiling enabled at /debug/pprof/")
	}

	// Start the relay server
	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("Starting Coldforge relay on %s", addr)
	log.Printf("Relay name: %s", cfg.RelayName)

	if err := http.ListenAndServe(addr, mux); err != nil {
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

package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/http/pprof"
	"strings"
	"time"

	"github.com/fiatjaf/eventstore/postgresql"
	"github.com/fiatjaf/khatru"
	"github.com/fiatjaf/relay29"
	"github.com/nbd-wtf/go-nostr"

	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/admin"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/algo"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/auth"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/feeds"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/cache"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/config"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/eventcache"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/giftwrap"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/handlers"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/haven"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/management"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/membership"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/metrics"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/protected"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/ratelimit"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/relay"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/search"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/session"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/storage"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/wot"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/logging"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/middleware"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/nip66"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/pubsub"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/tracing"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/writeahead"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/zaps"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/web"
)

// relay29Wrapper adapts khatru.Relay to relay29's expected interface
// relay29 expects BroadcastEvent to return nothing, but khatru returns int
type relay29Wrapper struct {
	*khatru.Relay
}

func (w *relay29Wrapper) BroadcastEvent(evt *nostr.Event) {
	w.Relay.BroadcastEvent(evt) // ignore return value
}

func (w *relay29Wrapper) AddEvent(ctx context.Context, evt *nostr.Event) (skipBroadcast bool, writeError error) {
	return w.Relay.AddEvent(ctx, evt)
}

// eventLookupAdapter implements haven.EventLookup using the PostgreSQL backend
type eventLookupAdapter struct {
	db *postgresql.PostgresBackend
}

// GetEventByID looks up an event by its ID
func (a *eventLookupAdapter) GetEventByID(ctx context.Context, id string) (*nostr.Event, error) {
	filter := nostr.Filter{
		IDs:   []string{id},
		Limit: 1,
	}

	eventCh, err := a.db.QueryEvents(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Get the first (and only) event from the channel
	for event := range eventCh {
		return event, nil
	}

	return nil, nil // Event not found
}

// wotUserFilterAdapter adapts wot.UserFilter to haven.WoTUserFilter
type wotUserFilterAdapter struct {
	filter *wot.UserFilter
}

// ShouldAllowToInbox implements haven.WoTUserFilter
func (a *wotUserFilterAdapter) ShouldAllowToInbox(ctx context.Context, event *nostr.Event, recipientPubkey string) haven.WoTFilterResult {
	result := a.filter.ShouldAllowToInbox(ctx, event, recipientPubkey)
	return haven.WoTFilterResult{
		Allowed: result.Allowed,
		Reason:  result.Reason,
		Source:  string(result.Source),
	}
}

// tierCounterAdapter adapts membership.Store to haven.TierCounter
type tierCounterAdapter struct {
	store *membership.Store
}

// CountMembersByTier implements haven.TierCounter
func (a *tierCounterAdapter) CountMembersByTier() (map[string]int, error) {
	ctx := context.Background()
	counts, err := a.store.CountMembersByTier(ctx)
	if err != nil {
		return nil, err
	}
	result := make(map[string]int)
	for tier, count := range counts {
		result[string(tier)] = count
	}
	return result, nil
}

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
		ServiceName: "cloistr-relay",
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
	defer func() { _ = rawDB.Close() }()

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
			defer func() { _ = cacheClient.Close() }()
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

	// Initialize cross-pod event pub/sub (if enabled and cache available)
	var eventPubSub *pubsub.PubSub
	if cacheClient != nil && cfg.EventPubSubEnabled {
		pubsubCfg := &pubsub.Config{
			Enabled: true,
		}
		eventPubSub = pubsub.New(cacheClient.RedisClient(), r, pubsubCfg)
		// Register store hook to publish stored events to other pods
		r.StoreEvent = append(r.StoreEvent, eventPubSub.CreateStoreEventHook())
		// Register ephemeral hook to publish NIP-46 and other ephemeral events to other pods
		r.OnEphemeralEvent = append(r.OnEphemeralEvent, eventPubSub.CreateEphemeralEventHook())
		// Start subscription to receive events from other pods
		eventPubSub.Start()
		defer eventPubSub.Stop()
		log.Println("Cross-pod event pub/sub enabled (Dragonfly/Redis)")
	}

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
	var wotHandler *wot.Handler
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
			AllowedPubkeys: cfg.AllowedPubkeys, // Bypass WoT for whitelisted pubkeys
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

		wotHandler = wot.RegisterHandlers(r, wotStore, wotCfg)
	}

	// Initialize Algorithmic Feeds (if enabled)
	var algoHandler *algo.Handler
	if cfg.AlgoEnabled {
		algoCfg := &algo.Config{
			Enabled:          true,
			DefaultAlgorithm: algo.ParseAlgorithm(cfg.AlgoDefault),
			WoTWeight:        cfg.AlgoWoTWeight,
			EngagementWeight: cfg.AlgoEngagementWeight,
			RecencyWeight:    cfg.AlgoRecencyWeight,
		}
		algoHandler = algo.RegisterAlgorithmSupport(r, algoCfg, rawDB, wotHandler)
	}
	_ = algoHandler // Available for feed handler integration

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

	// Initialize HAVEN-style box routing
	// Two modes: single-owner (legacy) or multi-user (per-user boxes)
	var havenSystem *haven.HavenSystem
	var havenMultiSystem *haven.MultiUserSystem
	var havenCfg *haven.Config

	if cfg.HavenMultiUserEnabled {
		// Multi-user mode: per-user HAVEN boxes with shared worker pools
		log.Println("HAVEN: initializing multi-user mode")

		// Initialize membership store (required for tier lookups)
		memberStore := membership.NewStore(rawDB)
		if err := memberStore.InitSchema(context.Background()); err != nil {
			log.Fatalf("Failed to initialize membership store: %v", err)
		}

		// Initialize HAVEN user settings store
		havenUserSettings := haven.NewUserSettingsStore(rawDB)
		if err := havenUserSettings.Init(context.Background()); err != nil {
			log.Fatalf("Failed to initialize HAVEN user settings: %v", err)
		}

		// Initialize WoT user settings store
		wotUserSettings := wot.NewUserSettingsStore(rawDB)
		if err := wotUserSettings.Init(context.Background()); err != nil {
			log.Fatalf("Failed to initialize WoT user settings: %v", err)
		}

		// Initialize B2B organization store
		orgStore := haven.NewOrgStore(rawDB)
		if err := orgStore.Init(context.Background()); err != nil {
			log.Fatalf("Failed to initialize organization store: %v", err)
		}
		log.Println("B2B organization store enabled")

		// Initialize BlastrManager with shared worker pool
		blastrCfg := haven.DefaultBlastrManagerConfig()
		blastrManager := haven.NewBlastrManager(blastrCfg, memberStore, havenUserSettings)
		blastrManager.Start()
		defer blastrManager.Stop()
		log.Printf("HAVEN BlastrManager: %d workers for per-user broadcasting", blastrCfg.WorkerCount)

		// Initialize ImporterManager with scheduler and shared worker pool
		importerCfg := haven.DefaultImporterManagerConfig()
		importerManager := haven.NewImporterManager(importerCfg, memberStore, havenUserSettings)
		importerManager.SetStoreFunc(func(ctx context.Context, event *nostr.Event, userPubkey string) error {
			// Store event in user's inbox (uses standard save path)
			return db.SaveEvent(ctx, event)
		})
		importerManager.Start()
		defer importerManager.Stop()
		log.Printf("HAVEN ImporterManager: %d workers, polling every %v", importerCfg.WorkerCount, importerCfg.PollInterval)

		// Register HAVEN settings watcher (NIP-78)
		havenSettingsWatcher := haven.NewHavenSettingsWatcher(havenUserSettings)
		r.OnEventSaved = append(r.OnEventSaved, havenSettingsWatcher.OnEventSaved())
		log.Println("HAVEN settings watcher enabled (NIP-78)")

		// Register WoT settings watcher (NIP-78)
		wotSettingsWatcher := wot.NewSettingsWatcher(wotUserSettings)
		r.OnEventSaved = append(r.OnEventSaved, wotSettingsWatcher.OnEventSaved())
		log.Println("WoT settings watcher enabled (NIP-78)")

		// Build handler config for multi-user mode
		havenCfg = &haven.Config{
			Enabled:               true,
			OwnerPubkey:           "", // No single owner in multi-user mode
			PrivateKinds:          cfg.HavenPrivateKinds,
			AllowPublicOutboxRead: cfg.HavenAllowPublicOutboxRead,
			AllowPublicInboxWrite: cfg.HavenAllowPublicInboxWrite,
			RequireAuthForChat:    cfg.HavenRequireAuthForChat,
			RequireAuthForPrivate: cfg.HavenRequireAuthForPrivate,
		}

		// Create per-user WoT filter (uses the wotUserSettings store)
		// Wrapped in adapter to convert wot.FilterResult to haven.WoTFilterResult
		wotUserFilter := &wotUserFilterAdapter{
			filter: wot.NewUserFilter(wotUserSettings, nil), // nil for relay handler - relay WoT runs separately
		}

		multiHandlerCfg := &haven.MultiUserHandlerConfig{
			MemberStore:       memberStore,
			UserSettingsStore: havenUserSettings,
			BlastrManager:     blastrManager,
			ImporterManager:   importerManager,
			WoTUserFilter:     wotUserFilter,
		}

		// Register per-user HAVEN handlers
		havenMultiSystem = haven.RegisterMultiUserHandlers(r, havenCfg, multiHandlerCfg)
		if havenMultiSystem != nil {
			defer havenMultiSystem.Stop()
			// Set up E-tag routing for reactions/reposts
			havenMultiSystem.SetEventLookup(&eventLookupAdapter{db: db})
			log.Println("HAVEN[multi] E-tag routing enabled for reactions/reposts")
		}

		// Start metrics collector for tier distribution
		metricsCollector := haven.NewMetricsCollector(&tierCounterAdapter{store: memberStore}, 60*time.Second)
		metricsCollector.Start()
		defer metricsCollector.Stop()
		log.Println("HAVEN metrics collector started (tier distribution every 60s)")

		log.Println("Per-user HAVEN enabled (multi-tenant mode)")

	} else if cfg.HavenEnabled && cfg.HavenOwnerPubkey != "" {
		// Single-owner mode: legacy HAVEN routing
		havenCfg = &haven.Config{
			Enabled:               true,
			OwnerPubkey:           cfg.HavenOwnerPubkey,
			PrivateKinds:          cfg.HavenPrivateKinds,
			AllowPublicOutboxRead: cfg.HavenAllowPublicOutboxRead,
			AllowPublicInboxWrite: cfg.HavenAllowPublicInboxWrite,
			RequireAuthForChat:    cfg.HavenRequireAuthForChat,
			RequireAuthForPrivate: cfg.HavenRequireAuthForPrivate,
			BlastrEnabled:         cfg.HavenBlastrEnabled,
			BlastrRelays:          cfg.HavenBlastrRelays,
			BlastrRetryEnabled:    cfg.HavenBlastrRetryEnabled && cacheClient != nil,
			BlastrMaxRetries:      cfg.HavenBlastrMaxRetries,
			BlastrRetryInterval:   cfg.HavenBlastrRetryInterval,
			ImporterEnabled:         cfg.HavenImporterEnabled,
			ImporterRelays:          cfg.HavenImporterRelays,
			ImporterRealtimeEnabled: cfg.HavenImporterRealtimeEnabled,
		}
		// Use RegisterFullSystem to enable Blastr and Importer
		havenSystem = haven.RegisterFullSystem(r, havenCfg, db.SaveEvent)
		if havenSystem != nil {
			defer havenSystem.Stop()
			// Set up E-tag routing for reactions/reposts
			havenSystem.SetEventLookup(&eventLookupAdapter{db: db})
			log.Println("HAVEN E-tag routing enabled for reactions/reposts")

			// Set up Blastr retry queue if cache is available
			if havenSystem.Blastr != nil && cacheClient != nil && cfg.HavenBlastrRetryEnabled {
				havenSystem.Blastr.SetRedisClient(cacheClient.RedisClient())
				log.Println("HAVEN Blastr retry queue enabled")
			}
		}
	}

	// Suppress unused variable warning for single-owner system (used in defer)
	_ = havenSystem

	// Initialize NIP-29 relay-based groups using relay29 (if enabled)
	if cfg.GroupsEnabled {
		if cfg.GroupsSecretKey == "" {
			log.Printf("Warning: NIP-29 groups enabled but GROUPS_SECRET_KEY not set - groups will not work properly")
		} else {
			// Initialize relay29 state
			groupsState := relay29.New(relay29.Options{
				Domain:    strings.TrimPrefix(strings.TrimPrefix(cfg.GroupsRelayURL, "wss://"), "ws://"),
				DB:        db,
				SecretKey: cfg.GroupsSecretKey,
			})

			// Configure state options
			groupsState.AllowPrivateGroups = cfg.GroupsAllowPrivate

			// Attach relay29 state to our existing relay (using wrapper for interface compatibility)
			groupsState.Relay = &relay29Wrapper{r}
			groupsState.GetAuthed = khatru.GetAuthed

			// Add relay29 handlers to the relay
			// Note: We already have StoreEvent, QueryEvents, DeleteEvent from relay.NewRelayWithOptions
			// Add relay29's query handlers for group-specific queries
			r.QueryEvents = append(r.QueryEvents,
				groupsState.NormalEventQuery,
				groupsState.MetadataQueryHandler,
				groupsState.AdminsQueryHandler,
				groupsState.MembersQueryHandler,
				groupsState.RolesQueryHandler,
			)
			r.RejectFilter = append(r.RejectFilter,
				groupsState.RequireKindAndSingleGroupIDOrSpecificEventReference,
			)
			r.RejectEvent = append(r.RejectEvent,
				groupsState.RequireHTagForExistingGroup,
				groupsState.RequireModerationEventsToBeRecent,
				groupsState.RestrictWritesBasedOnGroupRules,
				groupsState.RestrictInvalidModerationActions,
				groupsState.PreventWritingOfEventsJustDeleted,
				groupsState.CheckPreviousTag,
			)
			r.OnEventSaved = append(r.OnEventSaved,
				groupsState.ApplyModerationAction,
				groupsState.ReactToJoinRequest,
				groupsState.ReactToLeaveRequest,
				groupsState.AddToPreviousChecking,
			)

			// Derive public key for logging
			pubkey, _ := nostr.GetPublicKey(cfg.GroupsSecretKey)
			log.Printf("NIP-29 relay-based groups enabled (relay29, pubkey: %s...)", pubkey[:8])
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
		_, _ = w.Write([]byte("OK"))
	})

	// Serve favicon from embedded static files
	staticFS, err := fs.Sub(web.Static, "static")
	if err == nil {
		mux.Handle("/favicon.ico", http.FileServer(http.FS(staticFS)))
		mux.Handle("/favicon.svg", http.FileServer(http.FS(staticFS)))
		mux.Handle("/apple-touch-icon.png", http.FileServer(http.FS(staticFS)))
	}

	// RSS/Atom feed endpoints (if enabled)
	if cfg.FeedEnabled {
		// Determine owner pubkey - prefer HAVEN owner, fall back to relay pubkey
		ownerPubkey := cfg.HavenOwnerPubkey
		if ownerPubkey == "" {
			ownerPubkey = cfg.RelayPubkey
		}

		if ownerPubkey != "" {
			feedCfg := &feeds.Config{
				Enabled:          true,
				OwnerPubkey:      ownerPubkey,
				RelayURL:         cfg.RelayURL,
				RelayName:        cfg.RelayName,
				DefaultLimit:     cfg.FeedLimit,
				MaxLimit:         cfg.FeedMaxLimit,
				IncludeLongForm:  cfg.FeedIncludeLongForm,
				IncludeReplies:   cfg.FeedIncludeReplies,
				DefaultAlgorithm: cfg.AlgoDefault,
			}
			feedHandler := feeds.NewHandler(feedCfg, db)
			// Connect algorithm handler if enabled
			if algoHandler != nil {
				feedHandler.SetAlgoHandler(algoHandler)
				log.Println("RSS/Atom feeds with algorithmic sorting enabled")
			}
			feedHandler.RegisterRoutes(mux)
			log.Printf("RSS/Atom feeds enabled at /feed/rss and /feed/atom")
		} else {
			log.Println("RSS/Atom feeds disabled: no owner pubkey configured")
		}
	}

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

	// Create admin UI mux (served on relay-admin.* hostnames)
	var adminMux *http.ServeMux
	if mgmtStore != nil && len(cfg.AdminPubkeys) > 0 {
		// Enable distributed admin sessions if cache is available
		if cacheClient != nil {
			admin.SetRedisClient(cacheClient.RedisClient())
		}
		adminMux = http.NewServeMux()
		adminHandler := admin.NewHandler(mgmtStore, cfg.AdminPubkeys)
		// Inject HAVEN system for admin UI stats display
		adminHandler.SetHavenSystem(havenSystem, havenCfg)
		adminHandler.RegisterRoutes(adminMux)
		log.Println("Admin UI enabled for relay-admin.* hostnames")
	}

	// Host-based router: relay-admin.* -> admin UI, everything else -> relay
	router := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		// Strip port if present
		if idx := strings.Index(host, ":"); idx != -1 {
			host = host[:idx]
		}

		// Route admin hostnames to admin UI
		if strings.HasPrefix(host, "relay-admin.") && adminMux != nil {
			adminMux.ServeHTTP(w, r)
			return
		}

		// Everything else goes to relay
		mux.ServeHTTP(w, r)
	})

	// Start the relay server
	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("Starting Cloistr relay on %s", addr)
	log.Printf("Relay name: %s", cfg.RelayName)

	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("Failed to start relay: %v", err)
	}
}

// parseAuthConfig converts config auth settings to auth.Config
func parseAuthConfig(cfg *config.Config) *auth.Config {
	authCfg := &auth.Config{
		Policy:         auth.PolicyOpen,
		AllowedPubkeys: cfg.AllowedPubkeys,
		ExemptKinds:    cfg.AuthExemptKinds,
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

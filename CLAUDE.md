# CLAUDE.md - coldforge-relay

**Custom Nostr relay built with khatru (Go)**

**Status:** Working - 30 NIPs implemented (1, 9, 11, 13, 17, 22, 29, 32, 33, 40, 42, 43, 45, 46, 50, 51, 52, 57, 59, 65, 66, 70, 72, 73, 77, 85, 86, 88, 94) + WoT Filtering + HAVEN Box Routing + Admin UI + RSS/Atom Feeds + Algorithmic Feeds

**Domain:** relay.cloistr.xyz (Cloistr is the consumer-facing brand for Coldforge Nostr services)
**Admin UI:** relay-admin.cloistr.xyz (integrated via host-based routing)

## REQUIRED READING (Before ANY Action)

**Claude MUST read this file at the start of every session:**
- `~/claude/coldforge/cloistr/CLAUDE.md` - Cloistr project rules (contains further required reading)

## Documentation

Full documentation: `~/claude/coldforge/services/relay/CLAUDE.md`
Coldforge overview: `~/claude/coldforge/CLAUDE.md`

## Autonomous Work Mode (CRITICAL)

**Work autonomously. Do NOT stop to ask what to do next.**

- Keep working until the task is complete or you hit a genuine blocker
- Use the "Next Steps" section in the service docs to know what to work on
- Make reasonable decisions - don't ask for permission on obvious choices
- Only stop to ask if there's a true ambiguity that affects architecture
- If tests fail, fix them. If code needs review, use the reviewer agent. Keep going.
- Update this CLAUDE.md and the service docs as you make progress

## Agent Usage (IMPORTANT)

**Use agents proactively. Do not wait for explicit instructions.**

| When... | Use agent... |
|---------|-------------|
| Starting new work or need context | `explore` |
| Need to research NIPs or protocols | `explore` |
| Writing or modifying code | `reviewer` after significant changes |
| Writing tests | `test-writer` |
| Running tests | `tester` |
| Investigating bugs | `debugger` |
| Updating documentation | `documenter` |
| Creating Dockerfiles | `docker` |
| Setting up Kubernetes deployment | `atlas-deploy` |
| Security-sensitive code (auth, crypto) | `security` |

## Workflow

1. **Before coding:** Use `explore` to read the service documentation and understand requirements
2. **While coding:** Write code, then use `reviewer` to check it
3. **Testing:** Use `test-writer` to create tests, `tester` to run them
4. **Before committing:** Use `security` for auth/crypto code
5. **Deployment:** Use `docker` for containers, `atlas-deploy` for Kubernetes

## Quick Commands

```bash
# Run locally
docker compose up -d

# Run tests
docker build --target test -t coldforge-relay:test-runner .
docker run --rm coldforge-relay:test-runner

# Build only
docker compose build relay

# Check relay info (NIP-11)
curl -H "Accept: application/nostr+json" http://localhost:3334/

# View logs
docker compose logs -f relay
```

## Project Structure

```
├── cmd/
│   └── relay/          # Main relay entry point (includes host-based routing for admin UI)
├── internal/
│   ├── admin/          # Admin UI handlers (htmx + NIP-98 auth) - integrated into relay via host routing
│   ├── algo/           # Algorithmic feed scoring engine (opt-in, WoT-ranked, engagement, trending)
│   ├── auth/           # NIP-42 authentication
│   ├── cache/          # Redis/Dragonfly client wrapper
│   ├── calendar/       # NIP-52 calendar events and RSVPs
│   ├── config/         # Configuration loading
│   ├── eventcache/     # Hot event caching (Dragonfly)
│   ├── external/       # NIP-73 external content identifiers (ISBN, DOI, IMDB)
│   ├── feeds/          # RSS/Atom feed generation (Nostr-to-RSS bridge)
│   ├── giftwrap/       # NIP-59 gift wrap handling
│   ├── groups/         # NIP-29 relay-based groups (kinds 9000-9022, 39000-39003)
│   ├── handlers/       # Event validation, NIP-40/22/13
│   ├── haven/          # HAVEN-style box routing (inbox/outbox/chat/private)
│   ├── labels/         # NIP-32 content labeling (moderation, classification)
│   ├── lists/          # NIP-51 user lists (mutes, bookmarks, relay sets)
│   ├── logging/        # Structured JSON logging
│   ├── membership/     # NIP-43 relay access and membership management
│   ├── communities/    # NIP-72 moderated public communities
│   ├── management/     # NIP-86 relay management API + store
│   ├── middleware/     # Observability middleware (logging + tracing)
│   ├── nip66/          # NIP-66 relay discovery and self-monitoring
│   ├── polls/          # NIP-88 community polls and responses
│   ├── protected/      # NIP-70 protected events handling
│   ├── ratelimit/      # Distributed rate limiting (Dragonfly)
│   ├── relay/          # Khatru relay setup
│   ├── search/         # NIP-50 PostgreSQL full-text search
│   ├── session/        # Distributed session state (Dragonfly)
│   ├── storage/        # PostgreSQL backend
│   ├── tracing/        # Distributed tracing with spans
│   ├── trust/          # NIP-85 trusted assertion providers
│   ├── wot/            # Web of Trust filtering
│   ├── writeahead/     # Write-ahead log (Dragonfly)
│   └── zaps/           # NIP-57 Lightning zaps
├── dashboards/         # Grafana dashboard JSON files
├── web/
│   ├── templates/      # HTML templates (layout + 11 pages + 10 partials)
│   └── static/js/      # NIP-07/NIP-98 auth helper
├── tests/              # Test documentation
├── Dockerfile          # Relay multi-stage build (includes admin UI)
└── docker-compose.yml  # Local development
```

## Configuration

Set via environment variables:
- `RELAY_PORT` - Port (default: 3334)
- `RELAY_NAME` - Relay name
- `AUTH_POLICY` - "open", "auth-read", "auth-write", "auth-all"
- `ALLOWED_PUBKEYS` - Comma-separated whitelist
- `DB_HOST/PORT/NAME/USER/PASSWORD` - PostgreSQL connection
- `MAX_CREATED_AT_FUTURE` - NIP-22: max seconds into future (default: 300)
- `MAX_CREATED_AT_PAST` - NIP-22: max seconds into past (0 = unlimited)
- `MIN_POW_DIFFICULTY` - NIP-13: required PoW difficulty (0 = disabled)
- `RATE_LIMIT_EVENTS_PER_SEC` - Events per second per IP (default: 10)
- `RATE_LIMIT_FILTERS_PER_SEC` - Queries per second per IP (default: 20)
- `RATE_LIMIT_CONNECTIONS_PER_SEC` - Connections per second per IP (default: 5)
- `RATE_LIMIT_EXEMPT_KINDS` - Comma-separated kinds exempt from event rate limiting (e.g., 24133 for NIP-46)
- `RATE_LIMIT_EXEMPT_PUBKEYS` - Comma-separated hex pubkeys exempt from all rate limiting
- `ADMIN_PUBKEYS` - NIP-86: Comma-separated pubkeys for management API access
- `WOT_ENABLED` - Enable Web of Trust filtering (true/1)
- `WOT_OWNER_PUBKEY` - Relay owner pubkey (trust level 0)
- `WOT_UNKNOWN_POW_BITS` - PoW bits required for unknown pubkeys (default: 8)
- `WOT_UNKNOWN_RATE_LIMIT` - Events/sec for unknown pubkeys (default: 5)
- `WOT_USE_PAGERANK` - Use PageRank-based trust (Tier 2, requires cache)
- `WOT_PAGERANK_INTERVAL` - PageRank recompute interval in minutes (default: 60)
- `CACHE_URL` - Redis/Dragonfly URL (e.g., redis://dragonfly:6379)
- `RATE_LIMIT_DISTRIBUTED` - Use Dragonfly for distributed rate limiting (true/1)
- `WRITE_AHEAD_ENABLED` - Enable write-ahead log via Dragonfly (true/1)
- `EVENT_CACHE_ENABLED` - Enable hot event caching via Dragonfly (true/1)
- `SESSION_STORE_ENABLED` - Enable distributed session state via Dragonfly (true/1)
- `GIFTWRAP_ENABLED` - NIP-59: Enable gift wrap support (default: true)
- `GIFTWRAP_REQUIRE_AUTH` - NIP-59: Require auth to query gift wraps (default: true)
- `ZAPS_ENABLED` - NIP-57: Enable zap receipt support (default: true)
- `ZAPS_VALIDATE_RECEIPT` - NIP-57: Validate zap receipt structure (default: true)
- `PROTECTED_EVENTS_ENABLED` - NIP-70: Enable protected event handling (default: true)
- `PROTECTED_EVENTS_ALLOW` - NIP-70: Allow protected events from authenticated authors (default: true)

### Database Connection Pool Tuning
- `DB_MAX_OPEN_CONNS` - Max open connections (default: 25)
- `DB_MAX_IDLE_CONNS` - Max idle connections (default: 10)
- `DB_CONN_MAX_LIFETIME` - Max connection lifetime (default: 5m)
- `DB_CONN_MAX_IDLE_TIME` - Max idle time before closing (default: 1m)

### Profiling
- `PPROF_ENABLED` - Enable pprof endpoints at /debug/pprof/ (default: false)

### Logging
- `LOG_LEVEL` - Log level: debug, info, warn, error (default: info)
- `LOG_FORMAT` - Log format: json, text (default: json for production)

### NIP-66 Relay Monitoring
- `NIP66_ENABLED` - Enable NIP-66 relay discovery support (default: false)
- `NIP66_SELF_MONITOR` - Enable self-monitoring (publish own health events, default: false)
- `NIP66_MONITOR_KEY` - Private key for signing monitor events

### HAVEN Box Routing
- `HAVEN_ENABLED` - Enable HAVEN-style box routing (default: false)
- `HAVEN_OWNER_PUBKEY` - Owner pubkey for box routing (required if enabled)
- `HAVEN_PRIVATE_KINDS` - Additional kinds for private box (comma-separated)
- `HAVEN_ALLOW_PUBLIC_OUTBOX_READ` - Public can read owner's posts (default: true)
- `HAVEN_ALLOW_PUBLIC_INBOX_WRITE` - Anyone can tag/mention owner (default: true)
- `HAVEN_REQUIRE_AUTH_FOR_CHAT` - DMs require authentication (default: true)
- `HAVEN_REQUIRE_AUTH_FOR_PRIVATE` - Private box requires authentication (default: true)
- `HAVEN_BLASTR_ENABLED` - Enable outbox broadcasting (default: false)
- `HAVEN_BLASTR_RELAYS` - Relays to broadcast outbox events to (comma-separated)
- `HAVEN_BLASTR_RETRY_ENABLED` - Enable persistent retry queue (requires Redis/Dragonfly)
- `HAVEN_BLASTR_MAX_RETRIES` - Maximum retry attempts per event/relay (default: 6)
- `HAVEN_BLASTR_RETRY_INTERVAL` - Retry worker interval in seconds (default: 30)
- `HAVEN_IMPORTER_ENABLED` - Enable inbox event fetching (default: false)
- `HAVEN_IMPORTER_RELAYS` - Relays to fetch inbox events from (comma-separated)
- `HAVEN_IMPORTER_REALTIME` - Enable real-time WebSocket subscriptions (default: false, uses polling)

### RSS/Atom Feeds
- `FEED_ENABLED` - Enable RSS/Atom feeds (default: auto-enabled when HAVEN is enabled)
- `FEED_LIMIT` - Default number of items in feeds (default: 20)
- `FEED_MAX_LIMIT` - Maximum items allowed via ?limit= parameter (default: 100)
- `FEED_INCLUDE_LONG_FORM` - Include kind 30023 articles in feeds (default: true)
- `FEED_INCLUDE_REPLIES` - Include replies (events with e-tags) in feeds (default: false)

### Algorithmic Feeds (Opt-in)
- `ALGO_ENABLED` - Enable algorithmic feed support (default: false)
- `ALGO_DEFAULT` - Default algorithm: chronological, wot-ranked, engagement, trending (default: chronological)
- `ALGO_WOT_WEIGHT` - Weight for WoT score in trending algorithm (0-1, default: 0.3)
- `ALGO_ENGAGEMENT_WEIGHT` - Weight for engagement score (0-1, default: 0.4)
- `ALGO_RECENCY_WEIGHT` - Weight for recency score (0-1, default: 0.3)

### NIP-29 Relay-based Groups
- `GROUPS_ENABLED` - Enable NIP-29 groups support (default: false)
- `GROUPS_RELAY_URL` - Relay URL for groups (defaults to RELAY_URL)
- `GROUPS_ADMIN_PUBKEYS` - Pubkeys that can always create groups (defaults to ADMIN_PUBKEYS)
- `GROUPS_ALLOW_PUBLIC_CREATION` - Allow any authenticated user to create groups (default: false)
- `GROUPS_MAX_PER_USER` - Maximum groups a user can create (default: 10, 0 = unlimited)
- `GROUPS_DEFAULT_PRIVACY` - Default privacy: open, restricted, private, hidden, closed (default: restricted)
- `GROUPS_INVITE_EXPIRY_HOURS` - Default invite code expiry in hours (default: 168 = 1 week)

## Monitoring Endpoints (Relay)

- `/metrics` - Prometheus metrics (includes DB pool stats)
- `/health` - Health check (returns "OK")
- `/` - NIP-11 relay info (with Accept: application/nostr+json header)
- `/management` - NIP-86 relay management API (requires NIP-98 auth)
- `/feed/rss` or `/feed/rss.xml` - RSS 2.0 feed of owner's posts (when FEED_ENABLED)
- `/feed/atom` or `/feed/atom.xml` - Atom 1.0 feed of owner's posts (when FEED_ENABLED)
- `/debug/pprof/*` - Go pprof profiling (when PPROF_ENABLED=true)

## Admin UI

Htmx-based web interface for NIP-86 relay management, integrated into the main relay binary.

- **Routing:** Host-based routing in `cmd/relay/main.go` - requests to `relay-admin.*` hostnames are routed to the admin UI mux
- **URL:** `https://relay-admin.cloistr.xyz/` (via Cloudflare Tunnel, LAN DNS points to internal IP)
- **Auth:** NIP-07 browser extension + NIP-98 HTTP signatures
- **Features:** Pubkey ban/allow, event ban, moderation queue, IP blocking, kind allowlist, relay settings, HAVEN dashboard, event browser, connection stats, WoT visualization
- **HAVEN Page:** Shows box routing status, owner info, Blastr/Importer stats (auto-refreshes every 30s)
- **Event Browser:** Search/filter/view stored events with pagination, ban/unban actions
- **Connection Stats:** Database pool stats, event distribution by kind, server uptime
- **WoT Visualization:** Interactive D3.js trust network graph, follow statistics, trust level distribution
- **Requirements:** `ADMIN_PUBKEYS` must be set, `mgmtStore` initialized (happens when admin pubkeys configured)

## Completed Phases

| Phase | Focus | Status |
|-------|-------|--------|
| Phase 0 | Foundation (Core Infrastructure) | ✅ Complete |
| Phase 1 | Dragonfly Expansion | ✅ Complete |
| Phase 2 | Additional NIPs | ✅ Complete |
| Phase 3 | Infrastructure (HPA, alerting, backup) | ✅ Complete |
| Phase 4 | Performance (pool tuning, indexes, pprof) | ✅ Complete |
| Phase 5 | Observability (logging, tracing, Grafana) | ✅ Complete |
| NIP-66 | Relay Health Monitoring | ✅ Complete |
| HAVEN Phase 1 | Box routing | ✅ Complete |
| HAVEN Phase 2 | Blastr + Importer | ✅ Complete |
| HAVEN Phase 3 | Prometheus Metrics | ✅ Complete |
| RSS Bridge | Nostr-to-RSS/Atom feeds | ✅ Complete |
| Algorithmic Feeds | Opt-in ranked feeds (WoT, engagement, trending) | ✅ Complete |
| HAVEN E-tag Routing | Route reactions/reposts via event lookup | ✅ Complete |
| Blastr Retry Queue | Persistent retry for failed broadcasts | ✅ Complete |
| NIP-29 Groups | Relay-based closed-membership groups | ✅ Complete |
| NIP-17 Private DMs | Modern encrypted DMs (gift-wrapped) | ✅ Complete |
| Admin UI Improvements | Event browser, stats dashboard, WoT visualization | ✅ Complete |

## HAVEN-Style Relay Separation

**Reference:** [bitvora/haven](https://github.com/bitvora/haven) - "High Availability Vault for Events on Nostr"

HAVEN implements the Outbox Model (proposed by Mike Dilger) with four relay types in one:

### The Four Boxes

| Box | Purpose | Access | Event Kinds |
|-----|---------|--------|-------------|
| **Private** | Drafts, eCash, personal notes | Owner only (auth required) | 30024, 31234, 7375, 7376, 30078, 10003, 30003 |
| **Chat** | DMs and group chats | WoT-filtered (auth required) | 4, 13, 14, 15, 1059, 1060 (NIP-04, NIP-17, NIP-59) |
| **Inbox** | Events addressed to owner | Public write, owner read | 1, 6, 7, 9735, 1111, 30023 (when tagged) |
| **Outbox** | Owner's public notes | Owner write, public read | 0, 1, 3, 6, 7, 10002, 10050, 30023 |

### Implementation Status

| Feature | Status | Notes |
|---------|--------|-------|
| **Box Router** | ✅ Complete | Routes events by kind/author/tags |
| **E-tag Routing** | ✅ Complete | Routes reactions/reposts to inbox via event lookup |
| **Auth Policies** | ✅ Complete | Per-box authentication requirements |
| **Filter Routing** | ✅ Complete | Query routing to correct boxes |
| **WoT Integration** | ✅ Ready | Chat box uses existing WoT |
| **Inbox Importer** | ✅ Complete | Pull tagged events from other relays (polling) |
| **Inbox Subscriber** | ✅ Complete | Real-time WebSocket subscriptions for instant updates |
| **Outbox Blastr** | ✅ Complete | Broadcast outbox events to other relays |
| **Prometheus Metrics** | ✅ Complete | Full metrics for box routing, Blastr, Importer |
| **Admin UI** | ✅ Complete | HAVEN dashboard in relay admin panel |

### Configuration

```bash
# Enable HAVEN box routing
HAVEN_ENABLED=true
HAVEN_OWNER_PUBKEY=<hex>              # Owner pubkey for box routing

# Access control (defaults shown)
HAVEN_ALLOW_PUBLIC_OUTBOX_READ=true   # Public can read owner's posts
HAVEN_ALLOW_PUBLIC_INBOX_WRITE=true   # Anyone can tag/mention owner
HAVEN_REQUIRE_AUTH_FOR_CHAT=true      # DMs require authentication
HAVEN_REQUIRE_AUTH_FOR_PRIVATE=true   # Private box requires authentication

# Additional private kinds (comma-separated)
HAVEN_PRIVATE_KINDS=<kind1,kind2,...>

# Outbox Blastr (broadcast owner's posts to other relays)
HAVEN_BLASTR_ENABLED=true
HAVEN_BLASTR_RELAYS=wss://relay1.example.com,wss://relay2.example.com

# Inbox Importer (fetch events tagged to owner from other relays)
HAVEN_IMPORTER_ENABLED=true
HAVEN_IMPORTER_RELAYS=wss://relay1.example.com,wss://relay2.example.com
HAVEN_IMPORTER_REALTIME=true  # Enable real-time WebSocket subscriptions (vs 5-min polling)
```

### Architecture

```
internal/haven/
├── types.go         # Box types, default kinds, config, access policies, EventLookup interface
├── router.go        # Event/filter routing logic (includes E-tag routing for reactions/reposts)
├── handlers.go      # RejectEvent, RejectFilter, OverwriteFilter, HavenSystem
├── blastr.go        # Outbox event broadcasting to other relays
├── importer.go      # Inbox event fetching from other relays
├── metrics.go       # Prometheus metrics for HAVEN components
├── router_test.go   # Router tests (includes E-tag routing tests)
├── handlers_test.go # Handler tests
├── blastr_test.go   # Blastr tests
├── importer_test.go # Importer tests
└── metrics_test.go  # Metrics tests
```

### Blastr (Outbox Broadcasting)

The Blastr component automatically broadcasts owner's outbox events to configured relays:
- Listens for OnEventSaved events
- Routes only outbox events (owner's public posts)
- Manages relay connections with automatic reconnection
- Queues events for async broadcast
- Tracks broadcast statistics
- **Persistent Retry Queue**: Failed broadcasts are stored in Redis/Dragonfly and retried with exponential backoff (30s, 60s, 120s, 240s, 480s, 960s, max 6 retries)

### Importer (Inbox Fetching)

The Importer component polls configured relays for events addressed to the owner:
- Polls every 5 minutes (configurable)
- Looks for events with p-tag matching owner pubkey
- Deduplicates events to avoid storing duplicates
- Routes events to inbox or chat box based on kind
- Tracks import statistics

### E-tag Routing

The E-tag routing feature enables intelligent inbox routing for reactions (kind 7) and reposts (kind 6) that reference the owner's events, even without a p-tag:

**How it works:**
1. When a reaction or repost arrives without a p-tag to the owner
2. The router looks up the referenced event(s) in the e-tags
3. If any referenced event was authored by the owner, the event is routed to inbox
4. This allows receiving reactions/reposts to owner's content from other relays

**Implementation:**
- `EventLookup` interface in `types.go` for database queries
- `referencesOwnerEvent()` method in `router.go` for e-tag checking
- Adapter in `main.go` wraps PostgreSQL backend for event lookups
- P-tag routing takes precedence (no lookup needed if owner is p-tagged)

**Example:**
```
Event: Reaction (kind 7) from Alice
Tags: [["e", "owner_note_123"]]

Without E-tag routing: → BoxUnknown (rejected)
With E-tag routing: Looks up owner_note_123 → Author is owner → BoxInbox
```

### HAVEN Prometheus Metrics

HAVEN exposes comprehensive Prometheus metrics at `/metrics`:

**Box Routing Metrics:**
- `nostr_relay_haven_events_routed_total{box}` - Events routed to each box
- `nostr_relay_haven_events_rejected_total{box,reason}` - Events rejected per box
- `nostr_relay_haven_filters_routed_total{box}` - Filters routed to each box
- `nostr_relay_haven_filters_rejected_total{box,reason}` - Filter rejections
- `nostr_relay_haven_access_attempts_total{box,operation}` - Box access attempts
- `nostr_relay_haven_access_denied_total{box,operation,reason}` - Access denials

**Blastr Metrics:**
- `nostr_relay_haven_blastr_events_broadcast_total` - Successfully broadcast events
- `nostr_relay_haven_blastr_events_failed_total` - Failed broadcasts
- `nostr_relay_haven_blastr_events_queued_total` - Events queued for broadcast
- `nostr_relay_haven_blastr_events_dropped_total` - Events dropped (queue full)
- `nostr_relay_haven_blastr_relays_connected` - Connected relay count (gauge)
- `nostr_relay_haven_blastr_queue_size` - Current queue size (gauge)
- `nostr_relay_haven_blastr_relay_publish_total{relay,status}` - Per-relay publish stats
- `nostr_relay_haven_blastr_retry_queued_total` - Failed broadcasts queued for retry
- `nostr_relay_haven_blastr_retry_success_total` - Successful retries
- `nostr_relay_haven_blastr_retry_exhausted_total` - Retries that exhausted max attempts
- `nostr_relay_haven_blastr_retry_queue_size` - Current retry queue size (gauge)

**Importer Metrics:**
- `nostr_relay_haven_importer_events_imported_total` - Events imported
- `nostr_relay_haven_importer_events_skipped_total` - Events skipped (duplicates)
- `nostr_relay_haven_importer_fetch_errors_total` - Fetch errors
- `nostr_relay_haven_importer_relays_polled` - Relays polled count (gauge)
- `nostr_relay_haven_importer_last_poll_timestamp` - Last poll timestamp (gauge)
- `nostr_relay_haven_importer_relay_fetch_total{relay,status}` - Per-relay fetch stats

**System Status:**
- `nostr_relay_haven_enabled` - HAVEN enabled status (1/0)
- `nostr_relay_haven_blastr_enabled` - Blastr enabled status (1/0)
- `nostr_relay_haven_importer_enabled` - Importer enabled status (1/0)
- `nostr_relay_haven_box_events_stored{box}` - Events stored per box (gauge)

## RSS/Atom Feeds (Nostr-to-RSS Bridge)

The relay includes built-in RSS and Atom feed generation for syndicating owner's posts to traditional feed readers.

### Features

- **RSS 2.0** feed at `/feed/rss` (or `/feed/rss.xml`)
- **Atom 1.0** feed at `/feed/atom` (or `/feed/atom.xml`)
- Automatic conversion of kind 1 (short notes) and kind 30023 (long-form articles)
- Configurable item limits via `?limit=N` query parameter
- Proper HTML formatting with URL linkification
- Hashtag extraction from event t-tags
- Reply filtering (excludes replies by default)
- Cache headers for CDN/proxy caching (5 minute TTL)

### Configuration

```bash
# Enable feeds (auto-enabled when HAVEN is enabled)
FEED_ENABLED=true

# Items per feed
FEED_LIMIT=20              # Default items
FEED_MAX_LIMIT=100         # Maximum via ?limit=

# Content options
FEED_INCLUDE_LONG_FORM=true   # Include kind 30023 articles
FEED_INCLUDE_REPLIES=false    # Exclude replies by default
```

### Architecture

```
internal/feeds/
├── types.go         # Config, FeedItem, FeedMetadata
├── handler.go       # HTTP handlers, event-to-feed conversion
├── rss.go           # RSS 2.0 XML generation
├── atom.go          # Atom 1.0 XML generation
└── feeds_test.go    # Comprehensive tests
```

### Usage

```bash
# Get RSS feed
curl https://relay.cloistr.xyz/feed/rss

# Get Atom feed with custom limit
curl https://relay.cloistr.xyz/feed/atom?limit=50
```

## Algorithmic Feeds (Opt-in)

The relay supports opt-in algorithmic feed ranking, integrating with the existing WoT system.

### Available Algorithms

| Algorithm | Description | Scoring |
|-----------|-------------|---------|
| `chronological` | Default - no ranking | Timestamp only |
| `wot-ranked` | Rank by Web of Trust level | 70% WoT + 30% recency |
| `engagement` | Rank by reactions/reposts/zaps | 70% engagement + 30% recency |
| `trending` | Balance of all factors | Configurable weights |
| `personalized` | User preference boosting | Base score + personal boosts |

### Opt-in Methods

**Via REQ filter tag:**
```json
["REQ", "feed", {"kinds": [1], "#algo": ["wot-ranked"]}]
```

**Via RSS/Atom feed URL parameter:**
```bash
curl https://relay.cloistr.xyz/feed/rss?algo=trending
curl https://relay.cloistr.xyz/feed/atom?algo=engagement&limit=50
```

### Engagement Scoring

Events are scored based on:
- Reactions (kind 7): 1 point each
- Reposts (kind 6): 3 points each
- Replies (kind 1 with e-tag): 2 points each
- Zaps (kind 9735): 5 points + log bonus for amount

### User Preferences (Personalized Algorithm)

The personalized algorithm supports:
- **Muted pubkeys** - Filter out specific users
- **Muted words** - Filter content containing words
- **Muted hashtags** - Filter by hashtag
- **Boosted pubkeys** - Increase score for favorite users
- **Boosted hashtags** - Increase score for topics
- **Min WoT level** - Only show trusted users

### Architecture

```
internal/algo/
├── types.go         # Algorithm types, config, preferences
├── scorer.go        # Event scoring engine
├── handler.go       # Relay integration, filter handling
└── algo_test.go     # Comprehensive tests
```

### Configuration

```bash
# Enable algorithmic feeds (opt-in)
ALGO_ENABLED=true

# Default algorithm (users can override via ?algo= parameter)
ALGO_DEFAULT=chronological

# Trending algorithm weights (should sum to 1.0)
ALGO_WOT_WEIGHT=0.3
ALGO_ENGAGEMENT_WEIGHT=0.4
ALGO_RECENCY_WEIGHT=0.3
```

## NIP-29 Relay-based Groups

The relay implements NIP-29 for closed-membership group communication hosted on the relay.

### Event Kinds

| Kind | Name | Description |
|------|------|-------------|
| 9000 | Add User | Add user to group with optional role |
| 9001 | Remove User | Remove user from group |
| 9002 | Edit Metadata | Update group name, picture, about |
| 9005 | Delete Event | Delete an event from group |
| 9007 | Create Group | Create a new group |
| 9008 | Delete Group | Delete a group |
| 9009 | Create Invite | Create an invite code |
| 9021 | Join Request | Request to join a group |
| 9022 | Leave Request | Request to leave a group |
| 39000 | Group Metadata | Relay-published group info |
| 39001 | Group Admins | Admin list with roles |
| 39002 | Group Members | Member list |
| 39003 | Group Roles | Supported roles |

### Privacy Levels

| Privacy | Can Read | Can Write | Can Join | Show Metadata |
|---------|----------|-----------|----------|---------------|
| `open` | Anyone | Anyone | Yes | Yes |
| `restricted` | Anyone | Members only | Yes | Yes |
| `private` | Members only | Members only | Yes | Yes |
| `hidden` | Members only | Members only | Yes | Members only |
| `closed` | Members only | Members only | No | Yes |

### Architecture

```
internal/groups/
├── types.go         # Event kinds, Privacy, Role, Group, Config
├── store.go         # PostgreSQL storage with in-memory cache
├── handler.go       # Khatru RejectEvent/RejectFilter/OnEventSaved handlers
├── types_test.go    # Privacy method tests, kind helpers
└── handler_test.go  # Event routing, tag extraction tests
```

### Database Schema

The groups implementation creates these tables:
- `nip29_groups` - Group metadata (id, name, picture, about, privacy, created_by)
- `nip29_members` - Group memberships (group_id, pubkey, role, joined_at, added_by)
- `nip29_invites` - Invite codes (code, group_id, expires_at, max_uses, uses)

### Configuration

```bash
# Enable NIP-29 groups
GROUPS_ENABLED=true

# Admin configuration
GROUPS_ADMIN_PUBKEYS=<pubkey1,pubkey2>  # Can always create groups
GROUPS_ALLOW_PUBLIC_CREATION=false      # Allow anyone to create (requires auth)

# Limits
GROUPS_MAX_PER_USER=10                  # Max groups per user (0 = unlimited)
GROUPS_DEFAULT_PRIVACY=restricted       # Default privacy for new groups
GROUPS_INVITE_EXPIRY_HOURS=168          # Invite code expiry (default: 1 week)
```

## NIP-17 Private Direct Messages

The relay implements NIP-17 for modern encrypted direct messaging, replacing the deprecated NIP-04. NIP-17 provides superior privacy through NIP-44 encryption and NIP-59 gift wrapping.

### How NIP-17 Works

NIP-17 uses a three-layer encryption model for maximum privacy:

1. **Rumor (kind 14)**: The unsigned chat message with content and recipient p-tags
2. **Seal (kind 13)**: Wraps the rumor, signed with sender's key
3. **Gift Wrap (kind 1059)**: Wraps the seal with an ephemeral key, hiding all metadata

### Event Kinds

| Kind | Name | Description |
|------|------|-------------|
| 14 | Chat Message | Plain text DM with p-tags identifying recipients |
| 15 | File Message | Encrypted file metadata and URLs |
| 10050 | DM Relay List | User's preferred relays for receiving DMs (replaceable) |

### Privacy Features

| Feature | NIP-04 | NIP-17 |
|---------|--------|--------|
| Sender visible | Yes | No (ephemeral key) |
| Recipient visible | Yes | No (auth required) |
| Timestamp visible | Yes | Randomized |
| Encryption | AES-256-CBC | NIP-44 (ChaCha20) |
| Deniability | No | Yes (unsigned rumors) |

### Relay Behavior

The relay handles NIP-17 as follows:

- **Kind 14/15 → Chat Box**: NIP-17 message kinds route to the chat box
- **Kind 10050 → Outbox**: DM relay lists are public configuration events
- **Authentication**: Gift wrap queries require NIP-42 auth (via existing NIP-59 support)
- **Filter Restriction**: Users can only query kind 1059 events where they're the p-tagged recipient

### Configuration

NIP-17 is automatically enabled when NIP-59 gift wrap is enabled (default: true).

```bash
# NIP-59 gift wrap enables NIP-17 support
GIFTWRAP_ENABLED=true              # Enable gift wrap (default: true)
GIFTWRAP_REQUIRE_AUTH=true         # Require auth for gift wrap queries (default: true)

# HAVEN chat box routes NIP-17 kinds
HAVEN_ENABLED=true
HAVEN_REQUIRE_AUTH_FOR_CHAT=true   # DMs require authentication (default: true)
```

### Database Indexes

Optimized indexes are created automatically:
- `event_nip17_chat_idx` - Partial index for kind 14 events
- `event_nip17_relay_list_idx` - Partial index for kind 10050 events

### Prometheus Metrics

NIP-17 events are tracked with specific labels:
- `nostr_relay_events_received_total{kind="nip17_chat"}` - Kind 14 events
- `nostr_relay_events_received_total{kind="nip17_file"}` - Kind 15 events
- `nostr_relay_events_received_total{kind="nip17_relay_list"}` - Kind 10050 events
- `nostr_relay_events_received_total{kind="gift_wrap"}` - Kind 1059 events
- `nostr_relay_events_received_total{kind="seal"}` - Kind 13 events

## NIP-51 User Lists

The relay implements NIP-51 for standardized list management, enabling users to organize pubkeys, events, relays, and other references.

### Event Kinds

| Kind | Name | Privacy | Description |
|------|------|---------|-------------|
| 10000 | Mute List | Private | Muted pubkeys, events, hashtags, words |
| 10001 | Pin List | Public | Pinned/highlighted events |
| 10002 | Relay List | Public | NIP-65 relay preferences |
| 10003 | Bookmark List | Private | Bookmarked events |
| 10004 | Communities List | Public | Communities user follows |
| 10005 | Public Chats List | Public | Group chats user is in |
| 10006 | Blocked Relays | Private | Relays to avoid |
| 10007 | Search Relays | Public | Preferred NIP-50 relays |
| 10015 | Interests List | Public | Topics of interest |
| 10030 | Emoji List | Public | Custom emoji |
| 30000 | People Sets | Private | Categorized pubkey lists (d-tag) |
| 30001 | Bookmark Sets | Private | Categorized bookmarks (d-tag) |
| 30002 | Relay Sets | Public | Categorized relay groups (d-tag) |
| 30003 | Bookmark Sets | Private | Named bookmark collections (d-tag) |

### HAVEN Box Routing

NIP-51 lists are automatically routed to the appropriate HAVEN box:

**Private Box (owner only):**
- 10000 (Mute List) - Don't reveal who user muted
- 10003 (Bookmark List) - Personal bookmarks
- 10006 (Blocked Relays) - Private preferences
- 30000 (People Sets) - Can contain mute categories
- 30001 (Bookmark Sets) - Personal categorized bookmarks
- 30003 (Bookmark Sets) - Named bookmark collections

**Outbox (public):**
- 10001 (Pin List) - Curated public content
- 10002 (Relay List) - NIP-65 relay preferences
- 10004 (Communities List) - Communities user follows
- 10005 (Public Chats) - Group chats user is in
- 10007 (Search Relays) - Search relay preferences
- 10015 (Interests List) - Topics of interest
- 10030 (Emoji List) - Custom emoji
- 30002 (Relay Sets) - Categorized relay groups

### Architecture

```
internal/lists/
├── types.go         # NIP-51 kind definitions, privacy helpers
└── types_test.go    # Kind validation tests
```

## NIP-65 Relay List Metadata

The relay implements NIP-65 for advertising user relay preferences, enabling proper Outbox Model routing.

### How It Works

Users publish a kind 10002 replaceable event containing their preferred relays:
- **read** relays: where to find their incoming mentions/replies
- **write** relays: where they publish their own notes
- Clients use this to route events and discover where to find a user's content

### Event Structure

```json
{
  "kind": 10002,
  "tags": [
    ["r", "wss://relay.example.com/", "read"],
    ["r", "wss://relay.cloistr.xyz/", "write"],
    ["r", "wss://nos.lol/"]
  ],
  "content": ""
}
```

Tags without a read/write marker indicate both read and write.

### Relay Behavior

- Kind 10002 events are stored in the **Outbox** (owner's public data)
- Replaceable: newer events replace older ones per pubkey
- Cached via event cache for fast lookups
- Indexed via `event_replaceable_idx` for efficient queries

### Integration

- **Blastr** uses relay lists to determine where to broadcast
- **Importer** can use relay lists to discover where to fetch mentions from
- Clients query the relay for kind 10002 to route messages properly

## NIP-32 Content Labeling

The relay implements NIP-32 for content labeling, enabling distributed moderation and content classification.

### How It Works

Labels are kind 1985 events that tag other content (events, pubkeys, relays, topics) with structured metadata:
- **L tags**: Define label namespaces (e.g., "ugc", "relay/moderation")
- **l tags**: Contain label values with namespace references (e.g., ["l", "spam", "ugc"])

### Event Structure

```json
{
  "kind": 1985,
  "tags": [
    ["L", "ugc"],
    ["l", "spam", "ugc"],
    ["l", "nsfw", "ugc"],
    ["e", "abc123", "wss://relay.example.com"],
    ["p", "pubkey456"]
  ],
  "content": ""
}
```

### Label Namespaces

| Namespace | Purpose |
|-----------|---------|
| `ugc` | User-generated content labels |
| `relay/moderation` | Relay admin moderation labels |
| `content-warning` | Content warnings |
| `quality` | Quality/trust indicators |
| `ISO-639-1` | Language codes |
| `license` | Content licensing (CC, MIT, etc.) |

### Common Moderation Labels

| Label | Meaning |
|-------|---------|
| `spam` | Spam content |
| `nsfw` | Not safe for work |
| `adult` | Adult content |
| `gore` | Violent/gore content |
| `abuse` | Abusive content |
| `illegal` | Potentially illegal content |
| `impersonation` | Account impersonation |
| `bot` | Automated/bot account |

### Relay Behavior

- Kind 1985 events are stored in the **Outbox** (public labels)
- Indexed via `event_labels_idx` for efficient queries
- Admin UI displays "Label" for kind 1985 in stats
- Labels can be queried by target (e-tag, p-tag) for moderation

### Architecture

```
internal/labels/
├── types.go         # Kind constants, namespaces, Label/Target types
└── types_test.go    # Comprehensive tests
```

### Use Cases

1. **Content Moderation**: Admins label spam/abuse for filtering
2. **Content Discovery**: Users label content by topic
3. **Trust Networks**: Label trusted/verified accounts
4. **Client-side Filtering**: Clients apply labels for content warnings

## NIP-43 Relay Access & Membership

The relay implements NIP-43 for controlled access through membership and invite codes.

### Event Kinds

| Kind | Name | Description |
|------|------|-------------|
| 13534 | Membership List | Relay-published list of members |
| 8000 | Add Member | Notification when user joins |
| 8001 | Remove Member | Notification when user leaves |
| 28934 | Join Request | User request to join with optional invite code |
| 28935 | Invite Response | Ephemeral invite code response |
| 28936 | Leave Request | User request to leave |

### How It Works

1. **User requests to join** by publishing kind 28934 with optional invite code
2. **Relay validates invite** (if required) and adds user to members
3. **Relay publishes kind 8000** notification
4. **Relay updates kind 13534** membership list (if published)
5. **User can leave** by publishing kind 28936

All membership events include the NIP-70 protected tag (`-`) to prevent propagation to other relays.

### Configuration

```bash
# Enable NIP-43 membership
MEMBERSHIP_ENABLED=true

# Relay identity (required for signing)
MEMBERSHIP_RELAY_PRIVATE_KEY=<hex>

# Access control
MEMBERSHIP_REQUIRE_MEMBERSHIP=false  # Require membership to access
MEMBERSHIP_ALLOW_JOIN_REQUESTS=true  # Allow self-service joins
MEMBERSHIP_PUBLISH_LIST=false        # Publish member list publicly

# Invite defaults
MEMBERSHIP_INVITE_EXPIRY_HOURS=168   # 1 week
MEMBERSHIP_INVITE_MAX_USES=1         # Single-use by default
```

### Database Schema

The membership implementation creates these tables:
- `nip43_members` - Member pubkeys and join timestamps
- `nip43_invites` - Invite codes with expiry and usage tracking

### Architecture

```
internal/membership/
├── types.go         # Kind constants, Member, Invite, Config
├── store.go         # PostgreSQL persistence
└── types_test.go    # Comprehensive tests
```

### Use Cases

1. **Private Relay**: Only members can read/write
2. **Invite-Only**: New users need invite codes
3. **Public with Benefits**: Members get priority or extra features
4. **HAVEN Integration**: Combine with box routing for granular access

## NIP-72 Moderated Communities

The relay implements NIP-72 for Reddit-style public communities with moderation.

### How It Works

Unlike NIP-29 (relay-based closed groups), NIP-72 communities are:
- **Public**: Anyone can view and post
- **Moderated**: Moderators approve posts for visibility
- **Distributed**: Can span multiple relays

### Event Kinds

| Kind | Name | Description |
|------|------|-------------|
| 34550 | Community Definition | Replaceable event defining community and moderators |
| 1111 | Community Post | Posts to a community (references via A tag) |
| 4550 | Approval | Moderator approval (includes full post JSON) |

### Community Definition Structure

```json
{
  "kind": 34550,
  "tags": [
    ["d", "community-id"],
    ["name", "My Community"],
    ["description", "A test community"],
    ["image", "https://example.com/img.png", "800x600"],
    ["rules", "Be nice"],
    ["relay", "wss://relay.example.com"],
    ["p", "moderator-pubkey", "", "moderator", "admin"]
  ]
}
```

### Post Structure

Posts reference communities using uppercase A and P tags:
```json
{
  "kind": 1111,
  "tags": [
    ["A", "34550:owner-pubkey:community-id"],
    ["P", "owner-pubkey"]
  ],
  "content": "Hello community!"
}
```

### Approval Flow

1. User posts kind 1111 event with A tag referencing community
2. Moderator reviews post
3. Moderator publishes kind 4550 approval with:
   - Community a-tag reference
   - Post e-tag reference
   - Full post JSON in content
4. Clients display approved posts

### Architecture

```
internal/communities/
├── types.go         # Kind constants, Community, Approval types
└── types_test.go    # Comprehensive tests
```

### Comparison: NIP-29 vs NIP-72

| Feature | NIP-29 Groups | NIP-72 Communities |
|---------|---------------|-------------------|
| Access | Membership-based | Public |
| Moderation | By membership | By approval |
| Visibility | Members only | Anyone can view |
| Post flow | Direct if member | Pending approval |
| Relay role | Manages group | Stores events |

## NIP-52 Calendar Events

The relay implements NIP-52 for calendar events and RSVPs, enabling event scheduling on Nostr.

### Event Kinds

| Kind | Name | Description |
|------|------|-------------|
| 31922 | Date-based Event | Event on a specific date (all-day) |
| 31923 | Time-based Event | Event at specific time with timezone |
| 31924 | Calendar | Collection of calendar events |
| 31925 | RSVP | Response to calendar event |

### Date-based Event Structure

```json
{
  "kind": 31922,
  "tags": [
    ["d", "unique-event-id"],
    ["title", "Community Meetup"],
    ["start", "2024-12-15"],
    ["end", "2024-12-15"],
    ["location", "123 Main St"],
    ["p", "attendee-pubkey"]
  ],
  "content": "Event description here"
}
```

### Time-based Event Structure

```json
{
  "kind": 31923,
  "tags": [
    ["d", "unique-event-id"],
    ["title", "Weekly Call"],
    ["start", "1702656000"],
    ["end", "1702659600"],
    ["start_tzid", "America/New_York"],
    ["location", "https://meet.example.com"],
    ["p", "attendee-pubkey"]
  ],
  "content": "Weekly team sync"
}
```

### RSVP Structure

```json
{
  "kind": 31925,
  "tags": [
    ["d", "31923:organizer-pubkey:event-id"],
    ["a", "31923:organizer-pubkey:event-id"],
    ["status", "accepted"]
  ],
  "content": ""
}
```

Status values: `accepted`, `declined`, `tentative`

### Architecture

```
internal/calendar/
├── types.go         # Kind constants, CalendarEvent, RSVP, Calendar types
└── types_test.go    # Comprehensive tests
```

### Use Cases

1. **Community Events**: Nostr meetups and conferences
2. **Personal Calendars**: Private event tracking
3. **Group Scheduling**: Coordinate with followers/contacts
4. **Event Discovery**: Public events with location tags

## NIP-88 Polls

The relay implements NIP-88 for community polling, enabling structured voting on questions.

### Event Kinds

| Kind | Name | Description |
|------|------|-------------|
| 1068 | Poll | Poll question with options |
| 1018 | Poll Response | User's vote(s) |

### Poll Structure

```json
{
  "kind": 1068,
  "tags": [
    ["poll_option", "0", "Option A"],
    ["poll_option", "1", "Option B"],
    ["poll_option", "2", "Option C"],
    ["closed_at", "1718438400"],
    ["min_choices", "1"],
    ["max_choices", "2"]
  ],
  "content": "What is your favorite option?"
}
```

### Poll Response Structure

```json
{
  "kind": 1018,
  "tags": [
    ["e", "poll-event-id"],
    ["response", "0"],
    ["response", "2"]
  ],
  "content": ""
}
```

### Features

- **Multiple Choice**: Configure min/max selections
- **Deadline**: Optional `closed_at` timestamp
- **Validation**: `ValidateResponse()` checks selection bounds
- **Parsing**: Extract polls and responses from events

### Architecture

```
internal/polls/
├── types.go         # Kind constants, Poll, PollOption, PollResponse types
└── types_test.go    # Comprehensive tests
```

### Use Cases

1. **Community Decisions**: Vote on group topics
2. **Content Feedback**: Audience polling
3. **Event Planning**: Schedule preferences
4. **Governance**: DAO-style voting

## NIP-73 External Content IDs

The relay implements NIP-73 for referencing external content by global identifiers, enabling cross-referencing Nostr content with external media.

### Supported Identifier Types

| Type | Prefix | Example |
|------|--------|---------|
| ISBN | `isbn` | `isbn:9780141036144` |
| DOI | `doi` | `doi:10.1000/xyz123` |
| ISAN | `isan` | `isan:0000-0000-0000-0000-0000-X` |
| IMDB | `imdb` | `imdb:tt0111161` |
| TMDB | `tmdb` | `tmdb:movie/550` |
| Spotify | `spotify` | `spotify:album:abc123` |
| MusicBrainz | `musicbrainz` | `musicbrainz:release/abc` |
| Podcast GUID | `podcast:guid` | `podcast:guid:abc-123` |
| OpenLibrary | `openlibrary` | `openlibrary:OL123W` |

### Event Structure

External references use i-tags:

```json
{
  "kind": 1,
  "tags": [
    ["i", "isbn:9780141036144"],
    ["i", "doi:10.1000/xyz123"],
    ["k", "book"],
    ["k", "review"]
  ],
  "content": "Just finished reading 1984..."
}
```

### Features

- **Parsing**: `ParseExternalRefs()` extracts i-tags from events
- **Validation**: `IsValidISBN()`, `IsValidDOI()` for format checking
- **Kind Hints**: k-tags for content categorization
- **Formatting**: `FormatITag()` for creating i-tag values

### Architecture

```
internal/external/
├── types.go         # Type constants, ExternalRef, parsing functions
└── types_test.go    # Comprehensive tests
```

### Use Cases

1. **Book Reviews**: Reference books by ISBN
2. **Academic Discussion**: Reference papers by DOI
3. **Movie/TV Reviews**: Reference by IMDB/TMDB IDs
4. **Music Sharing**: Reference albums by Spotify/MusicBrainz
5. **Podcast Notes**: Reference episodes by GUID

## NIP-85 Trusted Assertions

The relay implements NIP-85 for delegated Web of Trust scoring, allowing users to trust service providers for reputation calculations.

### How It Works

1. Users publish kind 10040 events listing their trusted assertion providers
2. Providers publish kind 30040 events with trust assertions about pubkeys/events
3. Clients combine WoT data with trusted provider assertions
4. Enables specialized trust services (spam detection, verification, etc.)

### Event Kinds

| Kind | Name | Description |
|------|------|-------------|
| 10040 | Trusted Providers List | User's trusted assertion providers (replaceable) |
| 30040 | Trust Assertion | Provider's assertion about a target |

### Trusted Providers List Structure

```json
{
  "kind": 10040,
  "tags": [
    ["p", "provider-pubkey-1", "wss://provider-relay.com"],
    ["p", "provider-pubkey-2"]
  ],
  "content": ""
}
```

### Trust Assertion Structure

```json
{
  "kind": 30040,
  "tags": [
    ["d", "spam"],
    ["p", "target-pubkey"],
    ["e", "target-event-id"],
    ["assertion", "spam"],
    ["reason", "Known spam account"]
  ],
  "content": ""
}
```

### Assertion Types

| Type | Meaning |
|------|---------|
| `trusted` | Verified trustworthy |
| `spam` | Known spammer |
| `bot` | Automated account |
| `impersonation` | Fake identity |
| `verified` | Identity verified |
| `banned` | Should be blocked |

### Features

- **Provider Management**: Parse/create trusted provider lists
- **Assertion Parsing**: Extract assertions from events
- **Provider Check**: `IsTrustedProvider()` to validate sources
- **Flexible Targets**: Assert on pubkeys and/or events

### Architecture

```
internal/trust/
├── types.go         # Kind constants, TrustProvider, TrustAssertion types
└── types_test.go    # Comprehensive tests
```

### Use Cases

1. **Spam Filtering**: Trust spam detection services
2. **Identity Verification**: Use verification providers
3. **Content Moderation**: Delegate to moderation services
4. **WoT Augmentation**: Extend trust calculations with external data
5. **Domain Expertise**: Trust topic-specific curators

## Future Roadmap

| Item | Description | Priority | Status |
|------|-------------|----------|--------|
| **NIP-51 Lists** | User lists (mutes, bookmarks, relay sets) | High | ✅ Complete |
| **NIP-65 Relay List** | Relay list metadata (kind 10002) | High | ✅ Complete |
| **NIP-32 Labeling** | Content labeling for moderation (kind 1985) | High | ✅ Complete |
| **NIP-43 Relay Access** | Membership management and invites | Medium | ✅ Complete |
| **NIP-72 Communities** | Moderated public communities | Medium | ✅ Complete |
| **NIP-52 Calendar** | Calendar events and RSVPs | Low | ✅ Complete |
| **NIP-88 Polls** | Community polling | Low | ✅ Complete |
| **NIP-73 External IDs** | External content identifiers | Low | ✅ Complete |
| **NIP-85 Trusted Assertions** | Delegated WoT scoring | Low | ✅ Complete |
| **NIP-0A CRDTs** | Contact list conflict resolution (watching PR #1630) | Low | Watching |
| **Geographic Distribution** | Multi-region deployment | Low | Planned |

### Completed Enhancements
- ~~**Admin UI Improvements**~~: Event browser with filters, connection stats dashboard, interactive WoT visualization
- ~~**NIP-17 Private DMs**~~: Modern encrypted DMs with NIP-44 encryption and NIP-59 gift wrapping
- ~~**NIP-29 Groups**~~: Relay-based chat groups with membership and moderation
- ~~**Importer Webhooks**~~: Real-time WebSocket subscriptions for instant inbox updates
- ~~**Blastr Retry Logic**~~: Persistent retry queue for failed broadcasts (commit 924cd29)
- ~~**NIP-51 Lists**~~: User lists (mutes, bookmarks, relay sets) with HAVEN routing
- ~~**NIP-65 Relay List**~~: Relay list metadata for Outbox Model routing
- ~~**NIP-32 Labeling**~~: Content labeling for distributed moderation
- ~~**NIP-43 Membership**~~: Relay access management with invites
- ~~**NIP-72 Communities**~~: Reddit-style moderated communities
- ~~**NIP-52 Calendar**~~: Calendar events and RSVPs
- ~~**NIP-88 Polls**~~: Community polling with multi-choice support
- ~~**NIP-73 External IDs**~~: Reference external content (ISBN, DOI, IMDB)
- ~~**NIP-85 Trusted Assertions**~~: Delegated WoT providers

## Resources

- [HAVEN GitHub](https://github.com/bitvora/haven)
- [Outbox Model (Nostrify)](https://nostrify.dev/relay/outbox)
- [Relay Type Nomenclature Discussion](https://github.com/nostr-protocol/nips/issues/1282)

# CLAUDE.md - coldforge-relay

**Custom Nostr relay built with khatru (Go)**

**Status:** Working - 18 NIPs implemented (1, 9, 11, 13, 22, 33, 40, 42, 45, 46, 50, 57, 59, 66, 70, 77, 86, 94) + WoT Filtering + Admin UI

**Domain:** relay.cloistr.xyz (Cloistr is the consumer-facing brand for Coldforge Nostr services)
**Admin UI:** relay-admin.cloistr.xyz (LAN-only, standalone deployment)

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
│   ├── relay/          # Main relay entry point
│   └── admin/          # Standalone admin UI entry point
├── internal/
│   ├── admin/          # Admin UI handlers (htmx + NIP-98 auth)
│   ├── auth/           # NIP-42 authentication
│   ├── cache/          # Redis/Dragonfly client wrapper
│   ├── config/         # Configuration loading
│   ├── eventcache/     # Hot event caching (Dragonfly)
│   ├── giftwrap/       # NIP-59 gift wrap handling
│   ├── handlers/       # Event validation, NIP-40/22/13
│   ├── logging/        # Structured JSON logging
│   ├── management/     # NIP-86 relay management API + store
│   ├── middleware/     # Observability middleware (logging + tracing)
│   ├── nip66/          # NIP-66 relay discovery and self-monitoring
│   ├── protected/      # NIP-70 protected events handling
│   ├── ratelimit/      # Distributed rate limiting (Dragonfly)
│   ├── relay/          # Khatru relay setup
│   ├── search/         # NIP-50 PostgreSQL full-text search
│   ├── session/        # Distributed session state (Dragonfly)
│   ├── storage/        # PostgreSQL backend
│   ├── tracing/        # Distributed tracing with spans
│   ├── wot/            # Web of Trust filtering
│   ├── writeahead/     # Write-ahead log (Dragonfly)
│   └── zaps/           # NIP-57 Lightning zaps
├── dashboards/         # Grafana dashboard JSON files
├── web/
│   ├── templates/      # HTML templates (layout + 8 pages + 7 partials)
│   └── static/js/      # NIP-07/NIP-98 auth helper
├── tests/              # Test documentation
├── Dockerfile          # Relay multi-stage build
├── Dockerfile.admin    # Admin UI multi-stage build
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

## Monitoring Endpoints (Relay)

- `/metrics` - Prometheus metrics (includes DB pool stats)
- `/health` - Health check (returns "OK")
- `/` - NIP-11 relay info (with Accept: application/nostr+json header)
- `/management` - NIP-86 relay management API (requires NIP-98 auth)
- `/debug/pprof/*` - Go pprof profiling (when PPROF_ENABLED=true)

## Admin UI

Standalone htmx-based web interface for NIP-86 relay management.

- **Deployed as:** separate container (`coldforge-relay-admin`)
- **URL:** `https://relay-admin.cloistr.xyz/` (LAN-only via nginx)
- **Auth:** NIP-07 browser extension + NIP-98 HTTP signatures
- **Features:** Pubkey ban/allow, event ban, moderation queue, IP blocking, kind allowlist, relay settings
- **ArgoCD app:** `relay-admin-production` in coldforge-config
- **Config repo:** `coldforge-config/base/relay-admin/` + `overlays/production/relay-admin/`

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

## Next: HAVEN-Style Relay Separation

**Reference:** [bitvora/haven](https://github.com/bitvora/haven) - "High Availability Vault for Events on Nostr"

HAVEN implements the Outbox Model (proposed by Mike Dilger) with four relay types in one:

### The Four Boxes

| Box | Purpose | Access | Event Kinds |
|-----|---------|--------|-------------|
| **Private** | Drafts, eCash, personal notes | Owner only (auth required) | Any private kinds |
| **Chat** | DMs and group chats | WoT-filtered (auth required) | 4, 1059 (gift wrap) |
| **Inbox** | Events addressed to owner | Public write, owner read | Mentions, replies, zaps |
| **Outbox** | Owner's public notes | Owner write, public read | 1, 6, 7, 30023, etc. |

### Implementation Approach

**Option A: Virtual Separation (Single Relay)**
- Route events to different storage paths based on kind/author
- Different auth policies per "box"
- Simpler deployment, shared resources

**Option B: Physical Separation (Multiple Relays)**
- Four separate relay instances
- Different URLs: private.relay.xyz, chat.relay.xyz, etc.
- Better isolation, more complex deployment

**Option C: Hybrid (Recommended)**
- Single relay binary with box routing
- Virtual separation in storage
- Optional: Different ports/paths per box

### Features to Implement

1. **Box Router** - Route events to correct box based on kind/author/tags
2. **Auth Policies** - Different auth requirements per box
3. **Inbox Importer** - Pull tagged events from other relays
4. **Outbox Blastr** - Broadcast outbox events to other relays
5. **WoT Integration** - Already have this for chat/inbox spam protection

### Configuration (Planned)

```
HAVEN_ENABLED=true
HAVEN_MODE=virtual          # virtual, physical, hybrid
HAVEN_OWNER_PUBKEY=...      # Owner pubkey for box routing
HAVEN_PRIVATE_KINDS=...     # Kinds for private box
HAVEN_BLASTR_RELAYS=...     # Relays to blast outbox to
HAVEN_IMPORT_RELAYS=...     # Relays to import inbox from
```

## Future Roadmap

| Item | Description | Priority |
|------|-------------|----------|
| **HAVEN Separation** | Inbox/Outbox/Private/Chat model | 🔴 Next |
| **RSS Bridge** | atomstr or built-in feed integration | Medium |
| **Algorithmic Feeds** | User-controlled feed algorithms | Medium |
| **NIP-0A CRDTs** | Contact list conflict resolution | Medium |
| **Geographic Distribution** | Multi-region deployment | Low |

## Resources

- [HAVEN GitHub](https://github.com/bitvora/haven)
- [Outbox Model (Nostrify)](https://nostrify.dev/relay/outbox)
- [Relay Type Nomenclature Discussion](https://github.com/nostr-protocol/nips/issues/1282)

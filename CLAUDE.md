# CLAUDE.md - coldforge-relay

**Custom Nostr relay built with khatru (Go)**

**Status:** Working - 13 NIPs implemented (1, 9, 11, 13, 22, 33, 40, 42, 45, 46, 50, 77, 86) + WoT Filtering

**Domain:** relay.cloistr.xyz (Cloistr is the consumer-facing brand for Coldforge Nostr services)

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
├── cmd/relay/          # Main entry point
├── internal/
│   ├── auth/           # NIP-42 authentication
│   ├── cache/          # Redis/Dragonfly caching
│   ├── config/         # Configuration loading
│   ├── handlers/       # Event validation, NIP-40/22/13
│   ├── management/     # NIP-86 relay management API
│   ├── relay/          # Khatru relay setup
│   ├── search/         # NIP-50 PostgreSQL full-text search
│   ├── storage/        # PostgreSQL backend
│   └── wot/            # Web of Trust filtering
├── tests/              # Test documentation
├── Dockerfile          # Multi-stage build
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

## Monitoring Endpoints

- `/metrics` - Prometheus metrics
- `/health` - Health check (returns "OK")
- `/` - NIP-11 relay info (with Accept: application/nostr+json header)
- `/management` - NIP-86 relay management API (requires NIP-98 auth)

## Next Steps

See `~/claude/coldforge/services/relay/CLAUDE.md` for full roadmap.

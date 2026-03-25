# CLAUDE.md - coldforge-relay

**Custom Nostr relay built with khatru (Go)**

**Status:** Production - 30 NIPs implemented
**Domain:** relay.cloistr.xyz | **Admin:** relay-admin.cloistr.xyz

## Required Reading

| Document | Purpose |
|----------|---------|
| `~/claude/coldforge/cloistr/CLAUDE.md` | Cloistr project rules |
| `~/claude/coldforge/cloistr/services/relay/CLAUDE.md` | Service documentation |
| [docs/reference.md](docs/reference.md) | Full config, NIP details, architecture |

## Autonomous Work Mode

**Work autonomously. Do NOT stop to ask what to do next.**

- Keep working until task complete or genuine blocker
- Make reasonable decisions - don't ask permission on obvious choices
- If tests fail, fix them. Use reviewer agent. Keep going.
- Update docs as you make progress

## Agent Usage

| When | Agent |
|------|-------|
| Starting work / need context | `explore` |
| Research NIPs or protocols | `explore` |
| After significant code changes | `reviewer` |
| Writing tests | `test-writer` |
| Running tests | `tester` |
| Investigating bugs | `debugger` |
| Security-sensitive code | `security` |

## Quick Commands

```bash
docker compose up -d                    # Run locally
docker compose logs -f relay            # View logs

# Run tests
docker build --target test -t coldforge-relay:test-runner .
docker run --rm coldforge-relay:test-runner

# Check relay info (NIP-11)
curl -H "Accept: application/nostr+json" http://localhost:3334/
```

## Project Structure

```
cmd/relay/           Main entry point (host-based routing for admin UI)
internal/
  admin/             Admin UI (htmx + NIP-98 auth)
  algo/              Algorithmic feeds (WoT-ranked, engagement, trending)
  auth/              NIP-42 authentication
  cache/             Redis/Dragonfly wrapper
  config/            Configuration loading
  feeds/             RSS/Atom feed generation
  giftwrap/          NIP-59 gift wrap
  groups/            NIP-29 relay-based groups
  handlers/          Event validation (NIP-40/22/13)
  haven/             HAVEN box routing (inbox/outbox/chat/private)
  search/            NIP-50 PostgreSQL full-text search
  storage/           PostgreSQL backend
  wot/               Web of Trust filtering
  zaps/              NIP-57 Lightning zaps
web/templates/       HTML templates (htmx)
dashboards/          Grafana JSON files
```

## Key Configuration

| Variable | Purpose |
|----------|---------|
| `AUTH_POLICY` | open, auth-read, auth-write, auth-all |
| `WOT_ENABLED` | Enable Web of Trust filtering |
| `WOT_OWNER_PUBKEY` | Relay owner (trust level 0) |
| `HAVEN_ENABLED` | Enable HAVEN box routing |
| `CACHE_URL` | Redis/Dragonfly for distributed state |
| `ADMIN_PUBKEYS` | NIP-86 management API access |

**Full config:** See [docs/reference.md](docs/reference.md) for 50+ environment variables.

## Endpoints

| Path | Purpose |
|------|---------|
| `/` | NIP-11 relay info (Accept: application/nostr+json) |
| `/metrics` | Prometheus metrics |
| `/health` | Health check |
| `/management` | NIP-86 relay management (NIP-98 auth) |
| `/feed/rss` | RSS feed (when FEED_ENABLED) |
| `/feed/atom` | Atom feed (when FEED_ENABLED) |

## Implemented NIPs

1, 9, 11, 13, 17, 22, 29, 32, 33, 40, 42, 43, 45, 46, 50, 51, 52, 57, 59, 65, 66, 70, 72, 73, 77, 85, 86, 88, 94

**Details:** See [docs/reference.md](docs/reference.md) for NIP-specific documentation.

## Completed Phases

| Phase | Focus |
|-------|-------|
| Foundation | khatru, PostgreSQL, NIP-42 auth |
| Dragonfly | Caching, distributed state, pub/sub |
| NIPs | 30 implemented (see list above) |
| HAVEN | Single-owner box routing, Blastr, Importer |
| RSS/Algo | Feeds, ranked algorithms |
| NIP-29 | Relay-based groups |
| NIP-17 | Modern encrypted DMs |
| Admin UI | Event browser, stats, WoT viz |

## Roadmap: Per-User HAVEN

Transform from single-owner to multi-tenant. See [docs/per-user-haven-scope.md](docs/per-user-haven-scope.md).

| Phase | Focus |
|-------|-------|
| 1 | Tier Infrastructure (extend NIP-43 members, Lightning payments) |
| 2 | Per-User WoT (user filter layer, settings table) |
| 3 | Per-User HAVEN Routing (context-aware Router) |
| 4 | Shared Worker Blastr (pool architecture, tier gating) |
| 5 | Shared Worker Importer (scheduler + shared pool) |
| 6 | User Self-Service (settings UI, NIP-78, export) |

## Roadmap: B2B

| Model | Description |
|-------|-------------|
| Self-Hosted | Customer runs own instance (license fee) |
| Managed Relay | Coldforge hosts dedicated relay (monthly BTC) |
| Shared Enterprise | Org tier on relay.cloistr.xyz (per-seat BTC) |

## Future

| Item | Status |
|------|--------|
| NIP-0A CRDTs | Watching PR #1630 |
| Geographic Distribution | After multi-tenant stabilizes |

## Resources

- [HAVEN GitHub](https://github.com/bitvora/haven)
- [Outbox Model (Nostrify)](https://nostrify.dev/relay/outbox)

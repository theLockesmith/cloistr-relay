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

| Phase | Focus | Status |
|-------|-------|--------|
| Phase 0-5 | Foundation, Dragonfly, NIPs, Infra, Observability | Done |
| HAVEN | Box routing, Blastr, Importer, Metrics | Done |
| RSS/Algo | Feeds, ranked algorithms | Done |
| NIP-29 | Relay-based groups | Done |
| NIP-17 | Modern encrypted DMs | Done |
| Admin UI | Event browser, stats, WoT viz | Done |

## Future Roadmap

| Item | Priority |
|------|----------|
| NIP-0A CRDTs | Watching PR #1630 |
| Geographic Distribution | Planned |

## Resources

- [HAVEN GitHub](https://github.com/bitvora/haven)
- [Outbox Model (Nostrify)](https://nostrify.dev/relay/outbox)

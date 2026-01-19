# CLAUDE.md - coldforge-relay

**Custom Nostr relay built with khatru (Go)**

**Status:** Working - NIP-01, NIP-11, NIP-42 implemented

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
│   ├── config/         # Configuration loading
│   ├── handlers/       # Event validation, filters
│   ├── relay/          # Khatru relay setup
│   └── storage/        # PostgreSQL backend
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

## Next Steps

See `~/claude/coldforge/services/relay/CLAUDE.md` for full roadmap.

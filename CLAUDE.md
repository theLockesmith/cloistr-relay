# CLAUDE.md - coldforge-relay

**Nostr relay infrastructure - nostream vs custom build**

## Documentation

Full documentation: `~/claude/coldforge/services/relay/CLAUDE.md`
Coldforge overview: `~/claude/coldforge/CLAUDE.md`

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

- **Run locally:** `docker-compose up`
- **Run tests:** [TBD - scaffold first]
- **Build:** `docker build -t coldforge-relay .`

## Getting Started

If this directory is empty (no src/), use `service-init` to scaffold the project structure first.

Read the service documentation at `~/claude/coldforge/services/relay/CLAUDE.md` to understand what to build.

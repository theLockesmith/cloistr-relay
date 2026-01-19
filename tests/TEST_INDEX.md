# Test Index - coldforge-relay

This document provides a quick reference to all test files in the project.

## Unit Test Files

### `/internal/config/config_test.go`
**Package**: `config`
**Tests**: 18
**Dependencies**: `os`, `testing`

**What it tests**:
- Configuration loading from environment variables
- Default configuration values
- Environment variable overrides
- Invalid input error handling
- Comma-separated list parsing

**Key functions tested**:
- `Load()` - Main configuration loader
- `parseCommaSeparated()` - List parsing helper

**Run with**:
```bash
go test ./internal/config
# or
make test-config
```

### `/internal/handlers/handlers_test.go`
**Package**: `handlers`
**Tests**: 17
**Dependencies**: `context`, `crypto/rand`, `encoding/hex`, `testing`, `time`, `github.com/nbd-wtf/go-nostr`

**What it tests**:
- Event signature validation
- Event ID verification
- Timestamp validation (future events rejected if >5 min ahead)
- Filter complexity limits (authors, IDs, kinds)
- Edge cases at boundary conditions

**Key functions tested**:
- `rejectInvalidEvents()` - Event validation handler
- `rejectComplexFilters()` - Filter complexity handler

**Helper functions**:
- `createValidEvent()` - Creates properly signed test events
- `corruptHex()` - Corrupts hex strings for testing invalid sigs
- `generateRandomHex()` - Generates random hex for large filters

**Run with**:
```bash
go test ./internal/handlers
# or
make test-handlers
```

### `/internal/auth/auth_test.go`
**Package**: `auth`
**Tests**: 17 (12 functional, 5 integration placeholders)
**Dependencies**: `context`, `testing`, `github.com/nbd-wtf/go-nostr`

**What it tests**:
- NIP-42 authentication policies
- Policy string conversions
- Authenticated vs unauthenticated access
- Pubkey matching validation
- Whitelist enforcement
- Helper functions for authentication status

**Key functions tested**:
- `Policy.String()` - Policy enum to string conversion
- `requireAuthForRead()` - Read authentication handler
- `requireAuthForWrite()` - Write authentication handler (returns closure)
- `GetAuthenticatedPubkey()` - Extract pubkey from context
- `IsAuthenticated()` - Check authentication status

**Note**: Some tests cannot fully mock khatru's internal auth context. Full authentication flow testing requires integration tests with a real relay instance.

**Run with**:
```bash
go test ./internal/auth
# or
make test-auth
```

## Integration Test Files

### `/tests/relay_test.go`
**Package**: `tests`
**Tests**: 8 (2 functional, 6 placeholders)
**Dependencies**: `context`, `os`, `testing`, various internal packages

**What it tests**:
- Basic configuration loading integration
- Smoke tests for package imports

**Skipped tests** (require database/relay):
- `TestRelayInitialization` - Full relay startup
- `TestEventHandling` - Event publishing and retrieval
- `TestAuthenticationFlow` - NIP-42 authentication
- `TestEventValidation` - Event validation via relay
- `TestFilterComplexity` - Filter handling via relay

**Run with**:
```bash
go test ./tests/...
# or
make test-integration
```

## Test Documentation

### `/tests/README.md`
Comprehensive test documentation covering:
- Test structure and organization
- How to run tests (local and Docker)
- Coverage details for each package
- Test limitations and future work
- CI/CD integration examples

### `/TEST_SUMMARY.md`
High-level test coverage summary including:
- Overview of all test files
- Test statistics and metrics
- Coverage areas and percentages
- Edge cases covered
- Known limitations
- Future improvements
- CI/CD integration guidance

### `/.test-commands`
Quick reference card for common test commands:
- Makefile targets
- Direct Go commands
- Docker commands
- Coverage analysis
- Useful flags and examples

## Running All Tests

### Quick Run (all unit tests)
```bash
make test
# or
go test ./internal/...
```

### With Coverage
```bash
make test-coverage
# or
go test -cover ./internal/...
```

### Verbose Output
```bash
make test-verbose
# or
go test -v ./internal/...
```

### In Docker (no local Go required)
```bash
make docker-test
# or
docker build -t coldforge-relay-test --target test .
docker run --rm coldforge-relay-test
```

## Test File Locations

```
coldforge-relay/
├── internal/
│   ├── config/
│   │   ├── config.go
│   │   └── config_test.go          ← Config unit tests
│   ├── handlers/
│   │   ├── handlers.go
│   │   └── handlers_test.go        ← Handler unit tests
│   └── auth/
│       ├── auth.go
│       └── auth_test.go            ← Auth unit tests
├── tests/
│   ├── relay_test.go               ← Integration tests
│   ├── README.md                   ← Test documentation
│   └── TEST_INDEX.md               ← This file
├── Makefile                        ← Test targets
├── TEST_SUMMARY.md                 ← Coverage summary
└── .test-commands                  ← Quick reference
```

## Test Coverage by Package

| Package | Lines | Functions | Coverage |
|---------|-------|-----------|----------|
| config | ~117 | 2 | 100% |
| handlers | ~72 | 3 | 100% |
| auth | ~126 | 6 | ~95% |
| **Total** | **~315** | **11** | **~98%** |

*Note: Percentages are estimates. Run `make test-coverage-html` for exact coverage.*

## Adding New Tests

When adding new tests, follow these guidelines:

1. **Location**: Place test file next to the code it tests
   - `internal/pkg/file.go` → `internal/pkg/file_test.go`

2. **Naming**: Use descriptive test names
   - Format: `Test<Function>_<Scenario>_<ExpectedResult>`
   - Example: `TestLoad_InvalidPort_ReturnsError`

3. **Structure**: Follow existing patterns
   - Use table-driven tests for multiple similar cases
   - Mark helpers with `t.Helper()`
   - Clean up with `defer`

4. **Documentation**: Update this index and README
   - Add test count to this file
   - Document what the test covers
   - Update TEST_SUMMARY.md statistics

5. **Integration**: Add to Makefile if needed
   - Create `make test-<package>` target for new packages
   - Update `make test` to include new tests

## CI/CD Integration

Tests are designed to work in CI/CD pipelines:

**GitHub Actions Example**:
```yaml
name: Tests
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.24'
      - run: make test-coverage
      - uses: actions/upload-artifact@v3
        with:
          name: coverage
          path: coverage.html
```

**GitLab CI Example**:
```yaml
test:
  image: golang:1.24-alpine
  script:
    - apk add --no-cache make
    - make test-coverage
  artifacts:
    paths:
      - coverage.html
```

**Docker-based CI** (no Go in CI environment):
```yaml
test:
  script:
    - make docker-test-coverage
```

## Quick Reference

| What | Command |
|------|---------|
| Run all unit tests | `make test` |
| Run with coverage | `make test-coverage` |
| Run in Docker | `make docker-test` |
| Test single package | `go test ./internal/config` |
| Generate HTML coverage | `make test-coverage-html` |
| Run verbose | `make test-verbose` |
| Clean artifacts | `make clean` |

For more commands, see `.test-commands` or run `make help`.

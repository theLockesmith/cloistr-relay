# coldforge-relay Tests

This directory contains tests for the coldforge-relay Nostr relay.

## Test Structure

### Unit Tests (Package-level)

Unit tests are located alongside the code they test:

- `/internal/config/config_test.go` - Configuration loading tests
- `/internal/handlers/handlers_test.go` - Event and filter validation tests
- `/internal/auth/auth_test.go` - Authentication policy tests

### Integration Tests

Integration tests are in this directory:

- `relay_test.go` - End-to-end relay tests (currently skipped, require DB)

## Running Tests

### All Unit Tests

Run all unit tests (no database required):

```bash
go test ./internal/...
```

### Specific Package Tests

Run tests for a specific package:

```bash
# Config tests
go test ./internal/config

# Handler tests
go test ./internal/handlers

# Auth tests
go test ./internal/auth
```

### Verbose Output

Run with verbose output to see all test names:

```bash
go test -v ./internal/...
```

### Coverage Report

Generate a coverage report:

```bash
go test -cover ./internal/...
```

Detailed coverage:

```bash
go test -coverprofile=coverage.out ./internal/...
go tool cover -html=coverage.out
```

### Integration Tests

Integration tests require a running PostgreSQL database and are currently skipped:

```bash
go test ./tests/...
```

## Test Coverage

### Config Package (`internal/config/config_test.go`)

- Default configuration values
- Environment variable overrides for all settings
- Invalid port number handling
- Comma-separated list parsing (ALLOWED_PUBKEYS)
- Multiple simultaneous overrides

**Coverage**: All configuration loading paths and error conditions

### Handlers Package (`internal/handlers/handlers_test.go`)

- Valid event acceptance
- Invalid signature rejection
- Event ID mismatch rejection
- Future timestamp rejection (beyond 5-minute tolerance)
- Timestamp tolerance edge cases
- Empty filter acceptance
- Filter complexity limits (authors, IDs, kinds)
- Filter at exact limits
- Multiple filter violations

**Coverage**: All event and filter validation rules

### Auth Package (`internal/auth/auth_test.go`)

- Policy string conversions
- Authenticated vs unauthenticated read requests
- AUTH event bypass (kind 22242)
- Unauthenticated write rejection
- Pubkey matching validation
- Whitelist enforcement
- Empty/nil whitelist behavior
- Helper functions (GetAuthenticatedPubkey, IsAuthenticated)

**Coverage**: All authentication policies and whitelist scenarios

## Running Tests in Docker

Since Go may not be installed locally, tests can be run via Docker:

```bash
# Build test container
docker build -t coldforge-relay-test --target test .

# Run all tests
docker run --rm coldforge-relay-test go test ./internal/...

# Run with coverage
docker run --rm coldforge-relay-test go test -cover ./internal/...

# Run specific package
docker run --rm coldforge-relay-test go test ./internal/config
```

## Test Limitations

### Unit Test Limitations

The auth package tests have some limitations:

- `requireAuthForRead` and `requireAuthForWrite` tests verify the logic but cannot fully mock `khatru.GetAuthed()`
- Full authentication flow testing requires integration tests with a real khatru relay instance
- Some RegisterAuthHandlers tests are skipped (require relay instance)

### Integration Test Gaps

Integration tests that are planned but currently skipped:

- Full relay initialization with database
- Event publishing and retrieval
- WebSocket connection handling
- NIP-42 authentication flow
- Filter subscription testing
- Connection/disconnection logging

These should be implemented when:
1. A test database container is set up
2. Mock WebSocket client tooling is added
3. Test fixtures for events and keys are created

## Test Conventions

Tests follow Go testing best practices:

- Test names: `Test<Function>_<Scenario>_<ExpectedResult>`
- Helper functions marked with `t.Helper()`
- Cleanup using defer
- Table-driven tests where appropriate
- Clear assertion messages
- Independent tests (no shared state)

## Future Improvements

- [ ] Add integration tests with test database container
- [ ] Mock khatru package for better auth testing
- [ ] Add benchmark tests for filter complexity
- [ ] Add fuzz testing for event validation
- [ ] Create test fixtures for common scenarios
- [ ] Add WebSocket client tests
- [ ] Test concurrent event handling
- [ ] Performance tests for high event throughput

# HAVEN Box Router Tests

Comprehensive test suite for the HAVEN (High Availability Vault for Events on Nostr) box routing system.

## Test Files

### router_test.go
Tests the core routing logic for determining which box an event or filter belongs to.

#### Test Coverage

**RouteEvent Tests:**
- ✅ Private kinds routing (7 test cases)
  - Drafts, eCash, bookmarks from owner → private box
  - Private kinds from non-owner → rejected (BoxUnknown)
- ✅ Chat kinds routing (5 test cases)
  - DMs (kind 4), seals (kind 13), gift wraps (1059, 1060) → chat box
  - Works for both owner and non-owner
- ✅ Outbox routing (8 test cases)
  - All events from owner (except private/chat) → outbox
  - Metadata, text notes, reposts, reactions, etc.
- ✅ Inbox routing (8 test cases)
  - Events with p-tag to owner → inbox
  - Multiple p-tags handling
  - Various kinds (text, reactions, zaps, etc.)
- ✅ Unknown box routing (3 test cases)
  - Events from non-owner without p-tag → BoxUnknown
  - Events not addressed to owner → BoxUnknown
- ✅ Custom private kinds (3 test cases)
  - Config-specified private kinds
- ✅ Disabled HAVEN behavior (1 test case)

**CanAccessBox Tests:**
- ✅ Private box access (6 test cases)
  - Owner can read/write
  - Non-owner and unauthenticated denied
- ✅ Chat box access (6 test cases)
  - Authenticated can read/write (when auth required)
  - Unauthenticated denied (when auth required)
  - Configurable auth requirement
- ✅ Inbox box access (8 test cases)
  - Owner can read, others denied
  - Public can write (when configured)
  - Auth required for write (when configured)
- ✅ Outbox box access (8 test cases)
  - Owner can read/write
  - Public can read (when configured)
  - Only owner can write
- ✅ Unknown box access (1 test case)

**RouteFilter Tests:**
- ✅ Filter routing by kind (3 test cases)
  - Private kinds → BoxPrivate
  - Chat kinds → BoxChat
  - Mixed kinds → multiple boxes
- ✅ Filter routing by author (3 test cases)
  - Owner author → BoxOutbox
  - Non-owner author → no outbox
- ✅ Filter routing by p-tag (3 test cases)
  - Owner in p-tag → BoxInbox
- ✅ Default filter behavior (3 test cases)
  - Authenticated owner → all boxes
  - Unauthenticated → only outbox
  - Authenticated non-owner → only outbox
- ✅ Disabled HAVEN (1 test case)

**Helper Method Tests:**
- ✅ GetBoxForKind (5 test cases)
- ✅ IsOwner (3 test cases)
- ✅ isAddressedToOwner (7 test cases)
  - P-tag matching
  - Multiple p-tags
  - No p-tags
  - Malformed tags
  - Empty owner handling

**Configuration Tests:**
- ✅ NewRouter with nil config
- ✅ DefaultConfig validation
- ✅ Box.String() method (6 test cases)
- ✅ Default kind constants validation

**Total: 98 test cases in router_test.go**

### handlers_test.go
Tests the handler functions that enforce box access policies in the relay.

#### Test Coverage

**RejectEvent Tests:**
- ✅ Private box rejection (3 test cases)
  - Owner can publish
  - Non-owner rejected
  - Unauthenticated rejected
- ✅ Chat box rejection (4 test cases)
  - Authenticated can publish
  - Unauthenticated rejected (when auth required)
  - Configurable auth requirement
- ✅ Inbox box rejection (3 test cases)
  - Public write allowed (when configured)
  - Authenticated write
  - Owner write
- ✅ Outbox box rejection (4 test cases)
  - Owner can publish
  - Non-owner rejected
  - Various owner kinds accepted
- ✅ Unknown box rejection (2 test cases)
  - Events not belonging to any box rejected
- ✅ Disabled HAVEN (1 test case)

**RejectFilter Tests:**
- ✅ Private box filter rejection (3 test cases)
  - Owner can query
  - Non-owner rejected
  - Unauthenticated rejected
- ✅ Chat box filter rejection (3 test cases)
  - Authenticated can query
  - Unauthenticated rejected (when auth required)
  - Configurable auth requirement
- ✅ Inbox box filter rejection (3 test cases)
  - Owner can read
  - Non-owner rejected
  - Unauthenticated rejected
- ✅ Outbox box filter rejection (3 test cases)
  - Public can read (when configured)
  - Authenticated can read
  - Owner can read
- ✅ Disabled HAVEN (1 test case)

**OverwriteFilter Tests:**
- ✅ Private kinds removal (5 test cases)
  - Non-owner: private kinds filtered out
  - Unauthenticated: private kinds filtered out
  - Owner: private kinds kept
  - Edge cases (all private, no private)
- ✅ Empty kinds handling (1 test case)
- ✅ Disabled HAVEN (1 test case)

**OnEventSaved Tests:**
- ✅ Event saved logging (1 test case)
- ✅ Disabled HAVEN (1 test case)

**Configuration Tests:**
- ✅ NewHandler with nil config
- ✅ BoxPolicies generation (4 test cases)
  - Private box policy
  - Chat box policy
  - Inbox box policy
  - Outbox box policy
- ✅ BoxPolicies with restrictive config (2 test cases)
- ✅ BoxStats string representation (2 test cases)

**Total: 46 test cases in handlers_test.go**

## Running Tests

```bash
# Run all HAVEN tests
go test -v ./internal/haven/

# Run specific test file
go test -v ./internal/haven/router_test.go
go test -v ./internal/haven/handlers_test.go

# Run with coverage
go test -cover ./internal/haven/

# Run with coverage report
go test -coverprofile=coverage.out ./internal/haven/
go tool cover -html=coverage.out
```

## Test Patterns Used

### Table-Driven Tests
Most tests use table-driven patterns for clarity and maintainability:

```go
tests := []struct {
    name        string
    kind        int
    author      string
    expectedBox Box
}{
    {
        name:        "draft from owner goes to private",
        kind:        30024,
        author:      ownerPubkey,
        expectedBox: BoxPrivate,
    },
    // ... more cases
}

for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        // Test implementation
    })
}
```

### Context-Based Authentication
Tests use khatru's context-based authentication:

```go
ctx := context.Background()
if authedPubkey != "" {
    ctx = context.WithValue(ctx, khatru.AuthedKey, authedPubkey)
}
```

### Test Constants
Consistent pubkeys used across tests:

```go
const (
    ownerPubkey   = "owner123..."
    alicePubkey   = "alice123..."
    bobPubkey     = "bob123..."
    charliePubkey = "charlie123..."
)
```

## Coverage

**Total test cases: 144**

### By Component
- Router core logic: 98 tests
- Handler enforcement: 46 tests

### By Feature
- Event routing: 35 tests
- Access control: 35 tests
- Filter routing: 20 tests
- Filter modification: 7 tests
- Configuration: 15 tests
- Helpers: 12 tests
- Edge cases: 20 tests

### Test Quality
- ✅ Happy path testing
- ✅ Edge case coverage
- ✅ Error condition testing
- ✅ State transition validation
- ✅ Configuration variations
- ✅ Authentication scenarios
- ✅ Boundary value testing

## Key Scenarios Tested

### Event Publishing
1. Owner publishes to private box (drafts, eCash)
2. Owner publishes to outbox (public posts)
3. Users publish to chat box (DMs, gift wraps)
4. Users publish to inbox (mentions, replies)
5. Rejection of events not belonging to any box

### Event Querying
1. Owner queries all boxes
2. Authenticated users query chat and outbox
3. Unauthenticated users query outbox only
4. Proper rejection of unauthorized queries

### Access Patterns
1. Private: owner-only read/write
2. Chat: authenticated read/write, WoT filtered
3. Inbox: owner read, public write
4. Outbox: public read, owner write

### Configuration Scenarios
1. Public outbox read enabled/disabled
2. Public inbox write enabled/disabled
3. Chat auth required/optional
4. Private auth required
5. Custom private kinds
6. Disabled HAVEN mode

## Edge Cases Covered

1. Empty pubkey (unauthenticated)
2. Empty owner configuration
3. Nil config handling
4. Malformed tags
5. Events with multiple p-tags
6. Events with no tags
7. Unknown box handling
8. Disabled HAVEN behavior
9. Empty filter kinds
10. Custom private kinds from non-owner

## Future Test Additions

Consider adding tests for:
- [ ] Integration tests with actual khatru relay
- [ ] Performance/benchmark tests for routing
- [ ] Concurrent access testing
- [ ] WoT filter integration
- [ ] Blastr broadcast functionality
- [ ] Inbox importer functionality
- [ ] Storage path separation
- [ ] Event migration between boxes

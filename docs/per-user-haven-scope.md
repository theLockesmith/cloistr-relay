# Per-User HAVEN Architecture Scope

## Overview

Transform HAVEN from single-owner to multi-user architecture where each authenticated user gets their own inbox/outbox/private/chat boxes with individual Blastr/Importer settings and per-user WoT preferences.

## Design Principles

From Cloistr's core philosophy:
- **Sovereignty**: Users control their data, trust settings, and can leave with everything
- **Potato-Grade Design**: Shared worker pools, not per-user pods
- **Tiered Access**: Free tier gets basic access, paid tiers unlock HAVEN features
- **User-Level WoT**: Users decide who they trust, relay sets floor

## Current Architecture (Single-Owner)

```
Config:
  OwnerPubkey: "abc123..."
  BlastrRelays: [relay1, relay2]
  ImporterRelays: [relay3, relay4]

Routing:
  event.PubKey == OwnerPubkey → Outbox
  p-tag contains OwnerPubkey → Inbox
  Private kinds from OwnerPubkey → Private
  Chat kinds → Chat
  Everything else → Rejected
```

## Proposed Architecture (Per-User)

```
Membership (NIP-43):
  Member { Pubkey, Tier, JoinedAt }

UserSettings (per pubkey):
  Tier: free | hybrid | premium | enterprise
  BlastrEnabled: bool (tier-gated)
  BlastrRelays: []string (tier-limited count)
  ImporterEnabled: bool (tier-gated)
  ImporterRelays: []string (tier-limited count)
  WoTSettings: UserWoTSettings

Routing (context-aware):
  event.PubKey == AuthedUser → That user's Outbox
  p-tag contains RegisteredUser → That user's Inbox
  Private kinds from AuthedUser → That user's Private
  Chat kinds involving AuthedUser → That user's Chat
```

## Tiered Feature Model

Integration with existing NIP-43 membership + business model tiers:

| Tier | HAVEN Boxes | Blastr | Importer | WoT Control | Relay Limit |
|------|-------------|--------|----------|-------------|-------------|
| **Free** | No | No | No | Relay default only | N/A |
| **Hybrid** | Yes | Yes | Yes | User overrides | 3 relays |
| **Premium** | Yes | Yes | Yes | Full control | 10 relays |
| **Enterprise** | Yes | Yes | Yes | Full + custom | Unlimited |

```go
type MemberTier string

const (
    TierFree       MemberTier = "free"
    TierHybrid     MemberTier = "hybrid"
    TierPremium    MemberTier = "premium"
    TierEnterprise MemberTier = "enterprise"
)

type TierLimits struct {
    HasHavenBoxes     bool
    HasBlastr         bool
    HasImporter       bool
    HasWoTControl     bool
    MaxBlastrRelays   int  // 0 = unlimited
    MaxImporterRelays int
}

var TierConfig = map[MemberTier]TierLimits{
    TierFree:       {false, false, false, false, 0, 0},
    TierHybrid:     {true, true, true, true, 3, 3},
    TierPremium:    {true, true, true, true, 10, 10},
    TierEnterprise: {true, true, true, true, 0, 0},
}
```

## Per-User WoT Architecture

WoT operates at two levels:

### Relay-Level WoT (Floor)
- Operator-configured baseline trust
- Global blocklist/allowlist
- Minimum trust requirements for all users

### User-Level WoT (Per-Inbox)
- User's personal trust anchor (their own pubkey)
- User's blocklist (never reaches their inbox)
- User's trusted list (always allowed to their inbox)
- User can be stricter than relay, never more permissive

```go
type UserWoTSettings struct {
    Pubkey string

    // User's trust anchor (typically themselves)
    TrustAnchor string

    // Trust depth (how many hops from anchor)
    // nil = use relay default
    MaxTrustDepth *int

    // Personal blocklist - never reaches this user's inbox
    BlockedPubkeys []string

    // Personal trusted list - always reaches inbox (within relay floor)
    TrustedPubkeys []string

    // Minimum PoW for unknown senders to this user's inbox
    // nil = use relay default
    MinPowBits *int
}
```

### Filter Stack

```
Event arrives
    │
    ▼
┌─────────────────────────────┐
│ Relay WoT (Floor)           │
│ - Global blocklist check    │
│ - Minimum trust/PoW         │
└─────────────┬───────────────┘
              │ Pass
              ▼
┌─────────────────────────────┐
│ HAVEN Routing               │
│ - Determine target user     │
│ - Identify box              │
└─────────────┬───────────────┘
              │ Routed to user's inbox
              ▼
┌─────────────────────────────┐
│ User WoT (if inbox/chat)    │
│ - User's blocklist check    │
│ - User's trust preferences  │
└─────────────┬───────────────┘
              │ Pass
              ▼
         Store event
```

## Shared Worker Pool Architecture

**Critical: No per-user pods or goroutines. Shared pools process tagged jobs.**

### Blastr (Shared Pool)

```go
type BlastrManager struct {
    userStore   UserSettingsStore
    relayPool   *RelayPool           // Shared connections
    jobQueue    chan BlastrJob       // Single queue
    workerCount int                  // Fixed pool (e.g., 10)
    wg          sync.WaitGroup
}

type BlastrJob struct {
    Event       *nostr.Event
    UserPubkey  string    // Whose outbox this is
    Relays      []string  // That user's configured relays
    Priority    int       // Tier-based priority
}

func (m *BlastrManager) Start() {
    // Fixed number of workers, NOT per-user
    for i := 0; i < m.workerCount; i++ {
        m.wg.Add(1)
        go m.worker(i)
    }
}

func (m *BlastrManager) worker(id int) {
    defer m.wg.Done()
    for job := range m.jobQueue {
        // Any worker processes any user's job
        m.broadcastEvent(job)
    }
}

func (m *BlastrManager) OnEventSaved(ctx context.Context, event *nostr.Event) {
    authedPubkey := khatru.GetAuthed(ctx)
    if authedPubkey == "" || event.PubKey != authedPubkey {
        return
    }

    settings := m.userStore.GetSettings(authedPubkey)
    if !settings.BlastrEnabled || len(settings.BlastrRelays) == 0 {
        return
    }

    // Check tier limits
    tier := m.userStore.GetTier(authedPubkey)
    if !TierConfig[tier].HasBlastr {
        return
    }

    m.jobQueue <- BlastrJob{
        Event:      event,
        UserPubkey: authedPubkey,
        Relays:     settings.BlastrRelays,
        Priority:   tierPriority(tier),
    }
}
```

### Importer (Shared Pool + Scheduled Jobs)

```go
type ImporterManager struct {
    userStore   UserSettingsStore
    relayPool   *RelayPool
    storeFunc   func(context.Context, *nostr.Event) error
    jobQueue    chan ImporterJob
    workerCount int                  // Fixed pool (e.g., 5)
    wg          sync.WaitGroup
}

type ImporterJob struct {
    UserPubkey  string
    Relays      []string
    Since       time.Time
}

func (m *ImporterManager) Start() {
    // Fixed worker pool
    for i := 0; i < m.workerCount; i++ {
        m.wg.Add(1)
        go m.worker(i)
    }

    // Scheduler enqueues jobs for all users periodically
    go m.scheduler()
}

func (m *ImporterManager) scheduler() {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        users := m.userStore.GetUsersWithImporter()
        for _, user := range users {
            // Check tier allows importer
            tier := m.userStore.GetTier(user.Pubkey)
            if !TierConfig[tier].HasImporter {
                continue
            }

            m.jobQueue <- ImporterJob{
                UserPubkey: user.Pubkey,
                Relays:     user.ImporterRelays,
                Since:      user.LastImportTime,
            }
        }
    }
}

func (m *ImporterManager) worker(id int) {
    defer m.wg.Done()
    for job := range m.jobQueue {
        m.importForUser(job)
    }
}
```

## Database Schema

### Extended Membership Table (NIP-43 + Tiers)

```sql
-- Extends existing NIP-43 membership
ALTER TABLE members ADD COLUMN tier TEXT DEFAULT 'free';
ALTER TABLE members ADD COLUMN tier_expires_at TIMESTAMPTZ;
ALTER TABLE members ADD COLUMN lightning_address TEXT;

CREATE INDEX idx_members_tier ON members(tier);
```

### User HAVEN Settings

```sql
CREATE TABLE haven_user_settings (
    pubkey TEXT PRIMARY KEY REFERENCES members(pubkey) ON DELETE CASCADE,

    -- Blastr settings (tier-gated)
    blastr_enabled BOOLEAN DEFAULT false,
    blastr_relays TEXT[] DEFAULT '{}',

    -- Importer settings (tier-gated)
    importer_enabled BOOLEAN DEFAULT false,
    importer_relays TEXT[] DEFAULT '{}',
    importer_realtime BOOLEAN DEFAULT false,
    last_import_time TIMESTAMPTZ,

    -- Privacy settings
    public_outbox_read BOOLEAN DEFAULT true,
    public_inbox_write BOOLEAN DEFAULT true,
    require_auth_chat BOOLEAN DEFAULT true,
    require_auth_private BOOLEAN DEFAULT true,

    -- Metadata
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
```

### User WoT Settings

```sql
CREATE TABLE wot_user_settings (
    pubkey TEXT PRIMARY KEY REFERENCES members(pubkey) ON DELETE CASCADE,

    -- Trust anchor (typically their own pubkey)
    trust_anchor TEXT,

    -- Override relay defaults (NULL = use relay default)
    max_trust_depth INT,
    min_pow_bits INT,

    -- Personal lists
    blocked_pubkeys TEXT[] DEFAULT '{}',
    trusted_pubkeys TEXT[] DEFAULT '{}',

    -- Metadata
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_wot_blocked ON wot_user_settings USING GIN(blocked_pubkeys);
CREATE INDEX idx_wot_trusted ON wot_user_settings USING GIN(trusted_pubkeys);
```

## Router Refactoring

```go
type Router struct {
    memberStore  MemberStore
    settingsStore UserSettingsStore
    wotStore     WoTUserSettingsStore
}

type RoutingResult struct {
    Box         Box
    OwnerPubkey string  // Whose box this belongs to
    Tier        MemberTier
}

func (r *Router) RouteEvent(event *nostr.Event, authedPubkey string) RoutingResult {
    // Events from authenticated member go to their outbox
    if authedPubkey != "" && event.PubKey == authedPubkey {
        if member := r.memberStore.GetMember(authedPubkey); member != nil {
            // Check if tier allows HAVEN boxes
            if TierConfig[member.Tier].HasHavenBoxes {
                return RoutingResult{
                    Box:         BoxOutbox,
                    OwnerPubkey: authedPubkey,
                    Tier:        member.Tier,
                }
            }
        }
    }

    // Events p-tagged to members go to their inbox
    for _, tag := range event.Tags {
        if len(tag) >= 2 && tag[0] == "p" {
            targetPubkey := tag[1]
            if member := r.memberStore.GetMember(targetPubkey); member != nil {
                if TierConfig[member.Tier].HasHavenBoxes {
                    return RoutingResult{
                        Box:         BoxInbox,
                        OwnerPubkey: targetPubkey,
                        Tier:        member.Tier,
                    }
                }
            }
        }
    }

    // Not addressed to any member with HAVEN access
    return RoutingResult{Box: BoxUnknown, OwnerPubkey: ""}
}
```

## User Settings via NIP-78

Users publish their settings as signed events (portable, sovereign):

```json
{
  "kind": 30078,
  "tags": [["d", "cloistr-haven-settings"]],
  "content": "{
    \"blastr_enabled\": true,
    \"blastr_relays\": [\"wss://relay.damus.io\", \"wss://nos.lol\"],
    \"importer_enabled\": true,
    \"importer_relays\": [\"wss://relay.nostr.band\"],
    \"wot\": {
      \"blocked_pubkeys\": [\"abc...\", \"def...\"],
      \"trusted_pubkeys\": [\"ghi...\"],
      \"max_trust_depth\": 2
    }
  }"
}
```

Relay watches for these events and syncs to database. User exports = signed event they already have.

## Access Control Matrix

| Box | Read (owner) | Read (other) | Write (owner) | Write (other) |
|-----|--------------|--------------|---------------|---------------|
| Outbox | Yes | Per-user setting | Yes | No |
| Inbox | Yes | No | No | Per-user setting + User WoT |
| Private | Yes | No | Yes | No |
| Chat | Yes | With auth + User WoT | Yes | With auth + User WoT |

## Migration Path

### Phase 1: Tier Infrastructure
- Extend NIP-43 members table with tier column
- Add tier enforcement to existing handlers
- Lightning payment integration for tier upgrades

### Phase 2: Per-User WoT
- Add wot_user_settings table
- Implement user WoT filter layer
- Admin UI for user WoT management
- NIP-78 settings watcher for WoT

### Phase 3: Per-User HAVEN Routing
- Refactor Router to be context-aware
- Add haven_user_settings table
- Update Handler to use per-user routing
- Virtual box assignment

### Phase 4: Shared Worker Blastr
- Implement BlastrManager with shared pool
- Tier-based feature gating
- Tier-based relay limits
- Per-user metrics

### Phase 5: Shared Worker Importer
- Implement ImporterManager with scheduler + shared pool
- Tier-based feature gating
- Per-user metrics

### Phase 6: User Self-Service
- User-facing settings UI (via client app)
- NIP-78 settings publication
- Full export functionality (events + settings)

## Resource Scaling

| Users | Blastr Workers | Importer Workers | Memory | Notes |
|-------|----------------|------------------|--------|-------|
| 100 | 5 | 3 | ~100MB | Minimal load |
| 1,000 | 10 | 5 | ~200MB | Comfortable |
| 10,000 | 20 | 10 | ~500MB | May need queue depth tuning |
| 100,000 | 50 | 25 | ~2GB | Consider sharding by user range |

Workers are goroutines, not pods. Job queue handles burst. Relay connections pooled.

## Metrics

```
# Tier distribution
haven_members_by_tier{tier="free|hybrid|premium|enterprise"}

# Feature usage
haven_blastr_jobs_total{tier="..."}
haven_importer_jobs_total{tier="..."}

# User WoT
haven_wot_blocks_total{} - Events blocked by user WoT
haven_wot_allows_total{} - Events allowed by user trusted list

# Worker health
haven_blastr_queue_depth{}
haven_importer_queue_depth{}
haven_worker_processing_seconds{type="blastr|importer"}
```

## B2B Model

### Organization Structure

Businesses become "organization owners" who can manage members under their account:

```go
type Organization struct {
    ID              string
    Name            string
    OwnerPubkey     string      // Admin who manages the org
    Tier            MemberTier  // Org-wide tier (enterprise)
    MemberLimit     int         // Max members (0 = unlimited)
    LightningAddr   string      // Billing address
    CreatedAt       time.Time
}

type OrgMember struct {
    OrgID           string
    Pubkey          string
    Role            OrgRole     // admin | member
    InheritsTier    bool        // Uses org tier vs personal
    JoinedAt        time.Time
}

type OrgRole string
const (
    OrgRoleAdmin  OrgRole = "admin"   // Can manage members, settings
    OrgRoleMember OrgRole = "member"  // Uses org features
)
```

### B2B Tiers

| Model | Description | Billing |
|-------|-------------|---------|
| **Self-Hosted** | Customer runs their own relay instance | License fee (BTC) |
| **Managed Relay** | Coldforge hosts dedicated relay for org | Monthly BTC subscription |
| **Shared Enterprise** | Org gets enterprise tier on shared relay | Per-seat BTC pricing |

### Shared Enterprise (Multi-Tenant B2B)

Organizations on shared relay.cloistr.xyz:

```sql
CREATE TABLE organizations (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    owner_pubkey TEXT NOT NULL,
    tier TEXT DEFAULT 'enterprise',
    member_limit INT DEFAULT 0,  -- 0 = unlimited
    lightning_address TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE org_members (
    org_id TEXT REFERENCES organizations(id) ON DELETE CASCADE,
    pubkey TEXT REFERENCES members(pubkey) ON DELETE CASCADE,
    role TEXT DEFAULT 'member',
    inherits_tier BOOLEAN DEFAULT true,
    joined_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (org_id, pubkey)
);

CREATE INDEX idx_org_members_pubkey ON org_members(pubkey);
```

### B2B Feature Inheritance

```go
func (r *Router) GetEffectiveTier(pubkey string) MemberTier {
    // Check if user belongs to an org
    if orgMember := r.orgStore.GetOrgMembership(pubkey); orgMember != nil {
        if orgMember.InheritsTier {
            org := r.orgStore.GetOrg(orgMember.OrgID)
            return org.Tier  // Enterprise tier from org
        }
    }

    // Fall back to personal tier
    member := r.memberStore.GetMember(pubkey)
    if member == nil {
        return TierFree
    }
    return member.Tier
}
```

### B2B Admin Capabilities

Org admins can:
- Add/remove org members
- Set org-wide WoT defaults (members can override stricter)
- View org-wide usage metrics
- Manage org billing (Lightning)
- Export all org member data

### B2B Routing

Events between org members could optionally stay org-internal:

```go
type OrgSettings struct {
    OrgID               string
    InternalRelayOnly   bool    // Events stay on this relay
    SharedOutbox        bool    // Org members see each other's outbox
    OrgWoTBaseline      UserWoTSettings  // Org-wide trust defaults
}
```

### White-Label / Self-Hosted

For customers running their own instance:

1. **Open Source Core**: cloistr-relay is open source, self-hostable
2. **Enterprise License**: Paid license for commercial use, support
3. **Branding**: Customer configures their own domain, NIP-11 info
4. **Updates**: Customer pulls updates, manages their infra

```yaml
# Customer's config
relay_name: "Acme Corp Relay"
relay_url: "wss://relay.acme.com"
haven_enabled: true
# All users are Acme employees, org manages membership
```

## Resolved Questions

1. **User Registration**: Via NIP-43 membership (existing). Free tier = default.

2. **Resource Limits**: Tier-based. Free=0, Hybrid=3, Premium=10, Enterprise=unlimited.

3. **Unroutable Events**: Accept to community pool. HAVEN boxes are opt-in tier feature, not relay-wide rejection.

4. **User Departure**: Export via NIP-78 settings + standard event query. Delete on request (GDPR-style).

5. **WoT Control**: User-level for paid tiers. Relay floor always applies. Users raise the bar, never lower it.

6. **B2B Model**: Organizations get enterprise tier, members inherit. Org admins manage membership. Billing via Lightning to org address.

---

## Implementation Status

**Last Updated:** 2026-03-30

### Completed Phases

| Phase | Status | Commit | Files |
|-------|--------|--------|-------|
| **Phase 1: Tier Infrastructure** | ✅ Done | 52771a0 | `types.go` (MemberStore, MemberInfo, TierLimits) |
| **Phase 2: Per-User WoT** | ✅ Done | ff47462 | `internal/wot/user_settings.go`, `settings_watcher.go` |
| **Phase 3: Per-User Routing** | ✅ Done | 22ce08e | `router.go` (RoutingResult, RouteEventForUser), `user_settings.go` |
| **Phase 4: BlastrManager** | ✅ Done | 16441b1 | `blastr_manager.go` (shared worker pool) |
| **Phase 5: ImporterManager** | ✅ Done | 4f17e54 | `importer_manager.go` (scheduler + pool) |
| **Phase 6: User Self-Service** | ✅ Done | c0c392f | `settings_watcher.go` (NIP-78 HAVEN settings) |
| **Integration Wiring** | ✅ Done | 527bf82 | `cmd/relay/main.go` (HAVEN_MULTI_USER mode) |
| **B2B Organizations** | ✅ Done | 6489a2f | `organization.go` (OrgStore, OrgMember) |

### What's Wired

- `HAVEN_MULTI_USER=true` enables multi-tenant mode
- `membership.Store` implements `haven.MemberStore` for tier lookups
- `UserSettingsStore.Init()` creates `haven_user_settings` table
- `wot.UserSettingsStore.Init()` creates `wot_user_settings` table
- `OrgStore.Init()` creates `organizations`, `org_members`, `org_settings` tables
- `BlastrManager` registered on `OnEventSaved` hook
- `ImporterManager` scheduler running on startup
- `HavenSettingsWatcher` syncs NIP-78 events to database
- `wot.SettingsWatcher` syncs NIP-78 WoT events to database

### Handler Integration (Phase 7 - Complete)

The per-user routing is now wired into khatru handlers via `MultiUserHandler`:

| Component | Purpose | Status |
|-----------|---------|--------|
| `MultiUserHandler` | Per-user event/filter routing | **Done** |
| `RegisterMultiUserHandlers()` | Register handlers with khatru | **Done** |
| `WoTUserFilter` interface | Per-user WoT filtering in handlers | **Done** |
| `wotUserFilterAdapter` | Adapts wot.UserFilter to haven interface | **Done** |

When `HAVEN_MULTI_USER=true`:
1. `RouteEventForUser()` routes events to per-user boxes based on authenticated pubkey
2. `CanAccessUserBox()` checks per-user privacy settings for read/write access
3. Per-user WoT filter blocks events based on recipient's blocklist/trusted list
4. Per-user Blastr/Importer work via shared worker pools

**Note:** Single-owner HAVEN (`HAVEN_ENABLED=true` with `HAVEN_OWNER_PUBKEY`) and multi-user HAVEN are mutually exclusive - multi-user takes precedence.

### Database Tables Created

```sql
-- Per-user HAVEN settings
CREATE TABLE haven_user_settings (...)

-- Per-user WoT settings
CREATE TABLE wot_user_settings (...)

-- B2B organizations
CREATE TABLE organizations (...)
CREATE TABLE org_members (...)
CREATE TABLE org_settings (...)
```

### Configuration

Enable per-user mode:
```bash
HAVEN_MULTI_USER=true
```

This runs alongside single-owner HAVEN - they are not mutually exclusive.

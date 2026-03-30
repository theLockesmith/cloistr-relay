# coldforge-relay Reference

**Comprehensive reference documentation for the Nostr relay.**

For quick start and essential info, see [CLAUDE.md](../CLAUDE.md).

---

## Full Configuration

### Core Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `RELAY_PORT` | 3334 | Port |
| `RELAY_NAME` | - | Relay name |
| `AUTH_POLICY` | open | open, auth-read, auth-write, auth-all |
| `ALLOWED_PUBKEYS` | - | Comma-separated whitelist |
| `DB_HOST/PORT/NAME/USER/PASSWORD` | - | PostgreSQL connection |

### Event Validation (NIP-22/13)

| Variable | Default | Description |
|----------|---------|-------------|
| `MAX_CREATED_AT_FUTURE` | 300 | Max seconds into future |
| `MAX_CREATED_AT_PAST` | 0 | Max seconds into past (0 = unlimited) |
| `MIN_POW_DIFFICULTY` | 0 | Required PoW difficulty (0 = disabled) |

### Rate Limiting

| Variable | Default | Description |
|----------|---------|-------------|
| `RATE_LIMIT_EVENTS_PER_SEC` | 10 | Events per second per IP |
| `RATE_LIMIT_FILTERS_PER_SEC` | 20 | Queries per second per IP |
| `RATE_LIMIT_CONNECTIONS_PER_SEC` | 5 | Connections per second per IP |
| `RATE_LIMIT_EXEMPT_KINDS` | - | Comma-separated kinds exempt from rate limiting |
| `RATE_LIMIT_EXEMPT_PUBKEYS` | - | Comma-separated pubkeys exempt from all rate limiting |
| `RATE_LIMIT_DISTRIBUTED` | false | Use Dragonfly for distributed rate limiting |

### Web of Trust

| Variable | Default | Description |
|----------|---------|-------------|
| `WOT_ENABLED` | false | Enable WoT filtering |
| `WOT_OWNER_PUBKEY` | - | Relay owner pubkey (trust level 0) |
| `WOT_UNKNOWN_POW_BITS` | 8 | PoW bits required for unknown pubkeys |
| `WOT_UNKNOWN_RATE_LIMIT` | 5 | Events/sec for unknown pubkeys |
| `WOT_USE_PAGERANK` | false | Use PageRank-based trust (requires cache) |
| `WOT_PAGERANK_INTERVAL` | 60 | PageRank recompute interval in minutes |

### Cache (Dragonfly/Redis)

| Variable | Default | Description |
|----------|---------|-------------|
| `CACHE_URL` | - | Redis/Dragonfly URL (e.g., redis://dragonfly:6379) |
| `WRITE_AHEAD_ENABLED` | false | Enable write-ahead log |
| `EVENT_CACHE_ENABLED` | false | Enable hot event caching |
| `SESSION_STORE_ENABLED` | false | Enable distributed session state |

### Admin & Management

| Variable | Default | Description |
|----------|---------|-------------|
| `ADMIN_PUBKEYS` | - | NIP-86: Comma-separated pubkeys for management API |
| `PPROF_ENABLED` | false | Enable pprof endpoints at /debug/pprof/ |
| `LOG_LEVEL` | info | debug, info, warn, error |
| `LOG_FORMAT` | json | json, text |

### Database Pool

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_MAX_OPEN_CONNS` | 25 | Max open connections |
| `DB_MAX_IDLE_CONNS` | 10 | Max idle connections |
| `DB_CONN_MAX_LIFETIME` | 5m | Max connection lifetime |
| `DB_CONN_MAX_IDLE_TIME` | 1m | Max idle time before closing |

### NIP-59 Gift Wrap

| Variable | Default | Description |
|----------|---------|-------------|
| `GIFTWRAP_ENABLED` | true | Enable gift wrap support |
| `GIFTWRAP_REQUIRE_AUTH` | true | Require auth to query gift wraps |

### NIP-57 Zaps

| Variable | Default | Description |
|----------|---------|-------------|
| `ZAPS_ENABLED` | true | Enable zap receipt support |
| `ZAPS_VALIDATE_RECEIPT` | true | Validate zap receipt structure |

### NIP-70 Protected Events

| Variable | Default | Description |
|----------|---------|-------------|
| `PROTECTED_EVENTS_ENABLED` | true | Enable protected event handling |
| `PROTECTED_EVENTS_ALLOW` | true | Allow protected events from authenticated authors |

### NIP-66 Relay Monitoring

| Variable | Default | Description |
|----------|---------|-------------|
| `NIP66_ENABLED` | false | Enable relay discovery support |
| `NIP66_SELF_MONITOR` | false | Enable self-monitoring |
| `NIP66_MONITOR_KEY` | - | Private key for signing monitor events |

---

## HAVEN Box Routing

Reference: [bitvora/haven](https://github.com/bitvora/haven) - "High Availability Vault for Events on Nostr"

HAVEN implements the Outbox Model with four relay types in one.

### The Four Boxes

| Box | Purpose | Access | Event Kinds |
|-----|---------|--------|-------------|
| **Private** | Drafts, eCash, personal notes | Owner only (auth required) | 30024, 31234, 7375, 7376, 30078, 10003, 30003 |
| **Chat** | DMs and group chats | WoT-filtered (auth required) | 4, 13, 14, 15, 1059, 1060 (NIP-04, NIP-17, NIP-59) |
| **Inbox** | Events addressed to owner | Public write, owner read | 1, 6, 7, 9735, 1111, 30023 (when tagged) |
| **Outbox** | Owner's public notes | Owner write, public read | 0, 1, 3, 6, 7, 10002, 10050, 30023 |

### HAVEN Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `HAVEN_ENABLED` | false | Enable HAVEN box routing |
| `HAVEN_OWNER_PUBKEY` | - | Owner pubkey (required if enabled) |
| `HAVEN_PRIVATE_KINDS` | - | Additional kinds for private box |
| `HAVEN_ALLOW_PUBLIC_OUTBOX_READ` | true | Public can read owner's posts |
| `HAVEN_ALLOW_PUBLIC_INBOX_WRITE` | true | Anyone can tag/mention owner |
| `HAVEN_REQUIRE_AUTH_FOR_CHAT` | true | DMs require authentication |
| `HAVEN_REQUIRE_AUTH_FOR_PRIVATE` | true | Private box requires authentication |

### Blastr (Outbox Broadcasting)

| Variable | Default | Description |
|----------|---------|-------------|
| `HAVEN_BLASTR_ENABLED` | false | Enable outbox broadcasting |
| `HAVEN_BLASTR_RELAYS` | - | Relays to broadcast to (comma-separated) |
| `HAVEN_BLASTR_RETRY_ENABLED` | false | Enable persistent retry queue |
| `HAVEN_BLASTR_MAX_RETRIES` | 6 | Maximum retry attempts |
| `HAVEN_BLASTR_RETRY_INTERVAL` | 30 | Retry interval in seconds |

### Importer (Inbox Fetching)

| Variable | Default | Description |
|----------|---------|-------------|
| `HAVEN_IMPORTER_ENABLED` | false | Enable inbox event fetching |
| `HAVEN_IMPORTER_RELAYS` | - | Relays to fetch from (comma-separated) |
| `HAVEN_IMPORTER_REALTIME` | false | Enable real-time WebSocket subscriptions |

### Multi-User Mode (Per-User HAVEN)

| Variable | Default | Description |
|----------|---------|-------------|
| `HAVEN_MULTI_USER` | false | Enable per-user HAVEN with shared worker pools |

When enabled, initializes:
- **UserSettingsStore** - Per-user Blastr/Importer settings (NIP-78 synced)
- **WoT UserSettingsStore** - Per-user blocklists and trusted lists
- **BlastrManager** - Shared worker pool for per-user broadcasting
- **ImporterManager** - Scheduler + shared pool for per-user inbox import
- **OrgStore** - B2B organization management

Users manage settings via NIP-78 events (kind 30078, d-tag: `cloistr-haven-settings`).

**Tier-based feature gating:**

| Tier | HAVEN Boxes | Blastr | Importer | WoT Control | Relay Limit |
|------|-------------|--------|----------|-------------|-------------|
| free | No | No | No | Relay default | N/A |
| hybrid | Yes | Yes | Yes | User overrides | 3 |
| premium | Yes | Yes | Yes | Full control | 10 |
| enterprise | Yes | Yes | Yes | Full + custom | Unlimited |

### E-tag Routing

When a reaction (kind 7) or repost (kind 6) arrives without a p-tag to the owner:
1. Router looks up referenced event(s) in e-tags
2. If any referenced event was authored by owner, routes to inbox
3. Allows receiving reactions/reposts to owner's content from other relays

### HAVEN Architecture

```
internal/haven/
├── types.go              # Box types, default kinds, config, MemberStore interface
├── router.go             # Event/filter routing (single + multi-user)
├── handlers.go           # RejectEvent, RejectFilter, HavenSystem
├── blastr.go             # Single-owner outbox broadcasting
├── importer.go           # Single-owner inbox fetching
├── blastr_manager.go     # Per-user Blastr with shared worker pool
├── importer_manager.go   # Per-user Importer with scheduler
├── user_settings.go      # UserSettingsStore for per-user HAVEN settings
├── settings_watcher.go   # NIP-78 settings watcher (kind 30078)
├── organization.go       # B2B: Organization, OrgMember, OrgStore
├── metrics.go            # Prometheus metrics (single + per-tier)
└── *_test.go             # Tests
```

### HAVEN Prometheus Metrics

**Box Routing:**
- `nostr_relay_haven_events_routed_total{box}`
- `nostr_relay_haven_events_rejected_total{box,reason}`
- `nostr_relay_haven_filters_routed_total{box}`
- `nostr_relay_haven_access_denied_total{box,operation,reason}`

**Blastr:**
- `nostr_relay_haven_blastr_events_broadcast_total`
- `nostr_relay_haven_blastr_events_failed_total`
- `nostr_relay_haven_blastr_relays_connected` (gauge)
- `nostr_relay_haven_blastr_retry_queue_size` (gauge)

**Importer:**
- `nostr_relay_haven_importer_events_imported_total`
- `nostr_relay_haven_importer_events_skipped_total`
- `nostr_relay_haven_importer_last_poll_timestamp` (gauge)

---

## RSS/Atom Feeds

### Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `FEED_ENABLED` | auto | Auto-enabled when HAVEN is enabled |
| `FEED_LIMIT` | 20 | Default items in feeds |
| `FEED_MAX_LIMIT` | 100 | Maximum via ?limit= parameter |
| `FEED_INCLUDE_LONG_FORM` | true | Include kind 30023 articles |
| `FEED_INCLUDE_REPLIES` | false | Exclude replies by default |

### Endpoints

- `/feed/rss` or `/feed/rss.xml` - RSS 2.0 feed
- `/feed/atom` or `/feed/atom.xml` - Atom 1.0 feed

### Architecture

```
internal/feeds/
├── types.go         # Config, FeedItem, FeedMetadata
├── handler.go       # HTTP handlers, event-to-feed conversion
├── rss.go           # RSS 2.0 XML generation
├── atom.go          # Atom 1.0 XML generation
└── feeds_test.go    # Tests
```

---

## Algorithmic Feeds

### Available Algorithms

| Algorithm | Description | Scoring |
|-----------|-------------|---------|
| `chronological` | Default - no ranking | Timestamp only |
| `wot-ranked` | Rank by Web of Trust | 70% WoT + 30% recency |
| `engagement` | Rank by reactions/reposts/zaps | 70% engagement + 30% recency |
| `trending` | Balance of all factors | Configurable weights |
| `personalized` | User preference boosting | Base score + personal boosts |

### Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `ALGO_ENABLED` | false | Enable algorithmic feeds (opt-in) |
| `ALGO_DEFAULT` | chronological | Default algorithm |
| `ALGO_WOT_WEIGHT` | 0.3 | Weight for WoT in trending |
| `ALGO_ENGAGEMENT_WEIGHT` | 0.4 | Weight for engagement in trending |
| `ALGO_RECENCY_WEIGHT` | 0.3 | Weight for recency in trending |

### Usage

Via REQ filter: `["REQ", "feed", {"kinds": [1], "#algo": ["wot-ranked"]}]`

Via feed URL: `/feed/rss?algo=trending&limit=50`

### Engagement Scoring

- Reactions (kind 7): 1 point
- Reposts (kind 6): 3 points
- Replies: 2 points
- Zaps (kind 9735): 5 points + log bonus for amount

---

## NIP-29 Relay-based Groups

Closed-membership group communication hosted on the relay.

### Event Kinds

| Kind | Name | Description |
|------|------|-------------|
| 9000 | Add User | Add user to group with optional role |
| 9001 | Remove User | Remove user from group |
| 9002 | Edit Metadata | Update group name, picture, about |
| 9005 | Delete Event | Delete an event from group |
| 9007 | Create Group | Create a new group |
| 9008 | Delete Group | Delete a group |
| 9009 | Create Invite | Create an invite code |
| 9021 | Join Request | Request to join a group |
| 9022 | Leave Request | Request to leave a group |
| 39000 | Group Metadata | Relay-published group info |
| 39001 | Group Admins | Admin list with roles |
| 39002 | Group Members | Member list |
| 39003 | Group Roles | Supported roles |

### Privacy Levels

| Privacy | Can Read | Can Write | Can Join | Show Metadata |
|---------|----------|-----------|----------|---------------|
| `open` | Anyone | Anyone | Yes | Yes |
| `restricted` | Anyone | Members only | Yes | Yes |
| `private` | Members only | Members only | Yes | Yes |
| `hidden` | Members only | Members only | Yes | Members only |
| `closed` | Members only | Members only | No | Yes |

### Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `GROUPS_ENABLED` | false | Enable NIP-29 groups |
| `GROUPS_ADMIN_PUBKEYS` | - | Can always create groups |
| `GROUPS_ALLOW_PUBLIC_CREATION` | false | Allow anyone to create |
| `GROUPS_MAX_PER_USER` | 10 | Max groups per user (0 = unlimited) |
| `GROUPS_DEFAULT_PRIVACY` | restricted | Default for new groups |
| `GROUPS_INVITE_EXPIRY_HOURS` | 168 | Invite expiry (1 week) |

### Database Schema

- `nip29_groups` - Group metadata
- `nip29_members` - Group memberships
- `nip29_invites` - Invite codes

---

## NIP-17 Private Direct Messages

Modern encrypted DMs replacing deprecated NIP-04.

### Three-Layer Encryption

1. **Rumor (kind 14)**: Unsigned chat message with content and recipient p-tags
2. **Seal (kind 13)**: Wraps rumor, signed with sender's key
3. **Gift Wrap (kind 1059)**: Wraps seal with ephemeral key, hiding all metadata

### Event Kinds

| Kind | Name | Description |
|------|------|-------------|
| 14 | Chat Message | Plain text DM with p-tags |
| 15 | File Message | Encrypted file metadata and URLs |
| 10050 | DM Relay List | User's preferred DM relays (replaceable) |

### Privacy Comparison

| Feature | NIP-04 | NIP-17 |
|---------|--------|--------|
| Sender visible | Yes | No (ephemeral key) |
| Recipient visible | Yes | No (auth required) |
| Timestamp visible | Yes | Randomized |
| Encryption | AES-256-CBC | NIP-44 (ChaCha20) |
| Deniability | No | Yes (unsigned rumors) |

---

## NIP-51 User Lists

### Event Kinds

| Kind | Name | Privacy | Description |
|------|------|---------|-------------|
| 10000 | Mute List | Private | Muted pubkeys, events, hashtags, words |
| 10001 | Pin List | Public | Pinned/highlighted events |
| 10002 | Relay List | Public | NIP-65 relay preferences |
| 10003 | Bookmark List | Private | Bookmarked events |
| 10004 | Communities List | Public | Communities user follows |
| 10005 | Public Chats List | Public | Group chats user is in |
| 10006 | Blocked Relays | Private | Relays to avoid |
| 10007 | Search Relays | Public | Preferred NIP-50 relays |
| 30000 | People Sets | Private | Categorized pubkey lists |
| 30002 | Relay Sets | Public | Categorized relay groups |

### HAVEN Box Routing

**Private Box:** 10000, 10003, 10006, 30000, 30001, 30003
**Outbox:** 10001, 10002, 10004, 10005, 10007, 10015, 10030, 30002

---

## NIP-32 Content Labeling

### Label Event Structure (kind 1985)

```json
{
  "kind": 1985,
  "tags": [
    ["L", "ugc"],
    ["l", "spam", "ugc"],
    ["e", "abc123", "wss://relay.example.com"]
  ]
}
```

### Common Namespaces

| Namespace | Purpose |
|-----------|---------|
| `ugc` | User-generated content labels |
| `relay/moderation` | Relay admin moderation |
| `content-warning` | Content warnings |
| `ISO-639-1` | Language codes |

### Common Labels

spam, nsfw, adult, gore, abuse, illegal, impersonation, bot

---

## NIP-43 Relay Access & Membership

### Event Kinds

| Kind | Name | Description |
|------|------|-------------|
| 13534 | Membership List | Relay-published list of members |
| 8000 | Add Member | Join notification |
| 8001 | Remove Member | Leave notification |
| 28934 | Join Request | User request with optional invite |
| 28935 | Invite Response | Ephemeral invite code response |
| 28936 | Leave Request | User request to leave |

### Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `MEMBERSHIP_ENABLED` | false | Enable NIP-43 membership |
| `MEMBERSHIP_RELAY_PRIVATE_KEY` | - | For signing (required) |
| `MEMBERSHIP_REQUIRE_MEMBERSHIP` | false | Require membership to access |
| `MEMBERSHIP_ALLOW_JOIN_REQUESTS` | true | Allow self-service joins |
| `MEMBERSHIP_PUBLISH_LIST` | false | Publish member list publicly |

---

## NIP-72 Moderated Communities

Reddit-style public communities with moderation.

### Event Kinds

| Kind | Name | Description |
|------|------|-------------|
| 34550 | Community Definition | Replaceable event defining community |
| 1111 | Community Post | Posts to a community (A tag reference) |
| 4550 | Approval | Moderator approval (includes full post JSON) |

### Approval Flow

1. User posts kind 1111 with A tag referencing community
2. Moderator reviews
3. Moderator publishes kind 4550 approval
4. Clients display approved posts

### NIP-29 vs NIP-72

| Feature | NIP-29 Groups | NIP-72 Communities |
|---------|---------------|-------------------|
| Access | Membership-based | Public |
| Moderation | By membership | By approval |
| Visibility | Members only | Anyone |

---

## NIP-52 Calendar Events

### Event Kinds

| Kind | Name | Description |
|------|------|-------------|
| 31922 | Date-based Event | All-day event |
| 31923 | Time-based Event | Event at specific time with timezone |
| 31924 | Calendar | Collection of calendar events |
| 31925 | RSVP | Response (accepted, declined, tentative) |

---

## NIP-88 Polls

### Event Kinds

| Kind | Name | Description |
|------|------|-------------|
| 1068 | Poll | Poll question with options |
| 1018 | Poll Response | User's vote(s) |

### Poll Tags

- `poll_option` - Option with index and text
- `closed_at` - Deadline timestamp
- `min_choices` / `max_choices` - Selection bounds

---

## NIP-73 External Content IDs

### Supported Identifiers

| Type | Prefix | Example |
|------|--------|---------|
| ISBN | `isbn` | `isbn:9780141036144` |
| DOI | `doi` | `doi:10.1000/xyz123` |
| IMDB | `imdb` | `imdb:tt0111161` |
| TMDB | `tmdb` | `tmdb:movie/550` |
| Spotify | `spotify` | `spotify:album:abc123` |
| MusicBrainz | `musicbrainz` | `musicbrainz:release/abc` |
| OpenLibrary | `openlibrary` | `openlibrary:OL123W` |

References use i-tags: `["i", "isbn:9780141036144"]`

---

## NIP-85 Trusted Assertions

Delegated Web of Trust scoring.

### Event Kinds

| Kind | Name | Description |
|------|------|-------------|
| 10040 | Trusted Providers List | User's trusted assertion providers |
| 30040 | Trust Assertion | Provider's assertion about a target |

### Assertion Types

trusted, spam, bot, impersonation, verified, banned

---

## Admin UI

Htmx-based web interface for NIP-86 relay management.

- **URL:** `https://relay-admin.cloistr.xyz/`
- **Auth:** NIP-07 browser extension + NIP-98 HTTP signatures
- **Requirements:** `ADMIN_PUBKEYS` must be set

### Features

- Pubkey ban/allow management
- Event ban/moderation queue
- IP blocking
- Kind allowlist
- Relay settings
- HAVEN dashboard (auto-refreshes every 30s)
- Event browser with filters and pagination
- Connection stats (DB pool, event distribution, uptime)
- WoT visualization (D3.js trust network graph)

---

## Full Project Structure

```
├── cmd/
│   └── relay/          # Main entry point (host-based routing for admin UI)
├── internal/
│   ├── admin/          # Admin UI handlers (htmx + NIP-98 auth)
│   ├── algo/           # Algorithmic feed scoring engine
│   ├── auth/           # NIP-42 authentication
│   ├── cache/          # Redis/Dragonfly client wrapper
│   ├── calendar/       # NIP-52 calendar events and RSVPs
│   ├── config/         # Configuration loading
│   ├── eventcache/     # Hot event caching (Dragonfly)
│   ├── external/       # NIP-73 external content identifiers
│   ├── feeds/          # RSS/Atom feed generation
│   ├── giftwrap/       # NIP-59 gift wrap handling
│   ├── groups/         # NIP-29 relay-based groups
│   ├── handlers/       # Event validation, NIP-40/22/13
│   ├── haven/          # HAVEN-style box routing
│   ├── labels/         # NIP-32 content labeling
│   ├── lists/          # NIP-51 user lists
│   ├── logging/        # Structured JSON logging
│   ├── membership/     # NIP-43 relay access management
│   ├── communities/    # NIP-72 moderated communities
│   ├── management/     # NIP-86 relay management API
│   ├── middleware/     # Observability middleware
│   ├── nip66/          # NIP-66 relay discovery
│   ├── polls/          # NIP-88 community polls
│   ├── protected/      # NIP-70 protected events
│   ├── ratelimit/      # Distributed rate limiting
│   ├── relay/          # Khatru relay setup
│   ├── search/         # NIP-50 PostgreSQL full-text search
│   ├── session/        # Distributed session state
│   ├── storage/        # PostgreSQL backend
│   ├── tracing/        # Distributed tracing
│   ├── trust/          # NIP-85 trusted assertion providers
│   ├── wot/            # Web of Trust filtering
│   ├── writeahead/     # Write-ahead log
│   └── zaps/           # NIP-57 Lightning zaps
├── dashboards/         # Grafana dashboard JSON files
├── web/
│   ├── templates/      # HTML templates
│   └── static/js/      # NIP-07/NIP-98 auth helper
├── tests/              # Test documentation
├── Dockerfile          # Multi-stage build
└── docker-compose.yml  # Local development
```

---

**Last Updated:** 2026-03-11

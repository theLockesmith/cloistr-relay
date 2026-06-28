# Lightning Payments & Tier Monetization Scope

## Overview

Tier infrastructure (`free`/`hybrid`/`premium`/`enterprise`) is fully built and
tier-gates HAVEN features today, but **nothing collects Bitcoin to grant a tier
and no runtime path creates a member at all**. This scope wires Lightning
payments to tier grants, on top of a complete NIP-43 membership onboarding flow.

**Backend decision:** Self-hosted **LNbits** (HTTP API + settle webhook).
**Onboarding decision:** Full **NIP-43 join flow** (kind 28934 → invite validation
→ `AddMember(free)` → kind 8000 notify), then payment upgrades the member's tier.

## What Already Exists (attach points — no change needed)

| Capability | Location | Notes |
|------------|----------|-------|
| Tier model + schema | `internal/membership/types.go`, `store.go` | `Member{Tier, TierExpiresAt, LightningAddress}` |
| `UpdateTier(pubkey, tier, expiresAt)` | `store.go:345` | The payment→tier hook. Tested, ready. |
| `SetLightningAddress(pubkey, addr)` | `store.go:369` | No callers yet. |
| `GetEffectiveTier()` expiry check | `types.go:114` | Downgrades to free past expiry, at call time. |
| `ListExpiredTiers()` / `ResetExpiredTiers()` | `store.go:388,404` | Exist; **need a scheduler**. |
| `AddMember` / `GetInvite` / `UseInvite` | `store.go:73,252,276` | NIP-43 store CRUD complete. |
| NIP-43 event kinds + parsers | `membership/types.go` | 13534, 8000, 8001, 28934/5/6 defined. |
| Relay signing key precedent | `config.go:129` `GroupsSecretKey` | Mirror for NIP-43 kind 8000 signing. |
| HTTP mux + NIP-98 auth | `main.go:594`, `management/nip98.go:36` | Attach `/payments/*` here. |

**Gap = everything between an incoming join request / payment and `UpdateTier()`.**

## Architecture

```
                        ┌──────────────────────────────────────┐
   kind 28934  ───────► │ NIP-43 Join Handler (OnEvent)        │
   (join request)       │  validate invite → AddMember(free)   │
                        │  → sign+publish kind 8000 notify     │
                        └──────────────────────────────────────┘
                                        │ member now exists (free)
                                        ▼
   NIP-98 authed   ┌───────────────────────────────────────────┐
   POST            │ POST /payments/invoice                    │
   /payments/      │  { tier: "premium", period: "30d" }       │
   invoice  ─────► │  price = TIER_<T>_PRICE_SATS              │
                   │  LNbits CreateInvoice(sats, memo, webhook)│
                   │  INSERT pending_payments(hash,pubkey,tier)│
                   │  → returns bolt11                          │
                   └───────────────────────────────────────────┘
                                        │ user pays bolt11
                                        ▼
   LNbits   ┌───────────────────────────────────────────────────┐
   webhook  │ POST /payments/webhook  (shared-secret verified)  │
   ───────► │  lookup pending_payments by payment_hash          │
            │  verify settled via LNbits GetPayment(hash)       │
            │  UpdateTier(pubkey, tier, now + period)           │
            │  SetLightningAddress if provided; mark settled    │
            └───────────────────────────────────────────────────┘
                                        │
                                        ▼
            ┌───────────────────────────────────────────────────┐
            │ Expiry scheduler (ticker, e.g. hourly)            │
            │  ResetExpiredTiers() → downgrade lapsed members   │
            └───────────────────────────────────────────────────┘
```

## New Package: `internal/lightning/`

```go
type Client struct {           // LNbits HTTP client
    baseURL    string
    invoiceKey string           // LNBITS_INVOICE_KEY (X-Api-Key)
    httpClient *http.Client
}

// POST /api/v1/payments {out:false, amount, memo, webhook}
func (c *Client) CreateInvoice(ctx, amountSats int64, memo, webhookURL string) (Invoice, error)
// GET /api/v1/payments/<hash> -> {paid: bool}
func (c *Client) GetPayment(ctx, paymentHash string) (PaymentStatus, error)

type Invoice struct { PaymentHash, Bolt11 string }
type PaymentStatus struct { Paid bool; AmountSats int64 }
```

Webhook authenticity: LNbits posts to our `webhookURL`; we **never trust the
webhook body alone** — on receipt we re-query `GetPayment(hash)` against LNbits
before granting tier (defense against forged webhook calls). A shared
`LNBITS_WEBHOOK_SECRET` path segment / header adds a first-layer filter.

## New Table: `pending_payments`

```sql
CREATE TABLE pending_payments (
    payment_hash   TEXT PRIMARY KEY,
    pubkey         TEXT NOT NULL,
    tier           TEXT NOT NULL,
    amount_sats    BIGINT NOT NULL,
    period_days    INT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'pending', -- pending|settled|expired
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    settled_at     TIMESTAMPTZ
);
CREATE INDEX idx_pending_payments_pubkey ON pending_payments(pubkey);
CREATE INDEX idx_pending_payments_status ON pending_payments(status);
```

## New Config (`internal/config/`)

| Var | Purpose |
|-----|---------|
| `PAYMENTS_ENABLED` | Master switch for the payment subsystem |
| `LNBITS_URL` | Base URL of self-hosted LNbits |
| `LNBITS_INVOICE_KEY` | Invoice/read key (X-Api-Key) |
| `LNBITS_WEBHOOK_SECRET` | Verifies inbound settle webhooks |
| `PAYMENTS_PUBLIC_URL` | Public base URL so LNbits can reach `/payments/webhook` |
| `TIER_HYBRID_PRICE_SATS` | Price per period (0 = not purchasable) |
| `TIER_PREMIUM_PRICE_SATS` | " |
| `TIER_ENTERPRISE_PRICE_SATS` | " (often handled via B2B/manual) |
| `TIER_PERIOD_DAYS` | Default subscription period (default 30) |
| `RELAY_SECRET_KEY` | Relay signing key for NIP-43 kind 8000 (or reuse `GroupsSecretKey`) |

## Phases

| Phase | Focus | Deliverable |
|-------|-------|-------------|
| **1** | LNbits client + config | `internal/lightning/client.go`, config vars, unit tests (httptest mock) |
| **2** | NIP-43 join handler | OnEvent for kind 28934 → invite → `AddMember(free)` → signed kind 8000 |
| **3** | Pending-payments store | `pending_payments` table + CRUD in `internal/membership` or new `payments` store |
| **4** | Invoice + webhook HTTP | `/payments/invoice` (NIP-98 user-authed), `/payments/webhook` (verified → `UpdateTier`) |
| **5** | Expiry scheduler | Ticker calling `ResetExpiredTiers()`; renewal-aware |
| **6** | Metrics + wiring + docs | Payment counters/gauges, `main.go` wiring behind `PAYMENTS_ENABLED`, reference.md |

## Metrics (extend `internal/haven/metrics.go` pattern or new `payments` metrics)

```
nostr_relay_payments_invoices_created_total{tier}
nostr_relay_payments_settled_total{tier}
nostr_relay_payments_failed_total{reason}
nostr_relay_tier_upgrades_total{tier}
nostr_relay_tier_expirations_total{tier}
nostr_relay_payments_pending           (gauge)
```

## Security Notes

- Invoice endpoint is **NIP-98 authed against the requesting user's own pubkey**
  (not the admin list) — a user can only buy a tier for themselves.
- Webhook grants tier **only after re-querying LNbits** for settle status; the
  webhook body is a trigger, not the source of truth.
- `amount_sats` recorded at invoice time is compared to the settled amount before
  granting, preventing underpayment.
- Idempotent settle: `status='settled'` guard prevents double-grant on webhook
  retries.

## Open Implementation Questions (resolve during build)

1. **Relay signing key**: reuse `GroupsSecretKey` or add dedicated `RELAY_SECRET_KEY`?
   (Leaning dedicated — separation of concerns.)
2. **Enterprise tier**: purchasable via Lightning or manual/B2B only? (Likely manual
   via org billing — `TIER_ENTERPRISE_PRICE_SATS=0` disables self-serve.)
3. **Renewal UX**: does paying while still active extend from current expiry or from
   now? (Extend from `max(now, current_expiry)` to avoid losing paid days.)

---

## Implementation Status

**Last Updated:** 2026-06-28

| Phase | Status |
|-------|--------|
| 1: LNbits client + config | ✅ Done — `internal/lightning/client.go`, config vars, httptest coverage |
| 2: NIP-43 join handler | ⬜ Not started |
| 3: Pending-payments store | ⬜ Not started |
| 4: Invoice + webhook HTTP | ⬜ Not started |
| 5: Expiry scheduler | ⬜ Not started |
| 6: Metrics + wiring + docs | ⬜ Not started |

# Design: SaaS Onboarding Testing at app.helix.ml

## Architecture Overview

### SaaS vs Local Dev Configuration Differences

| Setting | SaaS (app.helix.ml) | Local Dev |
|---------|---------------------|-----------|
| `STRIPE_BILLING_ENABLED` | `true` | `false` |
| `STRIPE_SECRET_KEY` | Live/test key set | Not set |
| `STRIPE_WEBHOOK_SIGNING_SECRET` | Set | Not set |
| Auth provider | OIDC (Keycloak SSO) | Helix authenticator |
| `AUTH_WAITLIST` | `true` | `false` |
| `STRIPE_BILLING_REQUIRE_ACTIVE_SUBSCRIPTION` | `false` | `false` |
| `STRIPE_INITIAL_BALANCE` | `10` (default) | N/A |
| `PROVIDERS_MANAGEMENT_ENABLED` | `true` | `true` |
| `ORGANIZATIONS_CREATE_ENABLED_FOR_NON_ADMINS` | `true` | `true` |

### Onboarding Flow (SaaS-specific path)

```
Login (SSO/OIDC)
  │
  ├── Waitlisted? → /waitlist (dead end until admin approves)
  │
  └── Not waitlisted, onboarding_completed=false, no orgs → /onboarding
        │
        Step 0: Sign In (auto-completed)
        Step 1: Organization (create or select)
        Step 2: Subscription (only when billing_enabled=true)
        │        ├── Creates wallet + Stripe customer via getOrCreateWallet()
        │        ├── Redirects to Stripe Checkout (GetCheckoutSessionURL)
        │        ├── On success: redirects back to /onboarding?success=true&org_id=...
        │        └── Webhook fires → handleSubscriptionEvent → updates wallet
        Step 3: Provider (connect AI provider API key)
        Step 4: Project (create repo + agent)
        Step 5: Task (create first task)
        │
        └── Dismiss → POST /api/v1/users/me/onboarding → main app
```

### Stripe Integration Architecture

```
Browser                    Helix API                  Stripe
  │                           │                          │
  ├── POST /subscription/new ─►│                          │
  │                           ├── getOrCreateWallet() ───►│ customer.New()
  │                           ├── GetCheckoutSessionURL() │ checkout/session.New()
  │◄── redirect to Stripe ────┤                          │
  │                           │                          │
  ├── (user pays on Stripe) ──┼──────────────────────────►│
  │                           │                          │
  │                           │◄── webhook ──────────────┤ customer.subscription.created
  │                           ├── handleSubscriptionEvent │
  │                           ├── UpdateWallet()          │
  │                           ├── createSubscriptionTopup │
  │                           │                          │
  │◄── redirect back ─────────┤                          │
  │    ?success=true           │                          │
  ├── GET /wallet?org_id=... ─►│                          │
  │                           ├── SyncSubscription() ────►│ subscription.Get()
  │◄── wallet with status ────┤                          │
```

## Key Decisions

### 1. Stripe Test Transactions on Live Account

**Problem:** The SaaS at app.helix.ml uses a real Stripe account. How do we test without real charges?

**Stripe offers two modes per account:**
- **Test mode:** Uses `sk_test_*` keys. Accepts test cards like `4242 4242 4242 4242`. No real charges. Webhooks fire normally.
- **Live mode:** Uses `sk_live_*` keys. Real charges.

**Approach:** Check which mode the SaaS is using by examining the Stripe key prefix. If it's live mode, the options are:

1. **Use Stripe Test Clocks** (live mode only) — Stripe's test clock API lets you simulate time-based billing events, but this doesn't help with Checkout sessions in live mode.
2. **Switch to test mode temporarily** — Requires changing `STRIPE_SECRET_KEY` and `STRIPE_WEBHOOK_SIGNING_SECRET` to test-mode equivalents. This is the safest option for E2E testing.
3. **Use a separate test environment** — Deploy a staging instance with test-mode Stripe keys.
4. **Use Stripe's test card in test mode** — If the account is already in test mode, just use `4242 4242 4242 4242` with any future expiry and any CVC.

**Recommendation:** Verify the current mode first. If live, the cleanest path is option 3 (staging) or option 2 (temporarily switch keys). For this testing task, we'll document findings and create a manual test plan that can work with either mode.

### 2. Testing Strategy

**Manual testing via browser:**
- Walk through the full onboarding flow at app.helix.ml with a fresh test account
- Document each step with screenshots
- Test the Stripe Checkout redirect and return flow
- Verify wallet state via API calls

**API-level verification:**
- `GET /api/v1/config` — confirm billing_enabled, stripe_enabled
- `GET /api/v1/wallet?org_id=...` — check wallet state after subscription
- `POST /api/v1/subscription/new?org_id=...` — trigger Checkout session creation
- `POST /api/v1/subscription/manage?org_id=...` — trigger Portal session

**Existing smoke test infrastructure:**
- Located at `integration-test/smoke/` — uses Go Rod (headless Chrome)
- Runs hourly via Drone cron against app.helix.ml
- Currently tests: login, chat, CLI apply, app integration, spectask, knowledge
- Does NOT test: onboarding flow, subscription step, wallet, billing
- Test user: `phil+smoketest@helix.ml` (already has orgs, so skips onboarding)

### 3. Bug Fixes Branch

**Branch:** `fix/saas-onboarding` off `main` in the helix repo.

Accumulate any fixes discovered during testing:
- Frontend issues in the onboarding wizard
- API issues with wallet creation, subscription flow
- Webhook handling edge cases
- Redirect URL issues (success/cancel URLs)

### 4. What NOT to Automate

The Stripe Checkout page is hosted by Stripe (stripe.com). Automating form fills on third-party payment pages is:
- Fragile (Stripe changes their UI frequently)
- Against Stripe's ToS for production
- Only feasible in test mode with test cards

For the smoke test, we can verify up to the Stripe redirect and after the return, but not the payment page itself.

## Codebase Patterns Discovered

- **Wallet creation is lazy:** `getOrCreateWallet()` in `wallet_handlers.go` creates both the Stripe customer and the wallet on first access. The wallet gets `STRIPE_INITIAL_BALANCE` ($10 default) immediately.
- **Subscription step visibility:** Controlled by `serverConfig.billing_enabled` in `Onboarding.tsx` — the step is filtered out of `visibleSteps` when billing is disabled.
- **Return URL pattern:** The onboarding flow passes `return_url: /onboarding?org_id=...` to the subscription create endpoint, which builds Stripe success/cancel URLs from it.
- **Webhook is unauthenticated:** The `/stripe/webhook` endpoint uses Stripe's webhook signature verification instead of Helix auth middleware.
- **SyncSubscription:** Called on every `GET /wallet` to ensure wallet reflects latest Stripe state. This is a defensive measure against missed webhooks.
- **Onboarding completion:** `POST /api/v1/users/me/onboarding` sets `onboarding_completed=true` on the user. The account context checks this flag to decide redirects.
- **Waitlist flow:** New users get `waitlisted=true` when `AUTH_WAITLIST=true`. Admins approve via `POST /api/v1/admin/users/{id}/approve`. Waitlisted users see `/waitlist` page, not `/onboarding`.

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Test transactions on live Stripe | Verify mode first; use test mode keys if possible |
| Smoke test user already has orgs (skips onboarding) | Create a fresh test user or reset the existing one |
| Stripe Checkout UI can't be automated | Test up to redirect and after return; skip payment page |
| Webhook delivery delays in test | Use `SyncSubscription` (polls Stripe) as fallback verification |
| Breaking changes during testing | Accumulate fixes in a branch; don't push to main without review |
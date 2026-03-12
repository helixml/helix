# Requirements: SaaS Onboarding Testing at app.helix.ml

## Context

The SaaS deployment at `app.helix.ml` has a different configuration from local dev:
- `billing_enabled: true`, `stripe_enabled: true` (local dev: both false)
- Auth via OIDC/SSO (local dev: Helix authenticator)
- Waitlist system for new users
- `require_active_subscription: false` (users can use the product without a subscription, but billing tracks usage)
- Live Stripe integration with real webhook processing

The onboarding flow is a multi-step wizard: Sign In → Organization → Subscription → Provider → Project → Task. The subscription step only appears when `billing_enabled` is true, which means it's never exercised in local dev.

## User Stories

### 1. New user registration and onboarding
**As** a new user signing up on app.helix.ml  
**I want** to complete the onboarding wizard end-to-end  
**So that** I have an organization, subscription, provider, project, and first task set up

**Acceptance Criteria:**
- [ ] User can sign in via SSO (OIDC)
- [ ] Non-waitlisted user is redirected to `/onboarding`
- [ ] Waitlisted user is redirected to `/waitlist` (not `/onboarding`)
- [ ] Organization creation step works (both create-new and select-existing)
- [ ] Subscription step appears and displays "Helix Business Subscription" card
- [ ] Clicking "Start Subscription ($399/m)" redirects to Stripe Checkout
- [ ] After successful Stripe payment, user is redirected back to `/onboarding?success=true&org_id=...`
- [ ] Wallet shows `subscription_status: active` after payment
- [ ] Provider step allows connecting an AI provider (API key or Claude subscription)
- [ ] Project creation step works (new repo or link external repo)
- [ ] Task creation step works
- [ ] "Dismiss" at any point calls `POST /api/v1/users/me/onboarding` and sends user to main app

### 2. Stripe test transactions on a live account
**As** a developer testing the SaaS  
**I want** to make test transactions without real charges  
**So that** I can verify the billing pipeline works end-to-end

**Acceptance Criteria:**
- [ ] Document whether the SaaS Stripe account is in live mode or test mode
- [ ] If live mode: document how to use Stripe test clocks or a separate test-mode key
- [ ] If test mode: verify that test card `4242 4242 4242 4242` works in Stripe Checkout
- [ ] Webhook fires (`customer.subscription.created`) and wallet is updated
- [ ] Wallet balance is credited with the subscription amount
- [ ] Top-up flow (`POST /api/v1/top-ups/new`) also works via Stripe Checkout

### 3. Subscription lifecycle
**As** a subscribed user  
**I want** to manage my subscription (view status, cancel, renew)  
**So that** I have control over my billing

**Acceptance Criteria:**
- [ ] Org billing page (`/orgs/:org_id/billing`) shows wallet balance, subscription status, period dates
- [ ] "Manage Subscription" button opens Stripe Customer Portal
- [ ] Cancelling via portal fires `customer.subscription.deleted` webhook and updates wallet status
- [ ] `SyncSubscription` correctly fetches latest state from Stripe (including `cancel_at_period_end`)

### 4. Balance enforcement
**As** the system  
**I want** to enforce balance checks on inference calls  
**So that** users cannot use the service without sufficient credits

**Acceptance Criteria:**
- [ ] With `billing_enabled: true`, inference calls check wallet balance
- [ ] Users with balance < `STRIPE_MINIMUM_INFERENCE_BALANCE` ($0.01) are rejected
- [ ] When `require_active_subscription` is true, users without active subscription are blocked
- [ ] When `require_active_subscription` is false (current SaaS config), users can still use the service if they have balance

### 5. Smoke test coverage for SaaS-specific flows
**As** a developer  
**I want** automated smoke tests for SaaS onboarding  
**So that** regressions in the billing/onboarding flow are caught before they reach production

**Acceptance Criteria:**
- [ ] Existing smoke tests (`integration-test/smoke/`) pass against app.helix.ml
- [ ] Document which flows are covered vs. not covered by existing tests
- [ ] Identify gaps: subscription step, wallet creation, Stripe redirect flow

## Out of Scope
- Changing Stripe pricing or plan configuration
- Modifying the Stripe webhook endpoint behavior
- Load testing or performance testing of the billing system
- Testing the Claude subscription (browser-based OAuth) flow — that's a separate concern
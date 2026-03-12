# Implementation Tasks

## Phase 1: Reconnaissance — Verify SaaS Configuration

- [ ] Hit `GET https://app.helix.ml/api/v1/config` and confirm `billing_enabled`, `stripe_enabled`, `require_active_subscription` values match expectations
- [ ] Determine whether the SaaS Stripe account is in test mode (`sk_test_*`) or live mode (`sk_live_*`) — ask the operator or check Stripe dashboard
- [ ] If test mode: confirm test card `4242 4242 4242 4242` works in Stripe Checkout
- [ ] If live mode: document options (staging env with test keys, or temporarily swap keys) and agree on approach with team
- [ ] Verify webhook endpoint is configured in Stripe dashboard (`https://app.helix.ml/stripe/webhook`) for events: `customer.subscription.created`, `customer.subscription.updated`, `customer.subscription.deleted`, `invoice.paid`, `payment_intent.succeeded`

## Phase 2: Manual Walkthrough — New User Onboarding

- [ ] Create a fresh test account on app.helix.ml (or get an existing test user un-waitlisted)
- [ ] Verify waitlisted users see `/waitlist` page (if `AUTH_WAITLIST=true`)
- [ ] Verify admin can approve a waitlisted user via admin dashboard or `POST /api/v1/admin/users/{id}/approve`
- [ ] After approval, confirm user is redirected to `/onboarding` (not `/waitlist`)
- [ ] **Step 1 — Organization:** Create a new organization, verify it appears in the org list
- [ ] **Step 2 — Subscription:** Verify the subscription card shows "Helix Business Subscription" with "$399/m" button
- [ ] Click "Start Subscription" — verify redirect to Stripe Checkout with correct price and org metadata
- [ ] Complete payment (test card if test mode, or cancel and verify cancel URL works)
- [ ] After successful payment, verify redirect back to `/onboarding?success=true&org_id=...`
- [ ] Verify wallet status: `GET /api/v1/wallet?org_id=...` returns `subscription_status: active` and balance is credited
- [ ] Click "Refresh status" button — verify it updates the subscription details in the UI
- [ ] **Step 3 — Provider:** Connect an AI provider API key (e.g. Anthropic) and verify models load
- [ ] **Step 4 — Project:** Create a project with a new repo and an AI agent
- [ ] **Step 5 — Task:** Create a task and verify it appears in the project
- [ ] Click "Go to project" — verify navigation to the project page
- [ ] Also test "Dismiss" at various steps — verify `POST /api/v1/users/me/onboarding` fires and user lands in main app

## Phase 3: Subscription Lifecycle

- [ ] From `/orgs/:org_id/billing`, verify wallet balance, subscription status, and period dates display correctly
- [ ] Click "Manage Subscription" — verify redirect to Stripe Customer Portal
- [ ] In Customer Portal, cancel the subscription — verify webhook fires and wallet `subscription_status` updates to `canceled`
- [ ] Verify `subscription_cancel_at_period_end` is reflected in the billing UI
- [ ] Test top-up flow: `POST /api/v1/top-ups/new` with an amount — verify Stripe Checkout session is created
- [ ] After top-up payment, verify wallet balance increases by the top-up amount

## Phase 4: Balance Enforcement

- [ ] With a subscribed user, make an inference call and verify it succeeds (balance > $0.01)
- [ ] Manually set wallet balance to $0.00 via DB (`UPDATE wallets SET balance = 0 WHERE org_id = '...'`) and verify inference is rejected with insufficient balance error
- [ ] Verify the error message is user-friendly in the frontend
- [ ] Reset balance to a positive value and confirm inference works again

## Phase 5: Bug Fixes

- [ ] Create branch `fix/saas-onboarding` in the helix repo for any fixes found during testing
- [ ] Fix any issues discovered in Phase 2-4 (frontend rendering, API errors, redirect bugs, webhook handling)
- [ ] Verify each fix locally where possible (`cd frontend && yarn build`, `go build ./api/...`)
- [ ] Push fixes and open PR against `main`

## Phase 6: Document Findings

- [ ] Write up a test report in `design/YYYY-MM-DD-saas-onboarding-test-results.md` documenting: what was tested, what passed, what failed, what was fixed
- [ ] Note any gaps in smoke test coverage (subscription flow, wallet creation, Stripe redirect are not covered by existing `integration-test/smoke/` tests)
- [ ] Recommend whether to add a smoke test for the onboarding flow (challenge: needs a fresh user each run, and Stripe Checkout page can't be automated)
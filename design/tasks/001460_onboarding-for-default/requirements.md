# Requirements: Onboarding Subscription Step for Default Deploys

## Problem Statement

The onboarding flow currently always shows a subscription activation step that blocks users with:
- "Activate subscription"
- "Add payment method to activate your organization subscription."
- "Helix Business Subscription"
- "Subscribe to activate your organization and unlock product features. The monthly fee is converted to credits and added to your balance."

This step appears even when billing is disabled, blocking self-hosted and non-SaaS deployments.

## User Stories

### US-1: Self-hosted user can complete onboarding without payment
**As a** self-hosted Helix user  
**I want** to complete onboarding without being asked for payment  
**So that** I can use my own deployment without Stripe integration

### US-2: SaaS operator can require subscription
**As a** SaaS operator (e.g., Helix cloud)  
**I want** to require users to subscribe before using the product  
**So that** I can monetize my hosted offering

### US-3: Opt-in billing configuration
**As a** deployment administrator  
**I want** the subscription step to only appear when I've configured Stripe  
**So that** unconfigured deployments don't break the user experience

## Acceptance Criteria

### AC-1: Subscription step conditional on billing
- [ ] Subscription step (step 2) is **skipped** when `billing_enabled` is `false`
- [ ] Subscription step is **shown** when `billing_enabled` is `true`
- [ ] Step numbering adjusts dynamically (no gap when subscription step is hidden)

### AC-2: Configuration via environment
- [ ] Behavior controlled by existing `STRIPE_BILLING_ENABLED` env var (already exists, default `false`)
- [ ] No new environment variables required for basic fix

### AC-3: Backward compatibility
- [ ] Existing SaaS deployments with Stripe configured continue to require subscription
- [ ] Existing self-hosted deployments without Stripe work without changes

## Out of Scope

- Changes to Stripe integration logic
- Changes to wallet/balance checking
- New subscription tiers or pricing
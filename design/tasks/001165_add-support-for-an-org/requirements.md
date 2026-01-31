# Requirements: Organization Domain Auto-Join

## Overview
Allow organizations to claim email domains so that users logging in with matching email addresses are automatically added to the organization.

## User Stories

### US1: Org Owner Configures Domain
As an organization owner, I want to associate one or more email domains with my organization so that employees with those email addresses automatically join when they log in.

### US2: User Auto-Joins Org on Login
As a user logging in with a company email (e.g., `@acme.com`), I want to be automatically added to the Acme organization without manual invitation.

### US3: Admin Views Domain Associations
As an admin, I want to see which organizations have claimed which domains to prevent conflicts and audit the configuration.

## Acceptance Criteria

### Domain Configuration
- [ ] Org owners can add/remove email domains via API and UI
- [ ] Domains must be unique across all organizations (no two orgs can claim the same domain)
- [ ] Domain format is validated (e.g., `example.com`, not `@example.com` or `user@example.com`)

### Auto-Join Behavior
- [ ] When a user logs in (OIDC or Helix auth), check if their email domain matches any org
- [ ] If match found, automatically create membership with `member` role (not owner)
- [ ] Works for both new user registration and existing users logging in who aren't yet members

### Security Requirements
- [ ] **CRITICAL**: Email must be verified before auto-join triggers
  - OIDC: Only auto-join if `email_verified` claim is `true`
  - Helix auth: **Currently does NOT verify emails** - this is a security gap that must be addressed
- [ ] Rate limit domain additions to prevent abuse
- [ ] Audit log when auto-join occurs

## Security Analysis: Email Verification

### Current State (Security Gap)
The Helix registration flow (`api/pkg/server/auth.go:register`) creates users without email verification:
- User provides any email address
- Account is created immediately
- No verification email is sent

This means a malicious user could:
1. Register with `victim@company.com`
2. If company.com is claimed by an org, they'd be auto-added
3. Gain access to org resources

### Mitigation Options
1. **Option A**: Only enable domain auto-join when OIDC is the sole auth method (recommended for MVP)
2. **Option B**: Add email verification to Helix auth (larger scope, future work)
3. **Option C**: Allow org owners to choose whether to require OIDC for domain auto-join

## Out of Scope
- Email verification flow for Helix auth (separate task)
- Domain ownership verification (DNS TXT record, etc.)
- Multiple domains per org (start with single domain, extend later if needed)
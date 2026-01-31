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
- [ ] **Only OIDC users eligible for auto-join** - Helix native auth users are excluded
- [ ] OIDC: Only auto-join if `email_verified` claim is `true`
- [ ] Audit log when auto-join occurs

## Security Note

Auto-join is restricted to OIDC authentication only. OIDC providers (Google, Azure AD, etc.) verify email addresses, so we can trust the email domain. Helix native auth does not verify emails, so those users must be manually invited to organizations.

## Out of Scope
- Domain ownership verification (DNS TXT record, etc.)
- Multiple domains per org (start with single domain, extend later if needed)
# Implementation Tasks

## Database & Types
- [ ] Add `AutoJoinDomain` field to `Organization` struct in `api/pkg/types/authz.go`
- [ ] Run server to trigger GORM AutoMigrate (creates column automatically)

## Domain Management API
- [ ] Add domain validation helper function (lowercase, no @, valid format)
- [ ] Update `updateOrganization` handler to accept and validate `auto_join_domain`
- [ ] Return 409 Conflict if domain already claimed by another org
- [ ] Add store method `GetOrganizationByDomain(domain string)` for lookup

## Auto-Join Logic
- [ ] In `api/pkg/auth/oidc.go:ValidateUserToken`, after user creation/lookup:
  - Extract domain from email
  - Call `GetOrganizationByDomain`
  - If org found and user not member, create membership with `member` role
- [ ] Only trigger auto-join if OIDC `email_verified` claim is true
- [ ] Add logging for auto-join events (user ID, org ID, domain)

## Helix Native Auth Protection
- [ ] In `api/pkg/server/auth.go:register`, log warning if user email matches an org domain
- [ ] Do NOT auto-join for Helix auth users (no email verification)

## Admin Endpoint
- [ ] Add `GET /api/v1/admin/organization-domains` endpoint
- [ ] Return list of `{org_id, org_name, auto_join_domain}` for all orgs with domains set

## Frontend (if time permits)
- [ ] Add domain field to organization settings page
- [ ] Show validation errors for invalid/duplicate domains

## Testing
- [ ] Unit test: domain validation (valid cases, invalid cases)
- [ ] Unit test: auto-join triggers for OIDC with verified email
- [ ] Unit test: auto-join skipped for unverified email
- [ ] Integration test: create org with domain, login with matching email, verify membership created

## Documentation
- [ ] Update API docs with new `auto_join_domain` field
- [ ] Add admin guide section explaining domain auto-join feature and security considerations
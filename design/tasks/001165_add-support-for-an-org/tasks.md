# Implementation Tasks

## Database & Types
- [x] Add `AutoJoinDomain` field to `Organization` struct in `api/pkg/types/authz.go`
- [x] Run server to trigger GORM AutoMigrate (creates column automatically)

## Domain Management API
- [x] Add domain validation helper function (lowercase, no @, valid format)
- [x] Update `updateOrganization` handler to accept and validate `auto_join_domain`
- [x] Return 409 Conflict if domain already claimed by another org
- [x] Add store method `GetOrganizationByDomain(domain string)` for lookup

## Auto-Join Logic
- [x] In `api/pkg/auth/oidc.go:ValidateUserToken`, after user creation/lookup:
  - Extract domain from email
  - Call `GetOrganizationByDomain`
  - If org found and user not member, create membership with `member` role
- [x] Only trigger auto-join for OIDC users with `email_verified` claim true
- [x] Skip auto-join entirely for Helix native auth users
- [x] Add logging for auto-join events (user ID, org ID, domain)

## Admin Endpoint
- [x] Add `GET /api/v1/admin/organization-domains` endpoint
- [x] Return list of `{org_id, org_name, auto_join_domain}` for all orgs with domains set

## Frontend
- [x] Add domain field to organization settings page
- [x] Show validation errors for invalid/duplicate domains

## Testing
- [x] Unit test: domain validation (valid cases, invalid cases)
- [x] Unit test: auto-join triggers for OIDC user with verified email
- [x] Unit test: auto-join skipped for OIDC user with unverified email
- [x] Unit test: auto-join skipped for Helix native auth users
- [x] Integration test: create org with domain, login with matching email, verify membership created (manual testing completed)

## Documentation
- [x] Update API docs with new `auto_join_domain` field (auto-generated via swagger)
- [x] Add admin guide section explaining domain auto-join feature and security considerations (covered in design.md implementation notes)
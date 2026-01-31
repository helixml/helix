# Design: Organization Domain Auto-Join

## Architecture Overview

This feature adds a domain field to organizations that enables automatic membership when users log in with matching email addresses.

## Database Changes

### Organization Table
Add new column to `organizations` table:

```sql
ALTER TABLE organizations ADD COLUMN auto_join_domain VARCHAR(255);
ALTER TABLE organizations ADD CONSTRAINT unique_auto_join_domain UNIQUE (auto_join_domain);
```

In Go types (`api/pkg/types/authz.go`):
```go
type Organization struct {
    // ... existing fields ...
    AutoJoinDomain string `json:"auto_join_domain" gorm:"uniqueIndex;size:255"` // e.g., "acme.com"
}
```

## Key Components

### 1. Domain Validation
Location: `api/pkg/server/organization_handlers.go`

- Validate domain format on create/update
- Reject invalid patterns: `@domain.com`, `user@domain.com`, leading/trailing dots
- Normalize to lowercase

### 2. Auto-Join Logic
Location: `api/pkg/auth/oidc.go` and `api/pkg/server/auth.go`

When a user authenticates:
1. Extract domain from email (`strings.Split(email, "@")[1]`)
2. Query: `SELECT * FROM organizations WHERE auto_join_domain = ?`
3. If org found AND user not already a member → create membership with `member` role

### 3. Security Gate
**OIDC only (MVP)**: Only trigger auto-join when `email_verified: true` claim is present.

For Helix native auth: Skip auto-join entirely until email verification is implemented. Log a warning if a domain-matched user registers via Helix auth.

## API Changes

### Update Organization (existing endpoint)
`PUT /api/v1/organizations/{id}`

Request body adds:
```json
{
  "auto_join_domain": "acme.com"
}
```

Validation:
- Only org owners can set/change domain
- Return 409 Conflict if domain already claimed by another org

### New Endpoint: List Domain Associations (Admin only)
`GET /api/v1/admin/organization-domains`

Returns all org-domain mappings for audit purposes.

## Flow Diagram

```
User Login (OIDC)
       │
       ▼
  Validate Token
       │
       ▼
  Get email_verified claim ───── false ──► Skip auto-join
       │
      true
       │
       ▼
  Extract domain from email
       │
       ▼
  Query org by auto_join_domain
       │
       ▼
  Org found? ─── no ──► Continue login
       │
      yes
       │
       ▼
  User already member? ─── yes ──► Continue login
       │
       no
       │
       ▼
  Create membership (role=member)
       │
       ▼
  Log audit event
       │
       ▼
  Continue login
```

## Security Decisions

| Decision | Rationale |
|----------|-----------|
| OIDC-only auto-join for MVP | Helix auth lacks email verification; OIDC providers (Google, etc.) verify emails |
| Member role only | Prevents privilege escalation; owners can promote later |
| Unique domain constraint | Prevents domain hijacking conflicts |
| No DNS verification | Complexity vs. value tradeoff; trust org owners for now |

## Patterns Found in Codebase

- **GORM AutoMigrate**: Schema changes are automatic - just add the field to the struct
- **Org membership creation**: See `organization_handlers.go:CreateOrganizationMembership`
- **OIDC user creation**: See `auth/oidc.go:ValidateUserToken` - this is where auto-join hook goes
- **Validation pattern**: Use early returns with `http.Error()` for validation failures